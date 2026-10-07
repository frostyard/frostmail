package store

import (
	"encoding/json/v2"
	"testing"
	"time"
)

type threadMsg struct {
	id         int64
	msgid, irt string
	refs       []string
	subject    string
	day        int
	seen       bool
}

// threadFixture inserts an account and messages directly (no threads yet).
func threadFixture(t *testing.T, msgs ...threadMsg) *DB {
	t.Helper()
	d, _ := openTest(t)
	if _, err := d.db.ExecContext(t.Context(), `INSERT INTO accounts (id, kind, email, auth, imap_host, imap_port, imap_tls,
		imap_username, smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
		VALUES (1, 'imap', 'a@mailtest.test', 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now')`); err != nil {
		t.Fatal(err)
	}
	addThreadMsgs(t, d, msgs...)
	return d
}

func addThreadMsgs(t *testing.T, d *DB, msgs ...threadMsg) {
	t.Helper()
	for _, m := range msgs {
		refs, _ := json.Marshal(append([]string{}, m.refs...))
		norm := m.subject
		if len(norm) > 4 && norm[:4] == "Re: " {
			norm = norm[4:]
		}
		seen := 0
		if m.seen {
			seen = 1
		}
		when := FormatTime(time.Date(2026, 10, m.day, 12, 0, 0, 0, time.UTC))
		if _, err := d.db.ExecContext(t.Context(), `INSERT INTO messages (id, account_id, msgid_hdr, in_reply_to, refs_json,
			subject, subject_norm, internal_date, seen) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?)`,
			m.id, m.msgid, m.irt, string(refs), m.subject, norm, when, seen); err != nil {
			t.Fatal(err)
		}
	}
}

func assign(t *testing.T, d *DB, ids ...int64) []int64 {
	t.Helper()
	ctx := t.Context()
	var touched []int64
	if err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		touched, err = tx.AssignThreads(ctx, 1, ids)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return touched
}

func threadOf(t *testing.T, d *DB, id int64) int64 {
	t.Helper()
	var tid int64
	if err := d.db.QueryRowContext(t.Context(), `SELECT thread_id FROM messages WHERE id = ?`, id).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	return tid
}

func TestThreadsByReferences(t *testing.T) {
	d := threadFixture(t,
		threadMsg{id: 1, msgid: "a", subject: "Plan", day: 1},
		threadMsg{id: 2, msgid: "b", irt: "a", refs: []string{"a"}, subject: "Re: Plan", day: 2},
		threadMsg{id: 3, msgid: "c", subject: "Unrelated", day: 3},
	)
	assign(t, d, 1, 2, 3)
	if threadOf(t, d, 1) != threadOf(t, d, 2) || threadOf(t, d, 1) == threadOf(t, d, 3) {
		t.Fatalf("threads = %d %d %d", threadOf(t, d, 1), threadOf(t, d, 2), threadOf(t, d, 3))
	}
}

func TestThreadsReplyBeforeParent(t *testing.T) {
	d := threadFixture(t,
		threadMsg{id: 1, msgid: "b", irt: "a", refs: []string{"root", "a"}, subject: "Re: Plan", day: 2},
	)
	assign(t, d, 1)
	addThreadMsgs(t, d,
		threadMsg{id: 2, msgid: "a", refs: []string{"root"}, subject: "Plan", day: 1},
		threadMsg{id: 3, msgid: "root", subject: "Plan", day: 1},
	)
	assign(t, d, 2, 3)
	if threadOf(t, d, 1) != threadOf(t, d, 2) || threadOf(t, d, 2) != threadOf(t, d, 3) {
		t.Fatal("a parent arriving after its reply must join the reply's thread")
	}
}

func TestThreadsMerge(t *testing.T) {
	d := threadFixture(t,
		threadMsg{id: 1, msgid: "x", subject: "X", day: 1},
		threadMsg{id: 2, msgid: "y", subject: "Y", day: 1},
	)
	assign(t, d, 1, 2)
	tx, ty := threadOf(t, d, 1), threadOf(t, d, 2)
	if tx == ty {
		t.Fatal("x and y must start apart")
	}
	addThreadMsgs(t, d, threadMsg{id: 3, msgid: "z", refs: []string{"x", "y"}, subject: "Re: both", day: 2})
	touched := assign(t, d, 3)
	if threadOf(t, d, 2) != tx || threadOf(t, d, 3) != tx || len(touched) != 1 || touched[0] != tx {
		t.Fatalf("after merge: %d %d %d, touched %v; want all in %d", threadOf(t, d, 1), threadOf(t, d, 2), threadOf(t, d, 3), touched, tx)
	}
	var n int
	if err := d.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM threads WHERE id = ?`, ty).Scan(&n); err != nil || n != 0 {
		t.Fatalf("merged-away thread still exists: %d, %v", n, err)
	}
	if err := d.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM thread_refs WHERE thread_id = ?`, ty).Scan(&n); err != nil || n != 0 {
		t.Fatalf("thread_refs of the merged thread: %d, %v", n, err)
	}
}

func TestThreadsBySubject(t *testing.T) {
	d := threadFixture(t,
		threadMsg{id: 1, msgid: "p", subject: "Lunch", day: 1},
		threadMsg{id: 2, msgid: "q", subject: "Re: Lunch", day: 3},
		threadMsg{id: 3, msgid: "r", subject: "Re: Lunch", day: 20},
		threadMsg{id: 4, msgid: "s", subject: "Lunch", day: 3},
	)
	assign(t, d, 1, 2, 3, 4)
	if threadOf(t, d, 2) != threadOf(t, d, 1) {
		t.Error("a reply without references within 7 days must join by subject")
	}
	if threadOf(t, d, 3) == threadOf(t, d, 1) {
		t.Error("a reply after 7 days quiet must start a new thread")
	}
	if threadOf(t, d, 4) == threadOf(t, d, 1) {
		t.Error("the same subject without a reply prefix must not join")
	}
}

func TestThreadAggregatesAndRefresh(t *testing.T) {
	d := threadFixture(t,
		threadMsg{id: 1, msgid: "a", subject: "Plan", day: 1, seen: true},
		threadMsg{id: 2, msgid: "b", refs: []string{"a"}, subject: "Re: Plan", day: 5},
		threadMsg{id: 3, msgid: "c", refs: []string{"a"}, subject: "Re: Plan", day: 3},
	)
	assign(t, d, 1, 2, 3)
	tid := threadOf(t, d, 1)
	var count, unread int
	var last string
	q := `SELECT msg_count, unread_count, last_date FROM threads WHERE id = ?`
	if err := d.db.QueryRowContext(t.Context(), q, tid).Scan(&count, &unread, &last); err != nil {
		t.Fatal(err)
	}
	if count != 3 || unread != 2 || last != "2026-10-05T12:00:00.000Z" {
		t.Fatalf("aggregates count %d unread %d last %s", count, unread, last)
	}
	ctx := t.Context()
	err := d.Tx(ctx, func(tx *Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM messages`); err != nil {
			return err
		}
		return tx.RefreshThreads(ctx, []int64{tid})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM threads`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("empty thread survived refresh: %d, %v", count, err)
	}
}
