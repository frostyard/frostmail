package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// Gmail as one store (ADR-0012): messages of All Mail, Spam and Trash are
// stored once per X-GM-MSGID with a UID membership in the folder that holds
// them; every other mailbox is a label whose memberships (without UIDs)
// mirror X-GM-LABELS.

// MarkGmailLabels marks every mailbox of a Gmail account as a label except
// All Mail, Spam and Trash, which are synced.
func (t *Tx) MarkGmailLabels(ctx context.Context, accountID int64) error {
	_, err := t.ExecContext(ctx, `UPDATE mailboxes
		SET is_gmail_label = CASE WHEN role IN ('all', 'junk', 'trash') THEN 0 ELSE 1 END
		WHERE account_id = ?`, accountID)
	if err != nil {
		return fmt.Errorf("mark gmail labels: %w", err)
	}
	return nil
}

// GmailLabels maps X-GM-LABELS names to an account's label mailboxes:
// \Inbox, \Sent, \Draft, \Starred and \Important by role or attribute, and
// every label by its path.
func (t *Tx) GmailLabels(ctx context.Context, accountID int64) (map[string]int64, error) {
	rows, err := t.QueryContext(ctx, `SELECT id, path, role, attrs_json FROM mailboxes
		WHERE account_id = ? AND is_gmail_label = 1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("gmail labels: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var path, role, attrsJSON string
		if err := rows.Scan(&id, &path, &role, &attrsJSON); err != nil {
			return nil, fmt.Errorf("gmail labels: %w", err)
		}
		out[path] = id
		switch api.MailboxRole(role) {
		case api.MailboxRoleInbox:
			out[`\Inbox`] = id
		case api.MailboxRoleSent:
			out[`\Sent`] = id
		case api.MailboxRoleDrafts:
			out[`\Draft`] = id
		case api.MailboxRoleFlagged:
			out[`\Starred`] = id
		}
		var attrs []string
		if err := json.Unmarshal([]byte(attrsJSON), &attrs); err == nil {
			for _, a := range attrs {
				if strings.EqualFold(a, `\Important`) {
					out[`\Important`] = id
				}
			}
		}
	}
	return out, rows.Err()
}

// GmailResult is what InsertGmail changed.
type GmailResult struct {
	Added     []int64 // new messages
	Changed   []int64 // stored messages that moved here or changed labels
	Mailboxes []int64 // label mailboxes whose membership changed
}

// InsertGmail stores headers fetched from one synced folder of a Gmail
// account. A message already stored (by X-GM-MSGID) moves its UID
// membership here from the other synced folders and takes the reported
// flags; any other is inserted. Messages in All Mail get the labels of
// their header; messages in Spam and Trash have none. A header already
// stored at its UID is skipped, so a pass can be repeated.
func (t *Tx) InsertGmail(ctx context.Context, accountID, mailboxID int64, allMail bool, labels map[string]int64, hs []MessageHeader) (GmailResult, error) {
	var r GmailResult
	touched := map[int64]bool{}
	for _, h := range hs {
		var at int64
		err := t.QueryRowContext(ctx, `SELECT message_id FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`,
			mailboxID, int64(h.UID)).Scan(&at)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return r, fmt.Errorf("gmail insert uid %d: %w", h.UID, err)
		}
		var id int64
		err = t.QueryRowContext(ctx, `SELECT id FROM messages WHERE account_id = ? AND gm_msgid = ?`, accountID, int64(h.GmMsgID)).Scan(&id)
		switch {
		case h.GmMsgID != 0 && err == nil:
			if _, err := t.ExecContext(ctx, `DELETE FROM message_mailbox WHERE message_id = ? AND uid IS NOT NULL`, id); err != nil {
				return r, fmt.Errorf("gmail move %d: %w", id, err)
			}
			if _, err := t.ExecContext(ctx, `INSERT INTO message_mailbox (message_id, mailbox_id, uid, modseq) VALUES (?, ?, ?, ?)`,
				id, mailboxID, int64(h.UID), int64(h.ModSeq)); err != nil {
				return r, fmt.Errorf("gmail move %d: %w", id, err)
			}
			if _, err := t.UpdateFlags(ctx, mailboxID, []FlagUpdate{{UID: h.UID, ModSeq: h.ModSeq, Flags: h.Flags}}); err != nil {
				return r, err
			}
			r.Changed = append(r.Changed, id)
		case h.GmMsgID == 0 || errors.Is(err, sql.ErrNoRows):
			if id, err = t.insertMessage(ctx, accountID, h); err != nil {
				return r, err
			}
			if _, err := t.ExecContext(ctx, `UPDATE messages SET gm_msgid = ?, gm_thrid = ? WHERE id = ?`,
				nullUint(h.GmMsgID), nullUint(h.GmThrID), id); err != nil {
				return r, fmt.Errorf("gmail ids %d: %w", id, err)
			}
			if err := t.recordSeen(ctx, h); err != nil {
				return r, err
			}
			if _, err := t.ExecContext(ctx, `INSERT INTO message_mailbox (message_id, mailbox_id, uid, modseq) VALUES (?, ?, ?, ?)`,
				id, mailboxID, int64(h.UID), int64(h.ModSeq)); err != nil {
				return r, fmt.Errorf("gmail membership uid %d: %w", h.UID, err)
			}
			r.Added = append(r.Added, id)
		default:
			return r, fmt.Errorf("gmail lookup %d: %w", h.GmMsgID, err)
		}
		want := []string{}
		if allMail {
			want = h.Labels
		}
		mbs, err := t.setGmailLabels(ctx, id, want, labels)
		if err != nil {
			return r, err
		}
		for _, mb := range mbs {
			touched[mb] = true
		}
	}
	for mb := range touched {
		r.Mailboxes = append(r.Mailboxes, mb)
	}
	slices.Sort(r.Mailboxes)
	return r, nil
}

func nullUint(v uint64) any {
	if v == 0 {
		return nil
	}
	return int64(v)
}

// SetGmailLabels replaces the label memberships of the message at uid in a
// synced folder with labels (unknown names are skipped), returning the
// message and the label mailboxes that changed; found is false when the
// folder has no message at uid.
func (t *Tx) SetGmailLabels(ctx context.Context, mailboxID int64, uid uint32, labels []string, labelMap map[string]int64) (msgID int64, mailboxes []int64, found bool, err error) {
	err = t.QueryRowContext(ctx, `SELECT message_id FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`,
		mailboxID, int64(uid)).Scan(&msgID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, fmt.Errorf("gmail labels uid %d: %w", uid, err)
	}
	mailboxes, err = t.setGmailLabels(ctx, msgID, labels, labelMap)
	return msgID, mailboxes, true, err
}

// setGmailLabels makes a message's label memberships exactly the labels'
// mailboxes, returning the mailboxes it added or removed.
func (t *Tx) setGmailLabels(ctx context.Context, msgID int64, labels []string, labelMap map[string]int64) ([]int64, error) {
	want := map[int64]bool{}
	for _, l := range labels {
		if mb, ok := labelMap[l]; ok {
			want[mb] = true
		}
	}
	rows, err := t.QueryContext(ctx, `SELECT mm.mailbox_id FROM message_mailbox mm
		JOIN mailboxes mb ON mb.id = mm.mailbox_id
		WHERE mm.message_id = ? AND mb.is_gmail_label = 1`, msgID)
	if err != nil {
		return nil, fmt.Errorf("gmail labels of %d: %w", msgID, err)
	}
	have := map[int64]bool{}
	for rows.Next() {
		var mb int64
		if err := rows.Scan(&mb); err != nil {
			rows.Close()
			return nil, err
		}
		have[mb] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var changed []int64
	for mb := range have {
		if !want[mb] {
			if _, err := t.ExecContext(ctx, `DELETE FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?`, msgID, mb); err != nil {
				return nil, fmt.Errorf("drop label %d of %d: %w", mb, msgID, err)
			}
			changed = append(changed, mb)
		}
	}
	for mb := range want {
		if !have[mb] {
			if _, err := t.ExecContext(ctx, `INSERT INTO message_mailbox (message_id, mailbox_id) VALUES (?, ?)`, msgID, mb); err != nil {
				return nil, fmt.Errorf("add label %d to %d: %w", mb, msgID, err)
			}
			changed = append(changed, mb)
		}
	}
	slices.Sort(changed)
	return changed, nil
}

// RemoveGmailUIDs drops the memberships of uids in a synced folder, and the
// label memberships of their messages, but keeps the messages: one that
// moved to another synced folder is found again by its X-GM-MSGID in that
// folder's pass. PruneGmail deletes those left without a folder. It
// returns the messages and the label mailboxes affected.
func (t *Tx) RemoveGmailUIDs(ctx context.Context, mailboxID int64, uids []uint32) (msgs, mailboxes []int64, err error) {
	touched := map[int64]bool{}
	for _, uid := range uids {
		var id int64
		err := t.QueryRowContext(ctx, `SELECT message_id FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`,
			mailboxID, int64(uid)).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("gmail remove uid %d: %w", uid, err)
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`, mailboxID, int64(uid)); err != nil {
			return nil, nil, fmt.Errorf("gmail remove uid %d: %w", uid, err)
		}
		mbs, err := t.setGmailLabels(ctx, id, nil, nil)
		if err != nil {
			return nil, nil, err
		}
		for _, mb := range mbs {
			touched[mb] = true
		}
		msgs = append(msgs, id)
	}
	for mb := range touched {
		mailboxes = append(mailboxes, mb)
	}
	slices.Sort(mailboxes)
	return msgs, mailboxes, nil
}

