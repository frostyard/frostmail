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

// SmartMailboxUpdate changes the non-nil fields of a smart mailbox; a nil
// field keeps what is stored.
type SmartMailboxUpdate struct {
	Name         *string
	Conditions   *api.Conditions
	IncludeTrash *bool
	IncludeSent  *bool
}

// UpdateSmartMailbox changes the given fields of a smart mailbox and emits
// api.SmartChanged, returning the row as stored. It reads the row inside
// its transaction, so a change it does not give is kept (ErrNotFound when
// there is no such smart mailbox). The caller has checked the conditions it
// gives.
func (t *Tx) UpdateSmartMailbox(ctx context.Context, id int64, u SmartMailboxUpdate) (SmartMailbox, error) {
	cur, err := scanSmart(t.QueryRowContext(ctx, smartColumns+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return SmartMailbox{}, ErrNotFound
	}
	if err != nil {
		return SmartMailbox{}, fmt.Errorf("find smart mailbox %d: %w", id, err)
	}
	if u.Name != nil {
		cur.Name = *u.Name
	}
	if u.Conditions != nil {
		cur.Conditions = *u.Conditions
	}
	if u.IncludeTrash != nil {
		cur.IncludeTrash = *u.IncludeTrash
	}
	if u.IncludeSent != nil {
		cur.IncludeSent = *u.IncludeSent
	}
	conds, err := encodeConditions(cur.Conditions)
	if err != nil {
		return SmartMailbox{}, fmt.Errorf("update smart mailbox %d: %w", id, err)
	}
	_, err = t.ExecContext(ctx, `UPDATE smart_mailboxes SET name = ?, conditions_json = ?, include_trash = ?, include_sent = ?
		WHERE id = ?`, cur.Name, conds, bit(cur.IncludeTrash), bit(cur.IncludeSent), id)
	if err != nil {
		return SmartMailbox{}, fmt.Errorf("update smart mailbox %d: %w", id, err)
	}
	if err := t.Emit(ctx, api.SmartChanged{ID: id}); err != nil {
		return SmartMailbox{}, err
	}
	return getSmart(t.QueryRowContext(ctx, smartColumns+` WHERE id = ?`, id))
}

// DeleteSmartMailbox removes a smart mailbox, closes the gap in the
// positions, makes a notification scope that named it Inbox only, and
// emits api.SmartChanged (and api.SettingsChanged when the scope moved).
func (t *Tx) DeleteSmartMailbox(ctx context.Context, id int64) error {
	if err := t.deleteRow(ctx, "smart_mailboxes", id); err != nil {
		return err
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
	moved, err := t.moveRow(ctx, "smart_mailboxes", id, position)
	if err != nil || !moved {
		return err
	}
	return t.Emit(ctx, api.SmartChanged{ID: id})
}

// moveRow puts a row of a positioned table (smart_mailboxes, rules) at
// position, clamped to the list, moving the others along. It reports
// whether the row moved; ErrNotFound when there is no such row.
func (t *Tx) moveRow(ctx context.Context, table string, id int64, position int) (bool, error) {
	var from, count int
	err := t.QueryRowContext(ctx, `SELECT position, (SELECT COUNT(*) FROM `+table+`) FROM `+table+` WHERE id = ?`, id).
		Scan(&from, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("move %s: %w", table, err)
	}
	to := min(max(position, 0), count-1)
	if to == from {
		return false, nil
	}
	shift := `UPDATE ` + table + ` SET position = position - 1 WHERE position > ? AND position <= ?`
	lo, hi := from, to
	if to < from {
		shift = `UPDATE ` + table + ` SET position = position + 1 WHERE position >= ? AND position < ?`
		lo, hi = to, from
	}
	if _, err := t.ExecContext(ctx, shift, lo, hi); err != nil {
		return false, fmt.Errorf("move %s: %w", table, err)
	}
	if _, err := t.ExecContext(ctx, `UPDATE `+table+` SET position = ? WHERE id = ?`, to, id); err != nil {
		return false, fmt.Errorf("move %s: %w", table, err)
	}
	return true, nil
}

// deleteRow deletes a row of a positioned table and closes the gap;
// ErrNotFound when there is no such row.
func (t *Tx) deleteRow(ctx context.Context, table string, id int64) error {
	var pos int
	err := t.QueryRowContext(ctx, `DELETE FROM `+table+` WHERE id = ? RETURNING position`, id).Scan(&pos)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}
	if _, err := t.ExecContext(ctx, `UPDATE `+table+` SET position = position - 1 WHERE position > ?`, pos); err != nil {
		return fmt.Errorf("delete from %s: %w", table, err)
	}
	return nil
}
