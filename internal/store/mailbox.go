package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
)

// Mailbox is a mailboxes row with its local counts.
type Mailbox struct {
	ID        int64
	AccountID int64
	Path      string
	Name      string
	Delimiter string
	Role      api.MailboxRole
	Total     int64
	Unread    int64
	Label     bool // a Gmail label (ADR-0012)
}

// ListMailboxes returns mailboxes ordered by account, role (inbox first,
// user folders last) and path. accountID 0 means every account.
func (d *DB) ListMailboxes(ctx context.Context, accountID int64) ([]Mailbox, error) {
	return listMailboxes(ctx, d.db, accountID)
}

// ListMailboxes is DB.ListMailboxes inside the transaction.
func (t *Tx) ListMailboxes(ctx context.Context, accountID int64) ([]Mailbox, error) {
	return listMailboxes(ctx, t, accountID)
}

// querier is satisfied by *sql.DB and *Tx, so a query helper can run inside
// a write transaction and see its uncommitted rows.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func listMailboxes(ctx context.Context, q querier, accountID int64) ([]Mailbox, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT mb.id, mb.account_id, mb.path, mb.name, mb.delimiter, mb.role, mb.is_gmail_label,
		       COUNT(mm.message_id), COALESCE(SUM(m.seen = 0), 0)
		FROM mailboxes mb
		LEFT JOIN message_mailbox mm ON mm.mailbox_id = mb.id
		LEFT JOIN messages m ON m.id = mm.message_id
		WHERE ? = 0 OR mb.account_id = ?
		GROUP BY mb.id
		ORDER BY mb.account_id,
		         CASE mb.role WHEN 'inbox' THEN 0 WHEN 'drafts' THEN 1 WHEN 'sent' THEN 2
		              WHEN 'archive' THEN 3 WHEN 'all' THEN 4 WHEN 'flagged' THEN 5
		              WHEN 'junk' THEN 6 WHEN 'trash' THEN 7 ELSE 8 END,
		         mb.path`, accountID, accountID)
	if err != nil {
		return nil, fmt.Errorf("list mailboxes: %w", err)
	}
	defer rows.Close()
	out := []Mailbox{}
	for rows.Next() {
		var mb Mailbox
		var role string
		if err := rows.Scan(&mb.ID, &mb.AccountID, &mb.Path, &mb.Name, &mb.Delimiter, &role, &mb.Label, &mb.Total, &mb.Unread); err != nil {
			return nil, fmt.Errorf("list mailboxes: %w", err)
		}
		mb.Role = api.MailboxRole(role)
		out = append(out, mb)
	}
	return out, rows.Err()
}

// ServerMailbox is one mailbox as LIST reports it, with its role resolved.
type ServerMailbox struct {
	Path       string // decoded from modified UTF-7
	Delimiter  string // "" for a flat namespace
	Role       api.MailboxRole
	Attrs      []string // raw LIST attributes, such as \HasNoChildren
	Selectable bool     // false for \Noselect and \NonExistent
	Subscribed bool
}

// SyncState is a mailbox's position in the reconcile pass
// (docs/design/sync.md).
type SyncState struct {
	UIDValidity   uint32
	UIDNext       uint32
	HighestModSeq uint64 // 0 without CONDSTORE
	ServerCount   uint32 // MESSAGES when the pass ended
	LastSyncAt    time.Time
}

// storedMailbox is the subset of a mailboxes row ReplaceMailboxes compares.
type storedMailbox struct {
	id         int64
	path       string
	delimiter  string
	name       string
	role       string
	attrsJSON  string
	selectable bool
	subscribed bool
}

func loadStoredMailboxes(ctx context.Context, q querier, accountID int64) ([]storedMailbox, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, path, delimiter, name, role, attrs_json, selectable, subscribed
		FROM mailboxes WHERE account_id = ? ORDER BY id`, accountID)
	if err != nil {
		return nil, fmt.Errorf("load mailboxes: %w", err)
	}
	defer rows.Close()
	var out []storedMailbox
	for rows.Next() {
		var m storedMailbox
		if err := rows.Scan(&m.id, &m.path, &m.delimiter, &m.name, &m.role, &m.attrsJSON, &m.selectable, &m.subscribed); err != nil {
			return nil, fmt.Errorf("load mailboxes: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// mailboxName is the last component of path after delim, or the whole path
// when delim is empty or absent.
func mailboxName(path, delim string) string {
	if delim == "" {
		return path
	}
	if i := strings.LastIndex(path, delim); i >= 0 {
		return path[i+len(delim):]
	}
	return path
}

// marshalAttrs renders a LIST attribute list for attrs_json; nil becomes [].
func marshalAttrs(attrs []string) (string, error) {
	if attrs == nil {
		return "[]", nil
	}
	b, err := json.Marshal(attrs)
	if err != nil {
		return "", fmt.Errorf("marshal attrs: %w", err)
	}
	return string(b), nil
}

func bit(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// ReplaceMailboxes makes accountID's mailboxes match list, a full LIST
// result, and returns the account's mailboxes afterwards in ListMailboxes
// order, read through t. A path not stored yet is inserted; a stored path has
// its delimiter, name, role, attributes, selectable and subscribed columns
// updated, the roles after the account's own choices (Use This Mailbox
// For); a stored path missing from list is deleted, together with the
// messages left in no mailbox (and their search entries). Name is the last
// component of Path after Delimiter (the whole path when Delimiter is empty).
// It emits api.MailboxChanged for each mailbox inserted, changed or deleted
// (Deleted: true), and api.MessageRemoved for messages deleted with a
// mailbox. Task T-0012 implements it.
func (t *Tx) ReplaceMailboxes(ctx context.Context, accountID int64, list []ServerMailbox) ([]Mailbox, error) {
	list = slices.Clone(list)
	if err := t.applyRoleChoices(ctx, accountID, list); err != nil {
		return nil, err
	}
	stored, err := loadStoredMailboxes(ctx, t, accountID)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]storedMailbox, len(stored))
	for _, m := range stored {
		byPath[m.path] = m
	}
	present := make(map[string]bool, len(list))
	for _, sm := range list {
		present[sm.Path] = true
		name := mailboxName(sm.Path, sm.Delimiter)
		attrsJSON, err := marshalAttrs(sm.Attrs)
		if err != nil {
			return nil, err
		}
		cur, ok := byPath[sm.Path]
		if !ok {
			res, err := t.ExecContext(ctx, `
				INSERT INTO mailboxes
					(account_id, path, delimiter, name, role, attrs_json, selectable, subscribed)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				accountID, sm.Path, sm.Delimiter, name, sm.Role, attrsJSON, bit(sm.Selectable), bit(sm.Subscribed))
			if err != nil {
				return nil, fmt.Errorf("insert mailbox %q: %w", sm.Path, err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return nil, fmt.Errorf("insert mailbox %q: %w", sm.Path, err)
			}
			if err := t.Emit(ctx, api.MailboxChanged{ID: id, AccountID: accountID}); err != nil {
				return nil, err
			}
			continue
		}
		if cur.delimiter == sm.Delimiter && cur.name == name && cur.role == string(sm.Role) &&
			cur.attrsJSON == attrsJSON && cur.selectable == sm.Selectable && cur.subscribed == sm.Subscribed {
			continue
		}
		_, err = t.ExecContext(ctx, `
			UPDATE mailboxes
			SET delimiter = ?, name = ?, role = ?, attrs_json = ?, selectable = ?, subscribed = ?
			WHERE id = ?`,
			sm.Delimiter, name, sm.Role, attrsJSON, bit(sm.Selectable), bit(sm.Subscribed), cur.id)
		if err != nil {
			return nil, fmt.Errorf("update mailbox %q: %w", sm.Path, err)
		}
		if err := t.Emit(ctx, api.MailboxChanged{ID: cur.id, AccountID: accountID}); err != nil {
			return nil, err
		}
	}
	for _, m := range stored {
		if present[m.path] {
			continue
		}
		if err := t.deleteMailbox(ctx, accountID, m.id); err != nil {
			return nil, err
		}
	}
	return listMailboxes(ctx, t, accountID)
}

// orphanMessageIDs returns the IDs of messages whose only membership is in
// mailboxID, ascending.
func orphanMessageIDs(ctx context.Context, t *Tx, mailboxID int64) ([]int64, error) {
	rows, err := t.QueryContext(ctx, `
		SELECT mm.message_id FROM message_mailbox mm
		WHERE mm.mailbox_id = ?
		  AND NOT EXISTS (SELECT 1 FROM message_mailbox o
		                WHERE o.message_id = mm.message_id AND o.mailbox_id <> ?)
		ORDER BY mm.message_id`, mailboxID, mailboxID)
	if err != nil {
		return nil, fmt.Errorf("find orphan messages: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("find orphan messages: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// deleteMailbox deletes a mailbox and the messages left in no mailbox, with
// their search entries, emitting the mailbox and message events.
func (t *Tx) deleteMailbox(ctx context.Context, accountID, id int64) error {
	orphans, err := orphanMessageIDs(ctx, t, id)
	if err != nil {
		return err
	}
	if _, err := t.ExecContext(ctx, `DELETE FROM mailboxes WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete mailbox %d: %w", id, err)
	}
	for _, mid := range orphans {
		if _, err := t.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, mid); err != nil {
			return fmt.Errorf("delete message %d: %w", mid, err)
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, mid); err != nil {
			return fmt.Errorf("delete search entry %d: %w", mid, err)
		}
	}
	if err := t.Emit(ctx, api.MailboxChanged{ID: id, AccountID: accountID, Deleted: true}); err != nil {
		return err
	}
	if len(orphans) > 0 {
		slices.Sort(orphans)
		if err := t.Emit(ctx, api.MessageRemoved{AccountID: accountID, IDs: orphans}); err != nil {
			return err
		}
	}
	return nil
}

// MailboxSyncState returns a mailbox's stored sync state, with ok false when
// the mailbox has never been synced (no UIDVALIDITY yet). An unknown mailbox
// returns ErrNotFound. Task T-0012 implements it.
func (d *DB) MailboxSyncState(ctx context.Context, mailboxID int64) (SyncState, bool, error) {
	var (
		uidvalidity sql.NullInt64
		uidnext     sql.NullInt64
		modseq      sql.NullInt64
		count       sql.NullInt64
		lastSync    sql.NullString
	)
	err := d.db.QueryRowContext(ctx, `
		SELECT uidvalidity, uidnext, highestmodseq, server_count, last_sync_at
		FROM mailboxes WHERE id = ?`, mailboxID).
		Scan(&uidvalidity, &uidnext, &modseq, &count, &lastSync)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncState{}, false, ErrNotFound
	}
	if err != nil {
		return SyncState{}, false, fmt.Errorf("mailbox sync state: %w", err)
	}
	if !uidvalidity.Valid {
		return SyncState{}, false, nil
	}
	s := SyncState{
		UIDValidity:   uint32(uidvalidity.Int64),
		UIDNext:       uint32(uidnext.Int64),
		HighestModSeq: uint64(modseq.Int64),
		ServerCount:   uint32(count.Int64),
	}
	if lastSync.Valid {
		at, err := ParseTime(lastSync.String)
		if err != nil {
			return SyncState{}, false, err
		}
		s.LastSyncAt = at
	}
	return s, true, nil
}

// SetMailboxSyncState stores every field of s for the mailbox; a zero
// LastSyncAt is stored as NULL. It emits nothing. An unknown mailbox returns
// ErrNotFound. Task T-0012 implements it.
func (t *Tx) SetMailboxSyncState(ctx context.Context, mailboxID int64, s SyncState) error {
	var lastSync any
	if !s.LastSyncAt.IsZero() {
		lastSync = FormatTime(s.LastSyncAt)
	}
	res, err := t.ExecContext(ctx, `
		UPDATE mailboxes
		SET uidvalidity = ?, uidnext = ?, highestmodseq = ?, server_count = ?, last_sync_at = ?
		WHERE id = ?`,
		int64(s.UIDValidity), int64(s.UIDNext), int64(s.HighestModSeq), int64(s.ServerCount), lastSync, mailboxID)
	if err != nil {
		return fmt.Errorf("set mailbox sync state: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set mailbox sync state: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
