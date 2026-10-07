package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/frostyard/frostmail/internal/mimex"
)

// Address is one mailbox address.
type Address struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

// Part is one MIME part from BODYSTRUCTURE.
type Part struct {
	Path        string // IMAP part specifier: "1", "1.2"
	ContentType string // lowercase type/subtype
	Charset     string
	Encoding    string // lowercase Content-Transfer-Encoding
	Disposition string // inline, attachment or ""
	Filename    string
	ContentID   string // without angle brackets
	Size        int64
}

// MessageHeader is what a header fetch learns about one message in one
// mailbox (docs/design/sync.md, the reconcile pass).
type MessageHeader struct {
	UID             uint32
	ModSeq          uint64
	Flags           Flags
	InternalDate    time.Time
	Size            int64
	MessageID       string // without angle brackets
	InReplyTo       string
	References      []string
	Subject         string
	From            Address
	To              []Address
	Cc              []Address
	Bcc             []Address
	ReplyTo         []Address
	Date            time.Time // zero when the Date header is missing or invalid
	ListID          string
	ListUnsubscribe string
	AuthResults     string
	HasAttachments  bool
	Preview         string
	Parts           []Part
}

// FlagUpdate is a message's flags as the server reports them.
type FlagUpdate struct {
	UID    uint32
	ModSeq uint64
	Flags  Flags
}

