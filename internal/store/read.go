package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
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

// ViewIDs returns the IDs of the messages matching f, newest first
// (internal date, then the Date header, then ID; servers that stamp a batch
// of appended messages with one arrival time still list them by sent date). Deleted messages never match; every other
// condition is added only for the filter fields that are set. The
// mailbox filter uses EXISTS, not a JOIN, so a message in several
// mailboxes appears once. Text is turned into an FTS5 MATCH expression
// with SearchQuery; text with nothing searchable adds no condition.
func (d *DB) ViewIDs(ctx context.Context, f ViewFilter) ([]int64, error) {
	conds := []string{"m.deleted = 0"}
	var args []any
	if f.AccountID != 0 {
		conds = append(conds, "m.account_id = ?")
		args = append(args, f.AccountID)
	}
	if f.MailboxID != 0 {
		conds = append(conds, `EXISTS (SELECT 1 FROM message_mailbox mm WHERE mm.message_id = m.id AND mm.mailbox_id = ?)`)
		args = append(args, f.MailboxID)
	}
	if f.Unread != nil {
		conds = append(conds, "m.seen = ?")
		args = append(args, bit(!*f.Unread))
	}
	if f.Flagged != nil {
		conds = append(conds, "m.flagged = ?")
		args = append(args, bit(*f.Flagged))
	}
	if q := SearchQuery(f.Text); q != "" {
		conds = append(conds, "m.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)")
		args = append(args, q)
	}
	query := `SELECT m.id FROM messages m WHERE ` + strings.Join(conds, " AND ") +
		` ORDER BY m.internal_date DESC, COALESCE(m.date_hdr, '') DESC, m.id DESC`
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("view ids: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("view ids: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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

// summaryCols are the columns scanSummary reads, in order.
const summaryCols = `id, account_id, thread_id, subject, from_name, from_addr,
	date_hdr, internal_date, preview, has_attachments, size,
	seen, flagged, answered, forwarded, draft, deleted, flag_color, keywords_json`

// rowScanner is satisfied by *sql.Rows and *sql.Row.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanSummary scans summaryCols from row, plus extra destinations for any
// columns the caller added after them. thread_id NULL reads as 0; date_hdr
// NULL falls back to internal_date, both through ParseTime; Keywords is
// nil when the stored JSON array is empty.
func scanSummary(row rowScanner, extra ...any) (Summary, error) {
	var (
		s        Summary
		threadID sql.NullInt64
		dateHdr  sql.NullString
		internal string
		keywords string
	)
	dest := append([]any{&s.ID, &s.AccountID, &threadID, &s.Subject, &s.From.Name, &s.From.Addr,
		&dateHdr, &internal, &s.Preview, &s.HasAttachments, &s.Size,
		&s.Flags.Seen, &s.Flags.Flagged, &s.Flags.Answered, &s.Flags.Forwarded,
		&s.Flags.Draft, &s.Flags.Deleted, &s.Flags.Color, &keywords}, extra...)
	if err := row.Scan(dest...); err != nil {
		return Summary{}, err
	}
	s.ThreadID = threadID.Int64
	dateStr := internal
	if dateHdr.Valid {
		dateStr = dateHdr.String
	}
	date, err := ParseTime(dateStr)
	if err != nil {
		return Summary{}, err
	}
	s.Date = date
	kw, err := decodeStrings(keywords)
	if err != nil {
		return Summary{}, fmt.Errorf("keywords of message %d: %w", s.ID, err)
	}
	s.Flags.Keywords = kw
	return s, nil
}

// decodeStrings decodes a stored JSON array, returning nil when it is
// empty so callers keep nil fields.
func decodeStrings(col string) ([]string, error) {
	var out []string
	if err := json.Unmarshal([]byte(col), &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// decodeAddresses decodes a stored JSON address array, returning nil when
// it is empty.
func decodeAddresses(col string) ([]Address, error) {
	var out []Address
	if err := json.Unmarshal([]byte(col), &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// inPlace builds the "?, ?, …" placeholder list for n values.
func inPlace(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// Summaries returns the summaries of ids in the order given, skipping IDs
// that do not exist. Each summary carries the message's mailbox IDs in
// ascending order. No IDs returns an empty slice without querying.
func (d *DB) Summaries(ctx context.Context, ids []int64) ([]Summary, error) {
	if len(ids) == 0 {
		return []Summary{}, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	placeholders := inPlace(len(ids))
	rows, err := d.db.QueryContext(ctx, `SELECT `+summaryCols+` FROM messages WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("summaries: %w", err)
	}
	defer rows.Close()
	byID := make(map[int64]Summary, len(ids))
	for rows.Next() {
		s, err := scanSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("summaries: %w", err)
		}
		byID[s.ID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("summaries: %w", err)
	}
	mrows, err := d.db.QueryContext(ctx, `SELECT message_id, mailbox_id FROM message_mailbox
		WHERE message_id IN (`+placeholders+`) ORDER BY message_id, mailbox_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("summary mailboxes: %w", err)
	}
	defer mrows.Close()
	for mrows.Next() {
		var msgID, mailboxID int64
		if err := mrows.Scan(&msgID, &mailboxID); err != nil {
			return nil, fmt.Errorf("summary mailboxes: %w", err)
		}
		s := byID[msgID]
		s.MailboxIDs = append(s.MailboxIDs, mailboxID)
		byID[msgID] = s
	}
	if err := mrows.Err(); err != nil {
		return nil, fmt.Errorf("summary mailboxes: %w", err)
	}
	out := make([]Summary, 0, len(ids))
	for _, id := range ids {
		if s, ok := byID[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// GetMessage returns one message's details: its summary plus its address
// lists (nil when empty), header identifiers, list headers, blob ID (""
// when NULL) and parts ordered by path (nil when none). A missing row is
// ErrNotFound.
func (d *DB) GetMessage(ctx context.Context, id int64) (MessageDetail, error) {
	var (
		m                   MessageDetail
		toJSON, ccJSON      string
		replyJSON, refsJSON string
		blobSHA             sql.NullString
	)
	r := d.db.QueryRowContext(ctx, `SELECT `+summaryCols+`,
		to_json, cc_json, reply_to_json, msgid_hdr, in_reply_to, refs_json,
		list_id, list_unsubscribe, blob_sha
		FROM messages WHERE id = ?`, id)
	s, err := scanSummary(r, &toJSON, &ccJSON, &replyJSON, &m.MessageID, &m.InReplyTo,
		&refsJSON, &m.ListID, &m.ListUnsubscribe, &blobSHA)
	if errors.Is(err, sql.ErrNoRows) {
		return MessageDetail{}, ErrNotFound
	}
	if err != nil {
		return MessageDetail{}, fmt.Errorf("get message %d: %w", id, err)
	}
	m.Summary = s
	m.BlobID = blobSHA.String
	mbs, err := d.mailboxIDsOf(ctx, id)
	if err != nil {
		return MessageDetail{}, err
	}
	m.MailboxIDs = mbs
	if m.To, err = decodeAddresses(toJSON); err != nil {
		return MessageDetail{}, fmt.Errorf("to of message %d: %w", id, err)
	}
	if m.Cc, err = decodeAddresses(ccJSON); err != nil {
		return MessageDetail{}, fmt.Errorf("cc of message %d: %w", id, err)
	}
	if m.ReplyTo, err = decodeAddresses(replyJSON); err != nil {
		return MessageDetail{}, fmt.Errorf("reply-to of message %d: %w", id, err)
	}
	if m.References, err = decodeStrings(refsJSON); err != nil {
		return MessageDetail{}, fmt.Errorf("refs of message %d: %w", id, err)
	}
	parts, err := d.messageParts(ctx, id)
	if err != nil {
		return MessageDetail{}, err
	}
	m.Parts = parts
	return m, nil
}

// mailboxIDsOf returns the IDs of the mailboxes holding a message,
// ascending.
func (d *DB) mailboxIDsOf(ctx context.Context, id int64) ([]int64, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT mailbox_id FROM message_mailbox WHERE message_id = ? ORDER BY mailbox_id`, id)
	if err != nil {
		return nil, fmt.Errorf("mailboxes of message %d: %w", id, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var mbID int64
		if err := rows.Scan(&mbID); err != nil {
			return nil, fmt.Errorf("mailboxes of message %d: %w", id, err)
		}
		ids = append(ids, mbID)
	}
	return ids, rows.Err()
}

// messageParts returns a message's parts ordered by path, nil when none.
func (d *DB) messageParts(ctx context.Context, id int64) ([]Part, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT path, content_type, charset, encoding,
		disposition, filename, content_id, size FROM parts WHERE message_id = ? ORDER BY path`, id)
	if err != nil {
		return nil, fmt.Errorf("parts of message %d: %w", id, err)
	}
	defer rows.Close()
	var parts []Part
	for rows.Next() {
		var p Part
		if err := rows.Scan(&p.Path, &p.ContentType, &p.Charset, &p.Encoding,
			&p.Disposition, &p.Filename, &p.ContentID, &p.Size); err != nil {
			return nil, fmt.Errorf("parts of message %d: %w", id, err)
		}
		parts = append(parts, p)
	}
	return parts, rows.Err()
}

// MessageLocation returns where a message can be fetched from: the
// membership with a non-NULL uid and the lowest mailbox ID, joined to its
// mailbox path. None is ErrNotFound.
func (d *DB) MessageLocation(ctx context.Context, id int64) (Location, error) {
	var loc Location
	err := d.db.QueryRowContext(ctx, `SELECT b.account_id, mm.mailbox_id, b.path, mm.uid
		FROM message_mailbox mm JOIN mailboxes b ON b.id = mm.mailbox_id
		WHERE mm.message_id = ? AND mm.uid IS NOT NULL
		ORDER BY mm.mailbox_id LIMIT 1`, id).
		Scan(&loc.AccountID, &loc.MailboxID, &loc.MailboxPath, &loc.UID)
	if errors.Is(err, sql.ErrNoRows) {
		return Location{}, ErrNotFound
	}
	if err != nil {
		return Location{}, fmt.Errorf("message location %d: %w", id, err)
	}
	return loc, nil
}

// SetBody records that a message's raw body is stored in the blob store
// under blobID, moving body_state to 'full'. A missing message is
// ErrNotFound.
func (t *Tx) SetBody(ctx context.Context, id int64, blobID string) error {
	res, err := t.ExecContext(ctx, `UPDATE messages SET blob_sha = ?, body_state = 'full' WHERE id = ?`, blobID, id)
	if err != nil {
		return fmt.Errorf("set body %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set body %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
