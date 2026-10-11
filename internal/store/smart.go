package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/frostyard/frostmail/api"
)

// SmartMailbox is saved conditions listed as a mailbox (ADR-0023,
// docs/design/organize.md, Smart mailboxes).
type SmartMailbox struct {
	ID           int64
	Name         string
	Position     int // from 0, in sidebar order
	Conditions   api.Conditions
	IncludeTrash bool
	IncludeSent  bool
}

// storedConditions is how conditions are kept as JSON: with a version, so
// a later maild can read what an older one wrote.
type storedConditions struct {
	V          int             `json:"v"`
	Match      string          `json:"match"`
	Conditions []api.Condition `json:"conditions"`
}

const conditionsVersion = 1

func encodeConditions(c api.Conditions) (string, error) {
	data, err := json.Marshal(storedConditions{V: conditionsVersion, Match: string(c.Match), Conditions: c.Conditions})
	return string(data), err
}

func decodeConditions(s string) (api.Conditions, error) {
	var sc storedConditions
	if err := json.Unmarshal([]byte(s), &sc); err != nil {
		return api.Conditions{}, err
	}
	if sc.V != conditionsVersion {
		return api.Conditions{}, fmt.Errorf("conditions version %d is not %d", sc.V, conditionsVersion)
	}
	if sc.Conditions == nil {
		sc.Conditions = []api.Condition{}
	}
	return api.Conditions{Match: api.ConditionMatch(sc.Match), Conditions: sc.Conditions}, nil
}

const smartColumns = `SELECT id, name, position, conditions_json, include_trash, include_sent FROM smart_mailboxes`

func scanSmart(row rowScanner) (SmartMailbox, error) {
	var s SmartMailbox
	var conds string
	if err := row.Scan(&s.ID, &s.Name, &s.Position, &conds, &s.IncludeTrash, &s.IncludeSent); err != nil {
		return SmartMailbox{}, err
	}
	c, err := decodeConditions(conds)
	if err != nil {
		return SmartMailbox{}, fmt.Errorf("smart mailbox %d: %w", s.ID, err)
	}
	s.Conditions = c
	return s, nil
}

