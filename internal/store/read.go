package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
)

// ViewFilter selects the messages of a view (api.ViewQuery). Zero fields do
// not filter.
type ViewFilter struct {
	AccountID int64
	MailboxID int64
	Unread    *bool // true: unseen only; false: seen only
	Flagged   *bool
	Role      string // a mailbox role (api.MailboxRole), in any account
	Threads   bool   // one row per thread: its newest matching message
	// Keep lists messages that match even when Unread does not: the rows of
	// an open view, which stay when they are read (or marked unread).
	Keep []int64

	// A search (docs/specs/search.md), as internal/search parses it.
	Match         string    // FTS5 expression every message must match
	Exclude       string    // FTS5 expression no message may match
	HasAttachment *bool     // has:attachment
	After, Before time.Time // arrival in [After, Before)
	Roles         []string  // in: any of these mailbox roles

	// Conditions (ADR-0023), compiled each time the view is computed so
	// relative dates move along; checked with CheckConditions first.
	Conditions *api.Conditions
}

// viewOrder is the order every view lists messages in: newest first by
// internal date, then the Date header, then ID. Servers that stamp a batch
// of appended messages with one arrival time still list them by sent date.
const viewOrder = `m.internal_date DESC, COALESCE(m.date_hdr, '') DESC, m.id DESC`

// ViewIDs returns the IDs of the messages matching f, newest first
// (internal date, then the Date header, then ID; servers that stamp a batch
// of appended messages with one arrival time still list them by sent date). Deleted messages never match; every other
// condition is added only for the filter fields that are set. The
// mailbox filter uses EXISTS, not a JOIN, so a message in several
// mailboxes appears once. Match and Exclude are FTS5 expressions over
// messages_fts; dates bound the arrival (internal) date. Role, and any of
// Roles, keep only messages in a mailbox of that role, in any account.
// Keep's messages pass the Unread condition but every other.
// Threads keeps one row per thread: its newest message among those that
// match every other condition, a message with no thread being its own
// thread. The result keeps the view's order.
func (d *DB) ViewIDs(ctx context.Context, f ViewFilter) ([]int64, error) {
	where, args, err := d.viewWhere(f)
	if err != nil {
		return nil, fmt.Errorf("view ids: %w", err)
	}
	query := `SELECT m.id FROM messages m WHERE ` + where + ` ORDER BY ` + viewOrder
	if f.Threads {
		query = `SELECT id FROM (SELECT m.id AS id, m.internal_date AS ord_internal,` +
			` COALESCE(m.date_hdr, '') AS ord_hdr,` +
			` ROW_NUMBER() OVER (PARTITION BY COALESCE(m.thread_id, -m.id) ORDER BY ` + viewOrder + `) AS rn` +
			` FROM messages m WHERE ` + where + `)` +
			` WHERE rn = 1 ORDER BY ord_internal DESC, ord_hdr DESC, id DESC`
	}
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

// CountView counts the messages f lists, and the unread ones among them;
// messages, not threads (Threads is ignored).
func (d *DB) CountView(ctx context.Context, f ViewFilter) (total, unread int, err error) {
	where, args, err := d.viewWhere(f)
	if err != nil {
		return 0, 0, fmt.Errorf("count view: %w", err)
	}
	err = d.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(m.seen = 0), 0) FROM messages m WHERE `+where, args...).
		Scan(&total, &unread)
	if err != nil {
		return 0, 0, fmt.Errorf("count view: %w", err)
	}
	return total, unread, nil
}

// viewWhere is the condition over messages m that a view filter makes.
func (d *DB) viewWhere(f ViewFilter) (string, []any, error) {
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
	if f.Unread != nil && len(f.Keep) > 0 {
		keep, err := json.Marshal(f.Keep)
		if err != nil {
			return "", nil, err
		}
		conds = append(conds, "(m.seen = ? OR m.id IN (SELECT value FROM json_each(?)))")
		args = append(args, bit(!*f.Unread), string(keep))
	} else if f.Unread != nil {
		conds = append(conds, "m.seen = ?")
		args = append(args, bit(!*f.Unread))
	}
	if f.Flagged != nil {
		conds = append(conds, "m.flagged = ?")
		args = append(args, bit(*f.Flagged))
	}
	if f.Match != "" {
		conds = append(conds, "m.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)")
		args = append(args, f.Match)
	}
	if f.Exclude != "" {
		conds = append(conds, "m.id NOT IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)")
		args = append(args, f.Exclude)
	}
	if f.HasAttachment != nil {
		conds = append(conds, "m.has_attachments = ?")
		args = append(args, bit(*f.HasAttachment))
	}
	if !f.After.IsZero() {
		conds = append(conds, "m.internal_date >= ?")
		args = append(args, FormatTime(f.After))
	}
	if !f.Before.IsZero() {
		conds = append(conds, "m.internal_date < ?")
		args = append(args, FormatTime(f.Before))
	}
	for _, roles := range [][]string{{f.Role}, f.Roles} {
		if len(roles) == 0 || roles[0] == "" {
			continue
		}
		conds = append(conds, `EXISTS (SELECT 1 FROM message_mailbox mm JOIN mailboxes mb ON mb.id = mm.mailbox_id
			WHERE mm.message_id = m.id AND mb.role IN (`+inPlace(len(roles))+`))`)
		for _, r := range roles {
			args = append(args, r)
		}
	}
	if f.Conditions != nil {
		p, err := CompileConditions(*f.Conditions, d.Now(), time.Local)
		if err != nil {
			return "", nil, err
		}
		conds = append(conds, p.SQL)
		args = append(args, p.Args...)
	}
	return strings.Join(conds, " AND "), args, nil
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
	ThreadCount    int64 // messages in the thread; 1 for a message alone
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

// summaryCols are the columns scanSummary reads, in order. The last is the
// message's thread size, at least 1: the thread's msg_count, or 1 when the
// message has no thread or the count has not caught up. It is written
// against the unaliased messages table, as every query using it does.
const summaryCols = `id, account_id, thread_id, subject, from_name, from_addr,
	date_hdr, internal_date, preview, has_attachments, size,
	seen, flagged, answered, forwarded, draft, deleted, flag_color, keywords_json,
	MAX(1, COALESCE((SELECT t.msg_count FROM threads t WHERE t.id = messages.thread_id), 1))`

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
		&s.Flags.Draft, &s.Flags.Deleted, &s.Flags.Color, &keywords, &s.ThreadCount}, extra...)
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

// ThreadMessageIDs returns the IDs of a thread's messages that are not
// deleted, oldest first (internal date, then the Date header, then ID).
// The slice is empty, never nil, when the thread exists but holds no such
// message. A thread with no such ID is ErrNotFound.
func (d *DB) ThreadMessageIDs(ctx context.Context, threadID int64) ([]int64, error) {
	var one int64
	err := d.db.QueryRowContext(ctx, `SELECT 1 FROM threads WHERE id = ?`, threadID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("thread messages %d: %w", threadID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("thread messages %d: %w", threadID, err)
	}
	rows, err := d.db.QueryContext(ctx, `SELECT id FROM messages
		WHERE thread_id = ? AND deleted = 0
		ORDER BY internal_date ASC, COALESCE(date_hdr, '') ASC, id ASC`, threadID)
	if err != nil {
		return nil, fmt.Errorf("thread messages %d: %w", threadID, err)
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("thread messages %d: %w", threadID, err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
