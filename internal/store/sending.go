package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/frostyard/frostmail/api"
)

// Queries the draft saver, the outbox worker and the draft domain share
// (docs/design/send.md).

// bumpUpdatedAt sets a draft's updated_at to now (bound twice), or 1 ms
// past its current value when now is not later: times have millisecond
// resolution, and a change must always be newer than the server copy saved
// before it (saved_at), even within the same millisecond.
const bumpUpdatedAt = `updated_at = CASE WHEN ? > updated_at THEN ?
	ELSE strftime('%Y-%m-%dT%H:%M:%fZ', updated_at, '+0.001 seconds') END`

// TouchDraft sets a draft's updated_at to the transaction clock, so its
// server copy is saved again (its attachments changed). A missing draft
// wraps ErrNotFound.
func (t *Tx) TouchDraft(ctx context.Context, id int64) error {
	now := FormatTime(t.Now().Truncate(time.Millisecond))
	res, err := t.ExecContext(ctx, `UPDATE drafts SET `+bumpUpdatedAt+` WHERE id = ?`, now, now, id)
	if err != nil {
		return fmt.Errorf("touch draft %d: %w", id, err)
	}
	return oneRow(res, fmt.Sprintf("draft %d", id))
}

// GetDraft is DB.GetDraft inside the transaction.
func (t *Tx) GetDraft(ctx context.Context, id int64) (Draft, error) {
	return getDraftRow(ctx, t, id)
}

// GetOutbox is DB.GetOutbox inside the transaction.
func (t *Tx) GetOutbox(ctx context.Context, id int64) (OutboxItem, error) {
	return getOutboxRow(ctx, t, id)
}

// DraftByMessageID returns the account's draft with that Message-ID; a
// missing one wraps ErrNotFound.
func (d *DB) DraftByMessageID(ctx context.Context, accountID int64, msgid string) (Draft, error) {
	var id int64
	err := d.db.QueryRowContext(ctx, `SELECT id FROM drafts WHERE account_id = ? AND msgid_hdr = ? ORDER BY id LIMIT 1`,
		accountID, msgid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, fmt.Errorf("draft with Message-ID %s: %w", msgid, ErrNotFound)
	}
	if err != nil {
		return Draft{}, fmt.Errorf("draft with Message-ID %s: %w", msgid, err)
	}
	return getDraftRow(ctx, d.db, id)
}

// NextDraftSave returns the updated_at of the account's oldest draft whose
// server copy is missing or older than its content; ok is false when every
// copy is current. The saver waits until that time plus its quiet period.
func (d *DB) NextDraftSave(ctx context.Context, accountID int64) (at time.Time, ok bool, err error) {
	return minTime(ctx, d.db, `SELECT MIN(updated_at) FROM drafts
		WHERE account_id = ? AND (saved_at IS NULL OR saved_at < updated_at)`, accountID)
}

// NextOutboxDue returns the earliest send_at of the account's queued
// messages; ok is false when none is queued.
func (d *DB) NextOutboxDue(ctx context.Context, accountID int64) (at time.Time, ok bool, err error) {
	return minTime(ctx, d.db, `SELECT MIN(send_at) FROM outbox WHERE account_id = ? AND state = 'queued'`, accountID)
}

func minTime(ctx context.Context, db *sql.DB, query string, args ...any) (time.Time, bool, error) {
	var s sql.NullString
	if err := db.QueryRowContext(ctx, query, args...).Scan(&s); err != nil {
		return time.Time{}, false, err
	}
	if !s.Valid {
		return time.Time{}, false, nil
	}
	at, err := ParseTime(s.String)
	if err != nil {
		return time.Time{}, false, err
	}
	return at, true, nil
}

// OutboxOfDraft lists the outbox rows made from a draft, by ID.
func (t *Tx) OutboxOfDraft(ctx context.Context, draftID int64) ([]OutboxItem, error) {
	return queryOutbox(ctx, t, outboxSelect+` WHERE draft_id = ? ORDER BY id`, draftID)
}

// DeleteOutbox removes an outbox row whatever its state (a failed send that
// a new send of its draft replaces). A missing row wraps ErrNotFound.
func (t *Tx) DeleteOutbox(ctx context.Context, id int64) error {
	res, err := t.ExecContext(ctx, `DELETE FROM outbox WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete outbox %d: %w", id, err)
	}
	return oneRow(res, fmt.Sprintf("outbox %d", id))
}

// MailboxByRole returns the account's first mailbox with a role; a missing
// one wraps ErrNotFound.
func (d *DB) MailboxByRole(ctx context.Context, accountID int64, role api.MailboxRole) (Mailbox, error) {
	return mailboxByRole(ctx, d.db, accountID, role)
}

// MailboxByRole is DB.MailboxByRole inside the transaction.
func (t *Tx) MailboxByRole(ctx context.Context, accountID int64, role api.MailboxRole) (Mailbox, error) {
	return mailboxByRole(ctx, t, accountID, role)
}

func mailboxByRole(ctx context.Context, q querier, accountID int64, role api.MailboxRole) (Mailbox, error) {
	mbs, err := listMailboxes(ctx, q, accountID)
	if err != nil {
		return Mailbox{}, err
	}
	for _, mb := range mbs {
		if mb.Role == role {
			return mb, nil
		}
	}
	return Mailbox{}, fmt.Errorf("%s mailbox of account %d: %w", role, accountID, ErrNotFound)
}

// HasMessageIn reports whether a mailbox holds a message with that
// Message-ID (without angle brackets).
func (d *DB) HasMessageIn(ctx context.Context, mailboxID int64, msgid string) (bool, error) {
	var n int
	err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages m
		JOIN message_mailbox mm ON mm.message_id = m.id
		WHERE mm.mailbox_id = ? AND m.msgid_hdr = ?`, mailboxID, msgid).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("message %s in mailbox %d: %w", msgid, mailboxID, err)
	}
	return n > 0, nil
}

// MessageAtUID returns the ID of the message a mailbox holds at a UID; ok
// is false when there is none.
func (d *DB) MessageAtUID(ctx context.Context, mailboxID int64, uid uint32) (id int64, ok bool, err error) {
	err = d.db.QueryRowContext(ctx, `SELECT message_id FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`,
		mailboxID, uid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("message at UID %d of mailbox %d: %w", uid, mailboxID, err)
	}
	return id, true, nil
}