// SmartMailboxes returns every smart mailbox in position order.
func (d *DB) SmartMailboxes(ctx context.Context) ([]SmartMailbox, error) {
	rows, err := d.db.QueryContext(ctx, smartColumns+` ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("smart mailboxes: %w", err)
	}
	defer rows.Close()
	out := []SmartMailbox{}
	for rows.Next() {
		s, err := scanSmart(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SmartMailbox returns one smart mailbox, or ErrNotFound.
func (d *DB) SmartMailbox(ctx context.Context, id int64) (SmartMailbox, error) {
	return getSmart(d.db.QueryRowContext(ctx, smartColumns+` WHERE id = ?`, id))
}

func getSmart(row *sql.Row) (SmartMailbox, error) {
	s, err := scanSmart(row)
	if errors.Is(err, sql.ErrNoRows) {
		return SmartMailbox{}, ErrNotFound
	}
	return s, err
}

// CreateSmartMailbox adds s at the end of the list and emits
// api.SmartChanged. The caller has checked its conditions; s.ID and
// s.Position are ignored.
func (t *Tx) CreateSmartMailbox(ctx context.Context, s SmartMailbox) (SmartMailbox, error) {
	conds, err := encodeConditions(s.Conditions)
	if err != nil {
		return SmartMailbox{}, fmt.Errorf("create smart mailbox: %w", err)
	}
	var id int64
	err = t.QueryRowContext(ctx, `INSERT INTO smart_mailboxes (name, position, conditions_json, include_trash, include_sent)
		VALUES (?, (SELECT COALESCE(MAX(position) + 1, 0) FROM smart_mailboxes), ?, ?, ?) RETURNING id`,
		s.Name, conds, bit(s.IncludeTrash), bit(s.IncludeSent)).Scan(&id)
	if err != nil {
		return SmartMailbox{}, fmt.Errorf("create smart mailbox: %w", err)
	}
	if err := t.Emit(ctx, api.SmartChanged{ID: id}); err != nil {
		return SmartMailbox{}, err
	}
	return getSmart(t.QueryRowContext(ctx, smartColumns+` WHERE id = ?`, id))
}

// UpdateSmartMailbox stores s's name, conditions and includes, and emits
// api.SmartChanged; ErrNotFound when there is no such smart mailbox.
func (t *Tx) UpdateSmartMailbox(ctx context.Context, s SmartMailbox) (SmartMailbox, error) {
	conds, err := encodeConditions(s.Conditions)
	if err != nil {
		return SmartMailbox{}, fmt.Errorf("update smart mailbox: %w", err)
	}
	res, err := t.ExecContext(ctx, `UPDATE smart_mailboxes SET name = ?, conditions_json = ?, include_trash = ?, include_sent = ?
		WHERE id = ?`, s.Name, conds, bit(s.IncludeTrash), bit(s.IncludeSent), s.ID)
	if err != nil {
		return SmartMailbox{}, fmt.Errorf("update smart mailbox: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return SmartMailbox{}, ErrNotFound
	}
	if err := t.Emit(ctx, api.SmartChanged{ID: s.ID}); err != nil {
		return SmartMailbox{}, err
	}
	return getSmart(t.QueryRowContext(ctx, smartColumns+` WHERE id = ?`, s.ID))
}

// DeleteSmartMailbox removes a smart mailbox, closes the gap in the
// positions, makes a notification scope that named it Inbox only, and
// emits api.SmartChanged (and api.SettingsChanged when the scope moved).
func (t *Tx) DeleteSmartMailbox(ctx context.Context, id int64) error {
	var pos int
	err := t.QueryRowContext(ctx, `DELETE FROM smart_mailboxes WHERE id = ? RETURNING position`, id).Scan(&pos)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete smart mailbox: %w", err)
	}
	if _, err := t.ExecContext(ctx, `UPDATE smart_mailboxes SET position = position - 1 WHERE position > ?`, pos); err != nil {
		return fmt.Errorf("delete smart mailbox: %w", err)
	}
	if err := t.Emit(ctx, api.SmartChanged{ID: id, Deleted: true}); err != nil {
		return err
	}
	s, err := t.d.Settings(ctx)
	if err != nil {
		return err
	}
	if s.NotifyScope == string(api.NotifyScopeSmart) && s.NotifySmartID == id {
		s.NotifyScope, s.NotifySmartID = string(api.NotifyScopeInbox), 0
		return t.SetSettings(ctx, s)
	}
	return nil
}

// MoveSmartMailbox puts a smart mailbox at position (clamped to the list),
// moving the others along, and emits api.SmartChanged.
func (t *Tx) MoveSmartMailbox(ctx context.Context, id int64, position int) error {
	var from, count int
	err := t.QueryRowContext(ctx, `SELECT position, (SELECT COUNT(*) FROM smart_mailboxes) FROM smart_mailboxes WHERE id = ?`, id).
		Scan(&from, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("move smart mailbox: %w", err)
	}
	to := min(max(position, 0), count-1)
	if to == from {
		return nil
	}
	shift := `UPDATE smart_mailboxes SET position = position - 1 WHERE position > ? AND position <= ?`
	if to < from {
		shift = `UPDATE smart_mailboxes SET position = position + 1 WHERE position >= ? AND position < ?`
	}
	lo, hi := from, to
	if to < from {
		lo, hi = to, from
	}
	if _, err := t.ExecContext(ctx, shift, lo, hi); err != nil {
		return fmt.Errorf("move smart mailbox: %w", err)
	}
	if _, err := t.ExecContext(ctx, `UPDATE smart_mailboxes SET position = ? WHERE id = ?`, to, id); err != nil {
		return fmt.Errorf("move smart mailbox: %w", err)
	}
	return t.Emit(ctx, api.SmartChanged{ID: id})
}
