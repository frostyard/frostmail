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

// Op is a queued offline action (docs/design/sync.md, offline actions). Its
// payload is owned by the sync engine.
type Op struct {
	ID        int64
	AccountID int64
	Kind      string // flags, move, expunge
	Payload   []byte
	Attempts  int
}

// QueueOp records an action to replay against the server.
func (t *Tx) QueueOp(ctx context.Context, accountID int64, kind string, payload any) (int64, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("queue %s op: %w", kind, err)
	}
	now := FormatTime(t.Now())
	res, err := t.ExecContext(ctx, `INSERT INTO pending_ops (account_id, kind, payload_json, next_try_at, created_at)
		VALUES (?, ?, ?, ?, ?)`, accountID, kind, string(data), now, now)
	if err != nil {
		return 0, fmt.Errorf("queue %s op: %w", kind, err)
	}
	return res.LastInsertId()
}

// DueOps returns an account's queued actions due by now, oldest first.
func (d *DB) DueOps(ctx context.Context, accountID int64, now time.Time) ([]Op, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id, account_id, kind, payload_json, attempts FROM pending_ops
		WHERE account_id = ? AND state = 'queued' AND next_try_at <= ? ORDER BY id`, accountID, FormatTime(now))
	if err != nil {
		return nil, fmt.Errorf("due ops: %w", err)
	}
	defer rows.Close()
	var out []Op
	for rows.Next() {
		var op Op
		var payload string
		if err := rows.Scan(&op.ID, &op.AccountID, &op.Kind, &payload, &op.Attempts); err != nil {
			return nil, err
		}
		op.Payload = []byte(payload)
		out = append(out, op)
	}
	return out, rows.Err()
}

// DeleteOp removes a replayed action.
func (t *Tx) DeleteOp(ctx context.Context, id int64) error {
	_, err := t.ExecContext(ctx, `DELETE FROM pending_ops WHERE id = ?`, id)
	return err
}

// FailOp marks an action the server refused; it is not retried.
func (t *Tx) FailOp(ctx context.Context, id int64, reason string) error {
	_, err := t.ExecContext(ctx, `UPDATE pending_ops SET state = 'failed', attempts = attempts + 1, last_error = ? WHERE id = ?`, reason, id)
	return err
}

// FlagChange sets or clears flags; nil fields are left alone. A Color above
// 0 also flags the message; clearing Flagged also clears the color.
type FlagChange struct {
	Seen     *bool
	Flagged  *bool
	Answered *bool
	Color    *int
}

// ChangeFlags applies c to messages and returns the IDs whose flags changed.
func (t *Tx) ChangeFlags(ctx context.Context, ids []int64, c FlagChange) ([]int64, error) {
	var changed []int64
	for _, id := range ids {
		var seen, flagged, answered bool
		var color int
		err := t.QueryRowContext(ctx, `SELECT seen, flagged, answered, flag_color FROM messages WHERE id = ?`, id).Scan(&seen, &flagged, &answered, &color)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("message %d: %w", id, ErrNotFound)
		}
		if err != nil {
			return nil, err
		}
		ns, nf, na, nc := seen, flagged, answered, color
		if c.Seen != nil {
			ns = *c.Seen
		}
		if c.Answered != nil {
			na = *c.Answered
		}
		if c.Flagged != nil {
			nf = *c.Flagged
			if nf && nc == 0 {
				nc = 1
			}
		}
		if c.Color != nil {
			nc = *c.Color
			nf = nc > 0
		}
		if !nf {
			nc = 0
		}
		if ns == seen && nf == flagged && na == answered && nc == color {
			continue
		}
		if _, err := t.ExecContext(ctx, `UPDATE messages SET seen = ?, flagged = ?, answered = ?, flag_color = ? WHERE id = ?`,
			bit(ns), bit(nf), bit(na), nc, id); err != nil {
			return nil, err
		}
		changed = append(changed, id)
	}
	return changed, nil
}

// Membership is a message's place in one mailbox.
type Membership struct {
	MessageID   int64
	AccountID   int64
	MailboxID   int64
	MailboxPath string
	Role        string
	UID         uint32 // 0 while a local move is pending
}

// Memberships returns the mailboxes holding each message, in message order.
func (t *Tx) Memberships(ctx context.Context, ids []int64) ([]Membership, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := t.QueryContext(ctx, `SELECT mm.message_id, mb.account_id, mb.id, mb.path, mb.role, COALESCE(mm.uid, 0)
		FROM message_mailbox mm JOIN mailboxes mb ON mb.id = mm.mailbox_id
		WHERE mm.message_id IN (`+strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")+`)
		ORDER BY mm.message_id, mb.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Membership
	for rows.Next() {
		var m Membership
		var uid int64
		if err := rows.Scan(&m.MessageID, &m.AccountID, &m.MailboxID, &m.MailboxPath, &m.Role, &uid); err != nil {
			return nil, err
		}
		m.UID = uint32(uid)
		out = append(out, m)
	}
	return out, rows.Err()
}

// MoveMembership moves a message from one mailbox to another locally,
// leaving its UID unknown until the server reports it.
func (t *Tx) MoveMembership(ctx context.Context, messageID, from, to int64) error {
	_, err := t.ExecContext(ctx, `UPDATE message_mailbox SET mailbox_id = ?, uid = NULL, modseq = NULL, pending = 1
		WHERE message_id = ? AND mailbox_id = ?`, to, messageID, from)
	return err
}

// SetMembership records a message's mailbox and UID once the server
// confirmed a move (uid 0 leaves it unknown), clearing pending.
func (t *Tx) SetMembership(ctx context.Context, messageID, mailboxID int64, uid uint32) error {
	var u any
	if uid != 0 {
		u = int64(uid)
	}
	_, err := t.ExecContext(ctx, `UPDATE message_mailbox SET mailbox_id = ?, uid = ?, pending = 0 WHERE message_id = ? AND pending = 1`,
		mailboxID, u, messageID)
	return err
}

// MarkDeleted hides messages locally until an expunge is confirmed.
func (t *Tx) MarkDeleted(ctx context.Context, ids []int64) error {
	for _, id := range ids {
		if _, err := t.ExecContext(ctx, `UPDATE messages SET deleted = 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// ForgetFlagState makes the next reconcile pass of a mailbox fetch every
// flag, so local state the server refused is replaced by the server's.
func (t *Tx) ForgetFlagState(ctx context.Context, mailboxID int64) error {
	_, err := t.ExecContext(ctx, `UPDATE mailboxes SET highestmodseq = 0, server_count = NULL WHERE id = ?`, mailboxID)
	return err
}
