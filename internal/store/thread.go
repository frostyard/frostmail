package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// SubjectWindow is how far back a reply without references may join a
// thread by subject alone (docs/design/sync.md, threading).
const SubjectWindow = 7 * 24 * time.Hour

// AssignThreads puts each message in ids into a thread, in order, and
// returns the IDs of every thread it touched. It is incremental JWZ over
// thread_refs: every Message-ID a message carries (its own, In-Reply-To and
// References) maps to one thread. A message whose IDs reach no thread starts
// one; reaching one thread joins it; reaching several merges them into the
// oldest. A reply with no IDs at all (a subject with a Re: prefix) joins the
// newest thread with the same normalized subject seen within SubjectWindow.
// Thread aggregates are refreshed for the returned threads.
func (t *Tx) AssignThreads(ctx context.Context, accountID int64, ids []int64) ([]int64, error) {
	var touched []int64
	for _, id := range ids {
		tid, err := t.assignThread(ctx, accountID, id)
		if err != nil {
			return nil, fmt.Errorf("thread message %d: %w", id, err)
		}
		if !slices.Contains(touched, tid) {
			touched = append(touched, tid)
		}
	}
	slices.Sort(touched)
	return touched, t.RefreshThreads(ctx, touched)
}

func (t *Tx) assignThread(ctx context.Context, accountID, id int64) (int64, error) {
	var msgid, irt, refsJSON, subject, norm, internal string
	err := t.QueryRowContext(ctx, `SELECT msgid_hdr, in_reply_to, refs_json, subject, subject_norm, internal_date
		FROM messages WHERE id = ? AND account_id = ?`, id, accountID).Scan(&msgid, &irt, &refsJSON, &subject, &norm, &internal)
	if err != nil {
		return 0, err
	}
	var refs []string
	if err := json.Unmarshal([]byte(refsJSON), &refs); err != nil {
		return 0, err
	}
	var keys []string
	for _, k := range append(refs, irt, msgid) {
		if k != "" && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}

	threads, err := t.threadsFor(ctx, accountID, keys)
	if err != nil {
		return 0, err
	}
	var tid int64
	switch {
	case len(threads) > 0:
		tid = threads[0]
		for _, other := range threads[1:] {
			if err := t.mergeThread(ctx, other, tid); err != nil {
				return 0, err
			}
		}
	case len(refs) == 0 && irt == "" && norm != "" && !strings.EqualFold(norm, strings.TrimSpace(subject)):
		tid, err = t.threadBySubject(ctx, accountID, norm, internal)
		if err != nil {
			return 0, err
		}
	}
	if tid == 0 {
		res, err := t.ExecContext(ctx, `INSERT INTO threads (account_id, subject_norm, last_date) VALUES (?, ?, ?)`, accountID, norm, internal)
		if err != nil {
			return 0, err
		}
		if tid, err = res.LastInsertId(); err != nil {
			return 0, err
		}
	}
	for _, k := range keys {
		if _, err := t.ExecContext(ctx, `INSERT INTO thread_refs (account_id, msgid_hdr, thread_id) VALUES (?, ?, ?)
			ON CONFLICT (account_id, msgid_hdr) DO UPDATE SET thread_id = excluded.thread_id`, accountID, k, tid); err != nil {
			return 0, err
		}
	}
	if _, err := t.ExecContext(ctx, `UPDATE messages SET thread_id = ? WHERE id = ?`, tid, id); err != nil {
		return 0, err
	}
	return tid, nil
}

// threadsFor returns the distinct threads the keys map to, oldest first.
func (t *Tx) threadsFor(ctx context.Context, accountID int64, keys []string) ([]int64, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	args := []any{accountID}
	for _, k := range keys {
		args = append(args, k)
	}
	rows, err := t.QueryContext(ctx, `SELECT DISTINCT thread_id FROM thread_refs WHERE account_id = ? AND msgid_hdr IN (`+
		strings.TrimSuffix(strings.Repeat("?, ", len(keys)), ", ")+`) ORDER BY thread_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (t *Tx) threadBySubject(ctx context.Context, accountID int64, norm, internal string) (int64, error) {
	when, err := ParseTime(internal)
	if err != nil {
		return 0, err
	}
	var tid int64
	err = t.QueryRowContext(ctx, `SELECT id FROM threads WHERE account_id = ? AND subject_norm = ? COLLATE NOCASE
		AND last_date >= ? ORDER BY last_date DESC LIMIT 1`, accountID, norm, FormatTime(when.Add(-SubjectWindow))).Scan(&tid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return tid, err
}

// mergeThread moves everything in thread from into thread into and deletes
// from.
func (t *Tx) mergeThread(ctx context.Context, from, into int64) error {
	for _, q := range []string{
		`UPDATE messages SET thread_id = ? WHERE thread_id = ?`,
		`UPDATE thread_refs SET thread_id = ? WHERE thread_id = ?`,
	} {
		if _, err := t.ExecContext(ctx, q, into, from); err != nil {
			return err
		}
	}
	_, err := t.ExecContext(ctx, `DELETE FROM threads WHERE id = ?`, from)
	return err
}

// RefreshThreads recomputes the counts and last date of threads from their
// messages, and deletes threads left without messages (their thread_refs go
// with them, so a later message with those IDs starts afresh).
func (t *Tx) RefreshThreads(ctx context.Context, threadIDs []int64) error {
	for _, id := range threadIDs {
		_, err := t.ExecContext(ctx, `UPDATE threads SET
			msg_count = (SELECT COUNT(*) FROM messages WHERE thread_id = threads.id),
			unread_count = (SELECT COUNT(*) FROM messages WHERE thread_id = threads.id AND seen = 0 AND deleted = 0),
			flagged_count = (SELECT COUNT(*) FROM messages WHERE thread_id = threads.id AND flagged = 1),
			last_date = COALESCE((SELECT MAX(internal_date) FROM messages WHERE thread_id = threads.id), last_date)
			WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("refresh thread %d: %w", id, err)
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM threads WHERE id = ? AND msg_count = 0`, id); err != nil {
			return err
		}
	}
	return nil
}

// ThreadsOf returns the distinct thread IDs of messages, for RefreshThreads
// after their flags change or before they are removed.
func (t *Tx) ThreadsOf(ctx context.Context, messageIDs []int64) ([]int64, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}
	args := make([]any, len(messageIDs))
	for i, id := range messageIDs {
		args[i] = id
	}
	rows, err := t.QueryContext(ctx, `SELECT DISTINCT thread_id FROM messages WHERE thread_id IS NOT NULL AND id IN (`+
		strings.TrimSuffix(strings.Repeat("?, ", len(messageIDs)), ", ")+`) ORDER BY thread_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ThreadsOfUIDs returns the distinct thread IDs of the messages at uids in
// mailboxID, for RefreshThreads after RemoveUIDs.
func (t *Tx) ThreadsOfUIDs(ctx context.Context, mailboxID int64, uids []uint32) ([]int64, error) {
	var ids []int64
	for _, u := range uids {
		var id int64
		err := t.QueryRowContext(ctx, `SELECT message_id FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`, mailboxID, int64(u)).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return t.ThreadsOf(ctx, ids)
}
