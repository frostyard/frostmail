package store

import (
	"context"
	"time"
)

// DraftContent is what a compose window edits (api.DraftContent).
type DraftContent struct {
	IdentityID int64     `json:"identityId"`
	To         []Address `json:"to"`
	Cc         []Address `json:"cc"`
	Bcc        []Address `json:"bcc"`
	Subject    string    `json:"subject"`
	HTML       string    `json:"html"`
}

// DraftAttachment is a file attached to a draft, kept in the blob store.
type DraftAttachment struct {
	ID          int64
	BlobID      string
	Filename    string
	ContentType string
	Size        int64
}

// Draft is a message being written (docs/design/send.md, Drafts).
type Draft struct {
	ID          int64
	AccountID   int64
	Kind        string // new, reply, replyall, forward
	SourceID    int64  // the message replied to or forwarded; 0 for none
	Content     DraftContent
	Attachments []DraftAttachment // in the order they were added
	MessageID   string            // without angle brackets; the sent message keeps it
	InReplyTo   string
	References  []string
	ServerUID   uint32    // the copy in the Drafts mailbox; 0 before the first save
	SavedAt     time.Time // when that copy was written; zero before the first save
	UpdatedAt   time.Time
	CreatedAt   time.Time
}

// CreateDraft stores a new draft. Task T-0037 implements it; the stub stores
// nothing.
func (t *Tx) CreateDraft(_ context.Context, d Draft) (Draft, error) {
	return d, nil
}

// GetDraft returns one draft. Task T-0037 implements it.
func (d *DB) GetDraft(_ context.Context, _ int64) (Draft, error) {
	return Draft{}, ErrNotFound
}

// ListDrafts lists drafts. Task T-0037 implements it.
func (d *DB) ListDrafts(_ context.Context, _ int64) ([]Draft, error) {
	return nil, nil
}

// UpdateDraftContent replaces a draft's content. Task T-0037 implements it.
func (t *Tx) UpdateDraftContent(_ context.Context, _ int64, _ DraftContent) (Draft, error) {
	return Draft{}, ErrNotFound
}

// DeleteDraft deletes a draft. Task T-0037 implements it.
func (t *Tx) DeleteDraft(_ context.Context, _ int64) (Draft, error) {
	return Draft{}, ErrNotFound
}

// AddDraftAttachment attaches a stored blob to a draft. Task T-0037
// implements it.
func (t *Tx) AddDraftAttachment(_ context.Context, _ int64, a DraftAttachment) (DraftAttachment, error) {
	return a, nil
}

// RemoveDraftAttachment removes an attachment from a draft. Task T-0037
// implements it.
func (t *Tx) RemoveDraftAttachment(_ context.Context, _, _ int64) error {
	return ErrNotFound
}

// DraftsToSave lists drafts whose server copy is out of date. Task T-0037
// implements it.
func (d *DB) DraftsToSave(_ context.Context, _ int64, _ time.Time) ([]Draft, error) {
	return nil, nil
}

// SetDraftServerCopy records a draft's server copy. Task T-0037 implements it.
func (t *Tx) SetDraftServerCopy(_ context.Context, _ int64, _ uint32, _ time.Time) error {
	return ErrNotFound
}
