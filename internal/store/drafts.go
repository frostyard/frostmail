package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"
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

// draftCols are the drafts columns scanDraft reads, in order.
const draftCols = `id, account_id, kind, source_id, content_json, msgid_hdr, in_reply_to,
	refs_json, server_uid, saved_at, updated_at, created_at`

// draftSelect is the base SELECT for a full draft row.
const draftSelect = `SELECT ` + draftCols + ` FROM drafts`

// CreateDraft stores a new draft with created_at and updated_at set to the
// transaction clock truncated to the millisecond; d.ID, ServerUID and
// SavedAt are ignored. It returns the stored draft as GetDraft reads it,
// with the new ID.
func (t *Tx) CreateDraft(ctx context.Context, d Draft) (Draft, error) {
	now := FormatTime(t.Now().Truncate(time.Millisecond))
	content, err := json.Marshal(d.Content)
	if err != nil {
		return Draft{}, fmt.Errorf("create draft: %w", err)
	}
	refs, err := marshalRefs(d.References)
	if err != nil {
		return Draft{}, fmt.Errorf("create draft: %w", err)
	}
	var id int64
	err = t.QueryRowContext(ctx, `INSERT INTO drafts (account_id, kind, source_id, content_json,
		msgid_hdr, in_reply_to, refs_json, updated_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		d.AccountID, d.Kind, nullInt64(d.SourceID), string(content),
		d.MessageID, d.InReplyTo, refs, now, now).Scan(&id)
	if err != nil {
		return Draft{}, fmt.Errorf("create draft: %w", err)
	}
	return getDraftRow(ctx, t, id)
}

// GetDraft returns one draft with its attachments in insertion order (by
// ID), nil attachments when there are none. A missing draft is an error
// wrapping ErrNotFound.
func (d *DB) GetDraft(ctx context.Context, id int64) (Draft, error) {
	return getDraftRow(ctx, d.db, id)
}

// ListDrafts lists the drafts of an account (every account when accountID
// is 0), newest updated_at first, then higher ID first, each with its
// attachments.
func (d *DB) ListDrafts(ctx context.Context, accountID int64) ([]Draft, error) {
	query := draftSelect
	var args []any
	if accountID != 0 {
		query += ` WHERE account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY updated_at DESC, id DESC`
	drafts, err := queryDrafts(ctx, d.db, query, args...)
	if err != nil {
		return nil, err
	}
	if err := attachDrafts(ctx, d.db, drafts); err != nil {
		return nil, err
	}
	return drafts, nil
}

// UpdateDraftContent replaces a draft's content, sets updated_at to the
// transaction clock and returns the draft as stored. A missing draft is an
// error wrapping ErrNotFound.
func (t *Tx) UpdateDraftContent(ctx context.Context, id int64, c DraftContent) (Draft, error) {
	content, err := json.Marshal(c)
	if err != nil {
		return Draft{}, fmt.Errorf("update draft %d: %w", id, err)
	}
	res, err := t.ExecContext(ctx, `UPDATE drafts SET content_json = ?, updated_at = ? WHERE id = ?`,
		string(content), FormatTime(t.Now().Truncate(time.Millisecond)), id)
	if err != nil {
		return Draft{}, fmt.Errorf("update draft %d: %w", id, err)
	}
	if err := oneRow(res, fmt.Sprintf("update draft %d", id)); err != nil {
		return Draft{}, err
	}
	return getDraftRow(ctx, t, id)
}

// DeleteDraft returns the draft as it was, attachments and server copy
// included, then deletes it; its attachments go with it through the
// foreign key cascade. A missing draft is an error wrapping ErrNotFound.
func (t *Tx) DeleteDraft(ctx context.Context, id int64) (Draft, error) {
	dr, err := getDraftRow(ctx, t, id)
	if err != nil {
		return Draft{}, err
	}
	if _, err := t.ExecContext(ctx, `DELETE FROM drafts WHERE id = ?`, id); err != nil {
		return Draft{}, fmt.Errorf("delete draft %d: %w", id, err)
	}
	return dr, nil
}

// AddDraftAttachment attaches a stored blob to a draft and returns it with
// its new ID.
func (t *Tx) AddDraftAttachment(ctx context.Context, draftID int64, a DraftAttachment) (DraftAttachment, error) {
	var id int64
	err := t.QueryRowContext(ctx, `INSERT INTO draft_attachments (draft_id, blob_id, filename,
		content_type, size) VALUES (?, ?, ?, ?, ?) RETURNING id`,
		draftID, a.BlobID, a.Filename, a.ContentType, a.Size).Scan(&id)
	if err != nil {
		return DraftAttachment{}, fmt.Errorf("add draft attachment: %w", err)
	}
	a.ID = id
	return a, nil
}

// RemoveDraftAttachment deletes an attachment only if it belongs to the
// given draft; otherwise, or when it is missing, it returns an error
// wrapping ErrNotFound.
func (t *Tx) RemoveDraftAttachment(ctx context.Context, draftID, attachmentID int64) error {
	res, err := t.ExecContext(ctx, `DELETE FROM draft_attachments WHERE id = ? AND draft_id = ?`,
		attachmentID, draftID)
	if err != nil {
		return fmt.Errorf("remove draft attachment %d: %w", attachmentID, err)
	}
	if err := oneRow(res, fmt.Sprintf("draft attachment %d", attachmentID)); err != nil {
		return err
	}
	return nil
}

// DraftsToSave lists the account's drafts (every account when accountID is
// 0) whose updated_at is at or before quietBefore and whose server copy is
// missing or older than the content, oldest updated_at first, then lower
// ID, each with its attachments.
func (d *DB) DraftsToSave(ctx context.Context, accountID int64, quietBefore time.Time) ([]Draft, error) {
	query := draftSelect + ` WHERE updated_at <= ? AND (saved_at IS NULL OR saved_at < updated_at)`
	args := []any{FormatTime(quietBefore)}
	if accountID != 0 {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY updated_at ASC, id ASC`
	drafts, err := queryDrafts(ctx, d.db, query, args...)
	if err != nil {
		return nil, err
	}
	if err := attachDrafts(ctx, d.db, drafts); err != nil {
		return nil, err
	}
	return drafts, nil
}

// SetDraftServerCopy records a draft's server copy UID and the time it was
// written. A missing draft is an error wrapping ErrNotFound.
func (t *Tx) SetDraftServerCopy(ctx context.Context, id int64, uid uint32, savedAt time.Time) error {
	res, err := t.ExecContext(ctx, `UPDATE drafts SET server_uid = ?, saved_at = ? WHERE id = ?`,
		uid, FormatTime(savedAt.Truncate(time.Millisecond)), id)
	if err != nil {
		return fmt.Errorf("set draft server copy %d: %w", id, err)
	}
	if err := oneRow(res, fmt.Sprintf("draft %d", id)); err != nil {
		return err
	}
	return nil
}

// getDraftRow reads one draft by ID with its attachments, closing the
// drafts rows before loading them. A missing draft wraps ErrNotFound.
func getDraftRow(ctx context.Context, q querier, id int64) (Draft, error) {
	drafts, err := queryDrafts(ctx, q, draftSelect+` WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return Draft{}, err
	}
	if len(drafts) == 0 {
		return Draft{}, fmt.Errorf("draft %d: %w", id, ErrNotFound)
	}
	dr := drafts[0]
	atts, err := loadDraftAttachments(ctx, q, id)
	if err != nil {
		return Draft{}, err
	}
	dr.Attachments = atts
	return dr, nil
}

// queryDrafts runs a drafts query and returns every row scanned, closing
// the rows before returning.
func queryDrafts(ctx context.Context, q querier, query string, args ...any) ([]Draft, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("drafts: %w", err)
	}
	defer rows.Close()
	var out []Draft
	for rows.Next() {
		dr, err := scanDraft(rows)
		if err != nil {
			return nil, fmt.Errorf("drafts: %w", err)
		}
		out = append(out, dr)
	}
	return out, rows.Err()
}

// attachDrafts loads each draft's attachments, after the drafts rows are
// closed.
func attachDrafts(ctx context.Context, q querier, drafts []Draft) error {
	for i := range drafts {
		atts, err := loadDraftAttachments(ctx, q, drafts[i].ID)
		if err != nil {
			return err
		}
		drafts[i].Attachments = atts
	}
	return nil
}

// loadDraftAttachments returns a draft's attachments in insertion order
// (by ID), nil when there are none.
func loadDraftAttachments(ctx context.Context, q querier, draftID int64) ([]DraftAttachment, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, blob_id, filename, content_type, size
		FROM draft_attachments WHERE draft_id = ? ORDER BY id`, draftID)
	if err != nil {
		return nil, fmt.Errorf("attachments of draft %d: %w", draftID, err)
	}
	defer rows.Close()
	var out []DraftAttachment
	for rows.Next() {
		var a DraftAttachment
		if err := rows.Scan(&a.ID, &a.BlobID, &a.Filename, &a.ContentType, &a.Size); err != nil {
			return nil, fmt.Errorf("attachments of draft %d: %w", draftID, err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// scanDraft scans draftCols from a row into a Draft.
func scanDraft(row rowScanner) (Draft, error) {
	var (
		dr          Draft
		sourceID    sql.NullInt64
		contentJSON string
		refsJSON    string
		serverUID   sql.NullInt64
		savedAt     sql.NullString
		updatedAt   string
		createdAt   string
	)
	if err := row.Scan(&dr.ID, &dr.AccountID, &dr.Kind, &sourceID, &contentJSON,
		&dr.MessageID, &dr.InReplyTo, &refsJSON, &serverUID, &savedAt, &updatedAt, &createdAt); err != nil {
		return Draft{}, err
	}
	dr.SourceID = sourceID.Int64
	dr.ServerUID = uint32(serverUID.Int64)
	if err := json.Unmarshal([]byte(contentJSON), &dr.Content); err != nil {
		return Draft{}, fmt.Errorf("content of draft %d: %w", dr.ID, err)
	}
	refs, err := decodeStrings(refsJSON)
	if err != nil {
		return Draft{}, fmt.Errorf("refs of draft %d: %w", dr.ID, err)
	}
	dr.References = refs
	var perr error
	if dr.UpdatedAt, perr = ParseTime(updatedAt); perr != nil {
		return Draft{}, fmt.Errorf("updated_at of draft %d: %w", dr.ID, perr)
	}
	if dr.CreatedAt, perr = ParseTime(createdAt); perr != nil {
		return Draft{}, fmt.Errorf("created_at of draft %d: %w", dr.ID, perr)
	}
	if savedAt.Valid {
		if dr.SavedAt, perr = ParseTime(savedAt.String); perr != nil {
			return Draft{}, fmt.Errorf("saved_at of draft %d: %w", dr.ID, perr)
		}
	}
	return dr, nil
}

// marshalRefs encodes References as a JSON array, "[]" when empty.
func marshalRefs(refs []string) (string, error) {
	if refs == nil {
		refs = []string{}
	}
	b, err := json.Marshal(refs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// nullInt64 returns nil for 0 so a zero value is stored as NULL.
func nullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// oneRow reports ErrNotFound when a statement affected no rows.
func oneRow(res sql.Result, what string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n == 0 {
		return fmt.Errorf("%s: %w", what, ErrNotFound)
	}
	return nil
}