// InsertHeaders stores the messages in hs that mailboxID does not hold yet
// and returns their new IDs in the order of hs. A header whose UID is
// already in the mailbox is skipped, so a reconcile pass can be repeated
// after a crash. Each insert writes a messages row (body_state 'headers',
// thread_id NULL, subject_norm from mimex.NormalizeSubject, addresses and
// references as JSON arrays, date_hdr NULL for a zero Date), its
// message_mailbox row (uid, modseq) and its parts rows. It emits nothing:
// the sync engine threads, indexes and emits message.changed for the batch
// in the same transaction. Task T-0013 implements it.
func (t *Tx) InsertHeaders(ctx context.Context, accountID, mailboxID int64, hs []MessageHeader) ([]int64, error) {
	var ids []int64
	for _, h := range hs {
		var one int
		err := t.QueryRowContext(ctx, `SELECT 1 FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`,
			mailboxID, int64(h.UID)).Scan(&one)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("insert headers uid %d: %w", h.UID, err)
		}
		id, err := t.insertMessage(ctx, accountID, h)
		if err != nil {
			return nil, err
		}
		if _, err := t.ExecContext(ctx,
			`INSERT INTO message_mailbox (message_id, mailbox_id, uid, modseq) VALUES (?, ?, ?, ?)`,
			id, mailboxID, int64(h.UID), int64(h.ModSeq)); err != nil {
			return nil, fmt.Errorf("insert membership uid %d: %w", h.UID, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// insertMessage writes one messages row and its parts rows, returning the
// new message ID. The caller owns the message_mailbox row.
func (t *Tx) insertMessage(ctx context.Context, accountID int64, h MessageHeader) (int64, error) {
	raw := make([]string, 6)
	for i, v := range []any{h.References, h.To, h.Cc, h.Bcc, h.ReplyTo, h.Flags.Keywords} {
		b, err := json.Marshal(v)
		if err != nil {
			return 0, fmt.Errorf("marshal column %d uid %d: %w", i, h.UID, err)
		}
		raw[i] = string(b)
	}
	var dateHdr any
	if !h.Date.IsZero() {
		dateHdr = FormatTime(h.Date)
	}
	res, err := t.ExecContext(ctx, `INSERT INTO messages (
		account_id, msgid_hdr, in_reply_to, refs_json, subject, subject_norm,
		from_name, from_addr, to_json, cc_json, bcc_json, reply_to_json,
		date_hdr, internal_date, size, preview, has_attachments, list_id,
		list_unsubscribe, auth_results, seen, flagged, answered, forwarded,
		draft, deleted, flag_color, keywords_json, body_state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'headers')`,
		accountID, h.MessageID, h.InReplyTo, raw[0], h.Subject, mimex.NormalizeSubject(h.Subject),
		h.From.Name, h.From.Addr, raw[1], raw[2], raw[3], raw[4],
		dateHdr, FormatTime(h.InternalDate), h.Size, h.Preview, bit(h.HasAttachments), h.ListID,
		h.ListUnsubscribe, h.AuthResults,
		bit(h.Flags.Seen), bit(h.Flags.Flagged), bit(h.Flags.Answered), bit(h.Flags.Forwarded),
		bit(h.Flags.Draft), bit(h.Flags.Deleted), int64(h.Flags.Color), raw[5])
	if err != nil {
		return 0, fmt.Errorf("insert headers uid %d: %w", h.UID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert headers uid %d: %w", h.UID, err)
	}
	for _, p := range h.Parts {
		if _, err := t.ExecContext(ctx, `INSERT INTO parts (
			message_id, path, content_type, charset, encoding, disposition, filename, content_id, size)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, p.Path, p.ContentType, p.Charset, p.Encoding, p.Disposition, p.Filename, p.ContentID, p.Size); err != nil {
			return 0, fmt.Errorf("insert part %s uid %d: %w", p.Path, h.UID, err)
		}
	}
	return id, nil
}

// MailboxUIDs returns the UIDs stored for mailboxID in ascending order; none
// is an empty slice. Task T-0013 implements it.
func (d *DB) MailboxUIDs(ctx context.Context, mailboxID int64) ([]uint32, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT uid FROM message_mailbox WHERE mailbox_id = ? AND uid IS NOT NULL ORDER BY uid`, mailboxID)
	if err != nil {
		return nil, fmt.Errorf("mailbox uids: %w", err)
	}
	defer rows.Close()
	uids := []uint32{}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("mailbox uids: %w", err)
		}
		uids = append(uids, uint32(v))
	}
	return uids, rows.Err()
}

// UpdateFlags applies server-reported flags to the messages of mailboxID by
// UID and returns, in the order of ups, the IDs of messages whose stored
// flags changed. Every matched membership gets the update's modseq; UIDs not
// in the mailbox are skipped, and so are the flags of messages a queued
// action covers (QueueOp), whose local state stands until the replay. It
// emits nothing.
func (t *Tx) UpdateFlags(ctx context.Context, mailboxID int64, ups []FlagUpdate) ([]int64, error) {
	var changed []int64
	for _, u := range ups {
		var msgID int64
		err := t.QueryRowContext(ctx, `SELECT message_id FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`,
			mailboxID, int64(u.UID)).Scan(&msgID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("update flags uid %d: %w", u.UID, err)
		}
		if _, err := t.ExecContext(ctx, `UPDATE message_mailbox SET modseq = ? WHERE mailbox_id = ? AND uid = ?`,
			int64(u.ModSeq), mailboxID, int64(u.UID)); err != nil {
			return nil, fmt.Errorf("set modseq uid %d: %w", u.UID, err)
		}
		var queued bool
		if err := t.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pending_op_messages pm
			JOIN pending_ops p ON p.id = pm.op_id WHERE pm.message_id = ? AND p.state != 'failed')`, msgID).Scan(&queued); err != nil {
			return nil, fmt.Errorf("update flags uid %d: %w", u.UID, err)
		}
		if queued {
			continue
		}
		want, err := flagValues(u.Flags)
		if err != nil {
			return nil, fmt.Errorf("update flags uid %d: %w", u.UID, err)
		}
		var stored flagCols
		err = t.QueryRowContext(ctx, `SELECT seen, flagged, answered, forwarded, draft, deleted, flag_color, keywords_json
			FROM messages WHERE id = ?`, msgID).
			Scan(&stored.seen, &stored.flagged, &stored.answered, &stored.forwarded,
				&stored.draft, &stored.deleted, &stored.color, &stored.keywords)
		if err != nil {
			return nil, fmt.Errorf("read flags uid %d: %w", u.UID, err)
		}
		if stored == want {
			continue
		}
		if _, err := t.ExecContext(ctx, `UPDATE messages SET
			seen = ?, flagged = ?, answered = ?, forwarded = ?, draft = ?, deleted = ?, flag_color = ?, keywords_json = ?
			WHERE id = ?`,
			want.seen, want.flagged, want.answered, want.forwarded,
			want.draft, want.deleted, want.color, want.keywords, msgID); err != nil {
			return nil, fmt.Errorf("update flags uid %d: %w", u.UID, err)
		}
		changed = append(changed, msgID)
	}
	return changed, nil
}

// flagCols is a message's flag columns as stored, comparable field by field.
type flagCols struct {
	seen, flagged, answered, forwarded, draft, deleted, color int64
	keywords                                                  string
}

// flagValues renders f as the flag column values to write or compare.
func flagValues(f Flags) (flagCols, error) {
	kb, err := json.Marshal(f.Keywords)
	if err != nil {
		return flagCols{}, err
	}
	return flagCols{
		seen: bit(f.Seen), flagged: bit(f.Flagged), answered: bit(f.Answered),
		forwarded: bit(f.Forwarded), draft: bit(f.Draft), deleted: bit(f.Deleted),
		color: int64(f.Color), keywords: string(kb),
	}, nil
}

// RemoveUIDs removes the memberships of uids in mailboxID, deletes the
// messages left in no mailbox together with their parts and search entries,
// and returns the deleted message IDs in ascending order. UIDs not in the
// mailbox are skipped. It emits nothing. Task T-0013 implements it.
func (t *Tx) RemoveUIDs(ctx context.Context, mailboxID int64, uids []uint32) ([]int64, error) {
	var removed []int64
	for _, uid := range uids {
		var msgID int64
		err := t.QueryRowContext(ctx, `SELECT message_id FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`,
			mailboxID, int64(uid)).Scan(&msgID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("remove uid %d: %w", uid, err)
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`,
			mailboxID, int64(uid)); err != nil {
			return nil, fmt.Errorf("remove membership uid %d: %w", uid, err)
		}
		var left int64
		if err := t.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_mailbox WHERE message_id = ?`, msgID).Scan(&left); err != nil {
			return nil, fmt.Errorf("count mailboxes %d: %w", msgID, err)
		}
		if left > 0 {
			continue
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, msgID); err != nil {
			return nil, fmt.Errorf("delete message %d: %w", msgID, err)
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, msgID); err != nil {
			return nil, fmt.Errorf("delete search entry %d: %w", msgID, err)
		}
		removed = append(removed, msgID)
	}
	slices.Sort(removed)
	return removed, nil
}
