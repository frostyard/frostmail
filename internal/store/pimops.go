package store

import (
	"context"
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

// QueuePIMOp queues a change, due now, and returns its ID.
func (t *Tx) QueuePIMOp(ctx context.Context, op PIMOp) (int64, error) {
	if op.Payload == "" {
		op.Payload = "{}"
	}
	now := FormatTime(t.Now())
	res, err := t.ExecContext(ctx, `INSERT INTO pim_ops
		(account_id, collection_id, object_id, kind, href, if_match, payload_json, next_try_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		op.AccountID, op.CollectionID, op.ObjectID, op.Kind, op.Href, op.IfMatch, op.Payload, now, now)
	if err != nil {
		return 0, fmt.Errorf("queue pim op: %w", err)
	}
	return res.LastInsertId()
}
