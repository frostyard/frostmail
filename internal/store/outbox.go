package store

import (
	"context"
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
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// QueueOutbox inserts a queued message. Task T-0039 implements the outbox
// functions; the stubs store nothing.
func (t *Tx) QueueOutbox(_ context.Context, o OutboxItem) (OutboxItem, error) {
	return o, nil
}

// GetOutbox returns one outbox message.
func (d *DB) GetOutbox(_ context.Context, _ int64) (OutboxItem, error) {
	return OutboxItem{}, ErrNotFound
}

// ListOutbox lists unsent messages.
func (d *DB) ListOutbox(_ context.Context, _ int64) ([]OutboxItem, error) {
	return nil, nil
}

// DueOutbox lists queued messages whose time has come.
func (d *DB) DueOutbox(_ context.Context, _ int64, _ time.Time) ([]OutboxItem, error) {
	return nil, nil
}

// OutboxInState lists an account's messages in one state.
func (d *DB) OutboxInState(_ context.Context, _ int64, _ string) ([]OutboxItem, error) {
	return nil, nil
}

// MoveOutbox changes a message's state if it is in the expected one.
func (t *Tx) MoveOutbox(_ context.Context, _ int64, _, _ string) error {
	return ErrNotFound
}

// RetryOutboxLater puts a message that failed to send back in the queue.
func (t *Tx) RetryOutboxLater(_ context.Context, _ int64, _ time.Time, _ string) error {
	return ErrNotFound
}

// FailOutbox marks a message the server refused.
func (t *Tx) FailOutbox(_ context.Context, _ int64, _ string) error {
	return ErrNotFound
}

// RequeueOutbox queues a failed message again.
func (t *Tx) RequeueOutbox(_ context.Context, _ int64, _ time.Time) error {
	return ErrNotFound
}

// CancelOutbox removes a queued message.
func (t *Tx) CancelOutbox(_ context.Context, _ int64) (OutboxItem, error) {
	return OutboxItem{}, ErrNotFound
}
