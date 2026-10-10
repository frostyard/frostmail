package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"time"
)

// OutboxItem is a message on its way out (docs/design/send.md, Outbox).
type OutboxItem struct {
	ID         int64
	AccountID  int64
	DraftID    int64  // the draft it came from; 0 once the draft is gone
	State      string // queued, sending, accepted, sent, failed
	SendAt     time.Time
	BlobID     string   // the built message
	MessageID  string   // without angle brackets
	From       string   // the envelope sender
	Recipients []string // the envelope recipients: To, Cc and Bcc
	Subject    string
	To         []Address
	Attempts   int
	LastError  string
	Scheduled  bool // Send Later: due at a chosen time, not in its undo window
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// outboxCols are the outbox columns scanOutbox reads, in order.
const outboxCols = `id, account_id, draft_id, state, send_at, blob_id, msgid_hdr, from_addr,
	rcpt_json, subject, to_json, attempts, last_error, scheduled, created_at, updated_at`

// outboxSelect is the base SELECT for a full outbox row.
const outboxSelect = `SELECT ` + outboxCols + ` FROM outbox`

// QueueOutbox inserts a queued message with state queued, attempts 0 and an
// empty last_error; created_at and updated_at are the transaction clock.
// o.ID, State, Attempts, LastError and the times are ignored; Scheduled
// marks a Send Later row. It returns
// the row as GetOutbox reads it, with the new ID.
func (t *Tx) QueueOutbox(ctx context.Context, o OutboxItem) (OutboxItem, error) {
	if o.Recipients == nil {
		o.Recipients = []string{}
	}
	if o.To == nil {
		o.To = []Address{}
	}
	rcpt, err := json.Marshal(o.Recipients)
	if err != nil {
		return OutboxItem{}, fmt.Errorf("queue outbox: %w", err)
	}
	to, err := json.Marshal(o.To)
	if err != nil {
		return OutboxItem{}, fmt.Errorf("queue outbox: %w", err)
	}
	now := FormatTime(t.Now())
	var id int64
	err = t.QueryRowContext(ctx, `INSERT INTO outbox (account_id, draft_id, state, send_at, blob_id,
		msgid_hdr, from_addr, rcpt_json, subject, to_json, attempts, last_error, scheduled, created_at, updated_at)
		VALUES (?, ?, 'queued', ?, ?, ?, ?, ?, ?, ?, 0, '', ?, ?, ?) RETURNING id`,
		o.AccountID, nullInt64(o.DraftID), FormatTime(o.SendAt), o.BlobID, o.MessageID, o.From,
		string(rcpt), o.Subject, string(to), bit(o.Scheduled), now, now).Scan(&id)
	if err != nil {
		return OutboxItem{}, fmt.Errorf("queue outbox: %w", err)
	}
	return getOutboxRow(ctx, t, id)
}

// GetOutbox returns one outbox message. A missing message is an error
// wrapping ErrNotFound.
func (d *DB) GetOutbox(ctx context.Context, id int64) (OutboxItem, error) {
	return getOutboxRow(ctx, d.db, id)
}

// ListOutbox lists the unsent messages (state other than sent) of an
// account (every account when accountID is 0), by ID.
func (d *DB) ListOutbox(ctx context.Context, accountID int64) ([]OutboxItem, error) {
	query := outboxSelect + ` WHERE state != 'sent'`
	var args []any
	if accountID != 0 {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY id`
	return queryOutbox(ctx, d.db, query, args...)
}

// DueOutbox lists the account's queued messages with send_at at or before
// now (every account when accountID is 0), by send_at, then ID.
func (d *DB) DueOutbox(ctx context.Context, accountID int64, now time.Time) ([]OutboxItem, error) {
	query := outboxSelect + ` WHERE state = 'queued' AND send_at <= ?`
	args := []any{FormatTime(now)}
	if accountID != 0 {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY send_at, id`
	return queryOutbox(ctx, d.db, query, args...)
}

// OutboxInState lists an account's messages (every account when
// accountID is 0) in one state, by ID.
func (d *DB) OutboxInState(ctx context.Context, accountID int64, state string) ([]OutboxItem, error) {
	query := outboxSelect + ` WHERE state = ?`
	args := []any{state}
	if accountID != 0 {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY id`
	return queryOutbox(ctx, d.db, query, args...)
}

// MoveOutbox changes a message's state from one to another only if it is
// in the expected one, setting updated_at to the transaction clock. A
// missing message is an error wrapping ErrNotFound; a message in another
// state, one wrapping ErrConflict.
func (t *Tx) MoveOutbox(ctx context.Context, id int64, from, to string) error {
	return casOutbox(ctx, t, id,
		`UPDATE outbox SET state = ?, updated_at = ? WHERE id = ? AND state = ?`,
		to, FormatTime(t.Now()), id, from)
}

// RetryOutboxLater puts a message that failed to send back in the queue
// at sendAt, incrementing attempts and recording reason, only if it is
// sending. A missing message is an error wrapping ErrNotFound; a message
// in another state, one wrapping ErrConflict.
func (t *Tx) RetryOutboxLater(ctx context.Context, id int64, sendAt time.Time, reason string) error {
	return casOutbox(ctx, t, id,
		`UPDATE outbox SET state = 'queued', send_at = ?, attempts = attempts + 1,
			last_error = ?, updated_at = ? WHERE id = ? AND state = 'sending'`,
		FormatTime(sendAt), reason, FormatTime(t.Now()), id)
}

// FailOutbox marks a message the server refused, incrementing attempts
// and recording reason, only if it is sending. A missing message is an
// error wrapping ErrNotFound; a message in another state, one wrapping
// ErrConflict.
func (t *Tx) FailOutbox(ctx context.Context, id int64, reason string) error {
	return casOutbox(ctx, t, id,
		`UPDATE outbox SET state = 'failed', attempts = attempts + 1, last_error = ?, updated_at = ?
			WHERE id = ? AND state = 'sending'`,
		reason, FormatTime(t.Now()), id)
}

// RequeueOutbox queues a failed message again at sendAt, keeping its
// attempts and last error, only if it is failed. A missing message is an
// error wrapping ErrNotFound; a message in another state, one wrapping
// ErrConflict.
func (t *Tx) RequeueOutbox(ctx context.Context, id int64, sendAt time.Time) error {
	return casOutbox(ctx, t, id,
		`UPDATE outbox SET state = 'queued', send_at = ?, updated_at = ?
			WHERE id = ? AND state = 'failed'`,
		FormatTime(sendAt), FormatTime(t.Now()), id)
}

// CancelOutbox returns a queued message as it was and deletes it. A
// missing message is an error wrapping ErrNotFound; a message in another
// state, one wrapping ErrConflict.
func (t *Tx) CancelOutbox(ctx context.Context, id int64) (OutboxItem, error) {
	o, err := getOutboxRow(ctx, t, id)
	if err != nil {
		return OutboxItem{}, err
	}
	if o.State != "queued" {
		return OutboxItem{}, fmt.Errorf("cancel outbox %d: %w", id, ErrConflict)
	}
	if _, err := t.ExecContext(ctx, `DELETE FROM outbox WHERE id = ? AND state = 'queued'`, id); err != nil {
		return OutboxItem{}, fmt.Errorf("cancel outbox %d: %w", id, err)
	}
	return o, nil
}

// casOutbox runs a compare-and-set UPDATE whose WHERE clause pins the
// expected state. When it changes no row, a second query tells a missing
// message (ErrNotFound) from one in another state (ErrConflict).
func casOutbox(ctx context.Context, t *Tx, id int64, query string, args ...any) error {
	res, err := t.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("outbox %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("outbox %d: %w", id, err)
	}
	if n > 0 {
		return nil
	}
	_, err = getOutboxRow(ctx, t, id)
	if err != nil {
		return err
	}
	return fmt.Errorf("outbox %d: %w", id, ErrConflict)
}

// getOutboxRow reads one outbox row by ID. A missing row wraps
// ErrNotFound.
func getOutboxRow(ctx context.Context, q querier, id int64) (OutboxItem, error) {
	items, err := queryOutbox(ctx, q, outboxSelect+` WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return OutboxItem{}, err
	}
	if len(items) == 0 {
		return OutboxItem{}, fmt.Errorf("outbox %d: %w", id, ErrNotFound)
	}
	return items[0], nil
}

// queryOutbox runs an outbox query and returns every row scanned,
// closing the rows before returning.
func queryOutbox(ctx context.Context, q querier, query string, args ...any) ([]OutboxItem, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("outbox: %w", err)
	}
	defer rows.Close()
	var out []OutboxItem
	for rows.Next() {
		o, err := scanOutbox(rows)
		if err != nil {
			return nil, fmt.Errorf("outbox: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// scanOutbox scans outboxCols from a row into an OutboxItem. A NULL
// draft_id reads as 0.
func scanOutbox(row rowScanner) (OutboxItem, error) {
	var (
		o         OutboxItem
		draftID   sql.NullInt64
		sendAt    string
		rcptJSON  string
		toJSON    string
		createdAt string
		updatedAt string
	)
	if err := row.Scan(&o.ID, &o.AccountID, &draftID, &o.State, &sendAt, &o.BlobID,
		&o.MessageID, &o.From, &rcptJSON, &o.Subject, &toJSON, &o.Attempts, &o.LastError,
		&o.Scheduled, &createdAt, &updatedAt); err != nil {
		return OutboxItem{}, err
	}
	o.DraftID = draftID.Int64
	if err := json.Unmarshal([]byte(rcptJSON), &o.Recipients); err != nil {
		return OutboxItem{}, fmt.Errorf("recipients of outbox %d: %w", o.ID, err)
	}
	if err := json.Unmarshal([]byte(toJSON), &o.To); err != nil {
		return OutboxItem{}, fmt.Errorf("to of outbox %d: %w", o.ID, err)
	}
	var perr error
	if o.SendAt, perr = ParseTime(sendAt); perr != nil {
		return OutboxItem{}, fmt.Errorf("send_at of outbox %d: %w", o.ID, perr)
	}
	if o.CreatedAt, perr = ParseTime(createdAt); perr != nil {
		return OutboxItem{}, fmt.Errorf("created_at of outbox %d: %w", o.ID, perr)
	}
	if o.UpdatedAt, perr = ParseTime(updatedAt); perr != nil {
		return OutboxItem{}, fmt.Errorf("updated_at of outbox %d: %w", o.ID, perr)
	}
	return o, nil
}
