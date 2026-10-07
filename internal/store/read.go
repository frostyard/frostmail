package store

import (
	"context"
	"time"
)

// ViewFilter selects the messages of a view (api.ViewQuery). Zero fields do
// not filter.
type ViewFilter struct {
	AccountID int64
	MailboxID int64
	Text      string // full-text search terms, as typed (see SearchQuery)
	Unread    *bool  // true: unseen only; false: seen only
	Flagged   *bool
}

// ViewIDs returns the IDs of the messages matching f, newest first. Task
// T-0016 implements it.
func (d *DB) ViewIDs(ctx context.Context, f ViewFilter) ([]int64, error) {
	return nil, errNotImplemented
}

// Summary is a message list row (api.MessageSummary).
type Summary struct {
	ID             int64
	AccountID      int64
	MailboxIDs     []int64
	ThreadID       int64 // 0 until threaded
	Subject        string
	From           Address
	Date           time.Time // the Date header, else the internal date
	Preview        string
	Flags          Flags
	HasAttachments bool
	Size           int64
}

// MessageDetail is everything about a message except its body.
type MessageDetail struct {
	Summary
	To              []Address
	Cc              []Address
	ReplyTo         []Address
	MessageID       string
	InReplyTo       string
	References      []string
	ListID          string
	ListUnsubscribe string
	Parts           []Part
	BlobID          string // the raw message in the blob store; "" until fetched
}

// Location is where to fetch a message from on the server.
type Location struct {
	AccountID   int64
	MailboxID   int64
	MailboxPath string
	UID         uint32
}

// Summaries returns the summaries of ids in the order given. Task T-0017
// implements it.
func (d *DB) Summaries(ctx context.Context, ids []int64) ([]Summary, error) {
	return nil, errNotImplemented
}

// GetMessage returns one message's details. Task T-0017 implements it.
func (d *DB) GetMessage(ctx context.Context, id int64) (MessageDetail, error) {
	return MessageDetail{}, errNotImplemented
}

// MessageLocation returns where a message can be fetched from. Task T-0017
// implements it.
func (d *DB) MessageLocation(ctx context.Context, id int64) (Location, error) {
	return Location{}, errNotImplemented
}

// SetBody records that a message's raw body is stored in the blob store.
// Task T-0017 implements it.
func (t *Tx) SetBody(ctx context.Context, id int64, blobID string) error {
	return errNotImplemented
}
