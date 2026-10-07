package store

import (
	"context"
	"database/sql"
	"fmt"
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
}

// ListMailboxes returns mailboxes ordered by account, role (inbox first,
// user folders last) and path. accountID 0 means every account.
func (d *DB) ListMailboxes(ctx context.Context, accountID int64) ([]Mailbox, error) {
	return listMailboxes(ctx, d.db, accountID)
}

// querier is satisfied by *sql.DB and *Tx, so a query helper can run inside
// a write transaction and see its uncommitted rows.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func listMailboxes(ctx context.Context, q querier, accountID int64) ([]Mailbox, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT mb.id, mb.account_id, mb.path, mb.name, mb.delimiter, mb.role,
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
		if err := rows.Scan(&mb.ID, &mb.AccountID, &mb.Path, &mb.Name, &mb.Delimiter, &role, &mb.Total, &mb.Unread); err != nil {
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

// ReplaceMailboxes makes accountID's mailboxes match list, a full LIST
// result, and returns the account's mailboxes afterwards in ListMailboxes
// order, read through t. A path not stored yet is inserted; a stored path has
// its delimiter, name, role, attributes, selectable and subscribed columns
// updated; a stored path missing from list is deleted, together with the
// messages left in no mailbox (and their search entries). Name is the last
// component of Path after Delimiter (the whole path when Delimiter is empty).
// It emits api.MailboxChanged for each mailbox inserted, changed or deleted
// (Deleted: true), and api.MessageRemoved for messages deleted with a
// mailbox. Task T-0012 implements it.
func (t *Tx) ReplaceMailboxes(ctx context.Context, accountID int64, list []ServerMailbox) ([]Mailbox, error) {
	return nil, errNotImplemented
}

// MailboxSyncState returns a mailbox's stored sync state, with ok false when
// the mailbox has never been synced (no UIDVALIDITY yet). An unknown mailbox
// returns ErrNotFound. Task T-0012 implements it.
func (d *DB) MailboxSyncState(ctx context.Context, mailboxID int64) (SyncState, bool, error) {
	return SyncState{}, false, errNotImplemented
}

// SetMailboxSyncState stores every field of s for the mailbox; a zero
// LastSyncAt is stored as NULL. It emits nothing. An unknown mailbox returns
// ErrNotFound. Task T-0012 implements it.
func (t *Tx) SetMailboxSyncState(ctx context.Context, mailboxID int64, s SyncState) error {
	return errNotImplemented
}