// PruneGmail deletes the account's messages that belong to no mailbox (and
// their search entries) and returns their IDs.
func (t *Tx) PruneGmail(ctx context.Context, accountID int64) ([]int64, error) {
	rows, err := t.QueryContext(ctx, `SELECT id FROM messages m WHERE account_id = ?
		AND NOT EXISTS (SELECT 1 FROM message_mailbox mm WHERE mm.message_id = m.id)`, accountID)
	if err != nil {
		return nil, fmt.Errorf("prune gmail: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	threads, err := t.ThreadsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, err := t.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, id); err != nil {
			return nil, fmt.Errorf("prune %d: %w", id, err)
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, id); err != nil {
			return nil, fmt.Errorf("prune search entry %d: %w", id, err)
		}
	}
	return ids, t.RefreshThreads(ctx, threads)
}

// AssignGmailThreads puts messages in one thread per X-GM-THRID (JWZ
// threading for any without one) and refreshes the threads they joined.
func (t *Tx) AssignGmailThreads(ctx context.Context, accountID int64, ids []int64) ([]int64, error) {
	var touched []int64
	for _, id := range ids {
		var thrid sql.NullInt64
		var norm, internal string
		var old sql.NullInt64
		if err := t.QueryRowContext(ctx, `SELECT gm_thrid, subject_norm, internal_date, thread_id FROM messages WHERE id = ?`, id).
			Scan(&thrid, &norm, &internal, &old); err != nil {
			return nil, fmt.Errorf("gmail thread %d: %w", id, err)
		}
		if !thrid.Valid {
			tid, err := t.assignThread(ctx, accountID, id)
			if err != nil {
				return nil, err
			}
			touched = append(touched, tid)
			continue
		}
		var tid int64
		err := t.QueryRowContext(ctx, `SELECT id FROM threads WHERE account_id = ? AND gm_thrid = ?`, accountID, thrid.Int64).Scan(&tid)
		if errors.Is(err, sql.ErrNoRows) {
			res, err := t.ExecContext(ctx, `INSERT INTO threads (account_id, gm_thrid, subject_norm, last_date) VALUES (?, ?, ?, ?)`,
				accountID, thrid.Int64, norm, internal)
			if err != nil {
				return nil, fmt.Errorf("gmail thread %d: %w", id, err)
			}
			if tid, err = res.LastInsertId(); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, fmt.Errorf("gmail thread %d: %w", id, err)
		}
		if _, err := t.ExecContext(ctx, `UPDATE messages SET thread_id = ? WHERE id = ?`, tid, id); err != nil {
			return nil, err
		}
		touched = append(touched, tid)
		if old.Valid && old.Int64 != tid {
			touched = append(touched, old.Int64)
		}
	}
	slices.Sort(touched)
	touched = slices.Compact(touched)
	return touched, t.RefreshThreads(ctx, touched)
}
