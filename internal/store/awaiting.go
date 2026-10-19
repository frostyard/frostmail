package store

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"time"
)

// NewestFilter selects threads by their newest message for the waiting
// queries (thread.awaitingReply and thread.awaitingOthers; ADR-0042).
type NewestFilter struct {
	// Accounts, when not nil, keeps their threads only.
	Accounts []int64
	// After and Until bound the newest message's date: after the one, at
	// or before the other.
	After, Until time.Time
}

// NewestMessage is a thread's newest message, with what the waiting rules
// read of it.
type NewestMessage struct {
	MessageID, AccountID, ThreadID int64
	Date                           time.Time
	From                           Address
	To, Cc                         []Address
	ListID, ListUnsubscribe        string
	Answered                       bool
	Muted                          bool // its thread is muted (ADR-0028)
	InInbox                        bool // it is in a mailbox with the inbox role
}

// eligibleMessage is a message the waiting queries count: not a draft,
// not deleted, and in a mailbox other than Drafts, Trash and Junk.
const eligibleMessage = `m.thread_id IS NOT NULL AND m.deleted = 0 AND m.draft = 0
 AND EXISTS (SELECT 1 FROM message_mailbox mm JOIN mailboxes mb ON mb.id = mm.mailbox_id
  WHERE mm.message_id = m.id AND mb.role NOT IN ('drafts', 'trash', 'junk'))`

// NewestMessages returns, for each thread whose newest eligible message is
// dated within f's bounds, that message, newest first. A message's date is
// its Date header, else its arrival time, as its summary's.
func (d *DB) NewestMessages(ctx context.Context, f NewestFilter) ([]NewestMessage, error) {
	where := eligibleMessage + ` AND COALESCE(m.date_hdr, m.internal_date) > ?`
	args := []any{FormatTime(f.After)}
	if f.Accounts != nil {
		if len(f.Accounts) == 0 {
			return []NewestMessage{}, nil
		}
		where += ` AND m.account_id IN (` + inPlace(len(f.Accounts)) + `)`
		for _, a := range f.Accounts {
			args = append(args, a)
		}
	}
	args = append(args, FormatTime(f.Until))
	// A message dated at or before After cannot be its thread's newest
	// when a later one is in the window, so the window bounds the scan.
	rows, err := d.db.QueryContext(ctx, `WITH ranked AS (
 SELECT m.id, COALESCE(m.date_hdr, m.internal_date) AS d,
  ROW_NUMBER() OVER (PARTITION BY m.thread_id ORDER BY COALESCE(m.date_hdr, m.internal_date) DESC, m.id DESC) AS rn
 FROM messages m WHERE `+where+`)
SELECT m.id, m.account_id, m.thread_id, r.d, m.from_name, m.from_addr, m.to_json, m.cc_json,
 m.list_id, m.list_unsubscribe, m.answered, t.muted,
 EXISTS (SELECT 1 FROM message_mailbox mm JOIN mailboxes mb ON mb.id = mm.mailbox_id
  WHERE mm.message_id = m.id AND mb.role = 'inbox')
FROM ranked r JOIN messages m ON m.id = r.id JOIN threads t ON t.id = m.thread_id
WHERE r.rn = 1 AND r.d <= ? ORDER BY r.d DESC, m.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("newest messages: %w", err)
	}
	defer rows.Close()
	out := []NewestMessage{}
	for rows.Next() {
		var n NewestMessage
		var date, to, cc string
		if err := rows.Scan(&n.MessageID, &n.AccountID, &n.ThreadID, &date, &n.From.Name, &n.From.Addr, &to, &cc,
			&n.ListID, &n.ListUnsubscribe, &n.Answered, &n.Muted, &n.InInbox); err != nil {
			return nil, fmt.Errorf("newest message: %w", err)
		}
		if n.Date, err = ParseTime(date); err != nil {
			return nil, fmt.Errorf("newest message %d: %w", n.MessageID, err)
		}
		if err := json.Unmarshal([]byte(to), &n.To); err != nil {
			return nil, fmt.Errorf("newest message %d to: %w", n.MessageID, err)
		}
		if err := json.Unmarshal([]byte(cc), &n.Cc); err != nil {
			return nil, fmt.Errorf("newest message %d cc: %w", n.MessageID, err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
