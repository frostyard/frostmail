package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PIMOp is a pim_ops row: a change made here to a contact, event or task,
// waiting to be written to the server (docs/design/pim.md, Writes).
type PIMOp struct {
	ID           int64
	AccountID    int64
	CollectionID int64
	ObjectID     *int64 // nil once a delete removed the object
	Kind         string // put, delete, tasks.insert, tasks.patch, tasks.delete
	Href         string
	IfMatch      string // the ETag the change was made on; "" creates
	Payload      string // the patch as JSON, to apply again after a 412
	State        string // queued, running, failed
	Attempts     int
	NextTryAt    time.Time
	LastError    string
}

// QueuePIMOp queues a change, due at op.NextTryAt or now when that is
// zero, and returns its ID.
func (t *Tx) QueuePIMOp(ctx context.Context, op PIMOp) (int64, error) {
	if op.Payload == "" {
		op.Payload = "{}"
	}
	now := FormatTime(t.Now())
	due := now
	if !op.NextTryAt.IsZero() {
		due = FormatTime(op.NextTryAt)
	}
	res, err := t.ExecContext(ctx, `INSERT INTO pim_ops
		(account_id, collection_id, object_id, kind, href, if_match, payload_json, next_try_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		op.AccountID, op.CollectionID, op.ObjectID, op.Kind, op.Href, op.IfMatch, op.Payload, due, now)
	if err != nil {
		return 0, fmt.Errorf("queue pim op: %w", err)
	}
	return res.LastInsertId()
}

// DuePIMOps returns an account's queued changes that are due at now, oldest
// first.
func (d *DB) DuePIMOps(ctx context.Context, accountID int64, now time.Time) ([]PIMOp, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id, account_id, collection_id, object_id, kind, href, if_match,
		payload_json, state, attempts, next_try_at, last_error
		FROM pim_ops WHERE account_id = ? AND state = 'queued' AND next_try_at <= ? ORDER BY id`,
		accountID, FormatTime(now))
	if err != nil {
		return nil, fmt.Errorf("due pim ops: %w", err)
	}
	defer rows.Close()
	var out []PIMOp
	for rows.Next() {
		var (
			op   PIMOp
			obj  sql.NullInt64
			next string
		)
		if err := rows.Scan(&op.ID, &op.AccountID, &op.CollectionID, &obj, &op.Kind, &op.Href, &op.IfMatch,
			&op.Payload, &op.State, &op.Attempts, &next, &op.LastError); err != nil {
			return nil, fmt.Errorf("due pim ops: %w", err)
		}
		if obj.Valid {
			op.ObjectID = &obj.Int64
		}
		if op.NextTryAt, err = ParseTime(next); err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

// DonePIMOp removes a change the server took.
func (t *Tx) DonePIMOp(ctx context.Context, id int64) error {
	if _, err := t.ExecContext(ctx, `DELETE FROM pim_ops WHERE id = ?`, id); err != nil {
		return fmt.Errorf("done pim op: %w", err)
	}
	return nil
}

// RetryPIMOp counts a failed attempt and waits until next; with next zero
// the change has failed for good and the server's version stands.
func (t *Tx) RetryPIMOp(ctx context.Context, id int64, next time.Time, errText string) error {
	state, nextAt := "queued", FormatTime(next)
	if next.IsZero() {
		state, nextAt = "failed", FormatTime(t.Now())
	}
	_, err := t.ExecContext(ctx, `UPDATE pim_ops SET state = ?, attempts = attempts + 1, next_try_at = ?, last_error = ?
		WHERE id = ?`, state, nextAt, errText, id)
	if err != nil {
		return fmt.Errorf("retry pim op: %w", err)
	}
	return nil
}

// SetObjectETag records the ETag the server gave an object Frostmail wrote.
func (t *Tx) SetObjectETag(ctx context.Context, id int64, etag string) error {
	if _, err := t.ExecContext(ctx, `UPDATE objects SET etag = ? WHERE id = ?`, etag, id); err != nil {
		return fmt.Errorf("set object etag: %w", err)
	}
	return nil
}
