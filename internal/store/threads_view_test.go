package store

// CONTRACT TEST for task card T-0024 (docs/tasks). Do not edit.

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// threadViewFixture extends viewFixture (see view_ids_test.go) with roles
// and threads:
//
//	mailbox 10 A/INBOX role inbox, 11 A/Archive role archive, 20 B/INBOX role inbox
//	thread 100 (account A, msg_count 2): messages 1, 3 and the deleted 5
//	thread 101 (account A, msg_count 2): messages 2 and 6
//	messages 4 and 7 have no thread
func threadViewFixture(t *testing.T) *DB {
	t.Helper()
	d, _, _, _, _ := viewFixture(t)
	for _, q := range []string{
		`UPDATE mailboxes SET role = 'inbox' WHERE id IN (10, 20)`,
		`UPDATE mailboxes SET role = 'archive' WHERE id = 11`,
		`INSERT INTO threads (id, account_id, last_date, msg_count) VALUES (100, 1, 'x', 2), (101, 1, 'x', 2), (102, 1, 'x', 0)`,
		`UPDATE messages SET thread_id = 100 WHERE id IN (1, 3, 5)`,
		`UPDATE messages SET thread_id = 101 WHERE id IN (2, 6)`,
	} {
		if _, err := d.db.ExecContext(t.Context(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return d
}

func TestViewIDsRolesAndThreads(t *testing.T) {
	d := threadViewFixture(t)
	cases := []struct {
		name string
		f    ViewFilter
		want []int64
	}{
		{"every inbox", ViewFilter{Role: "inbox"}, []int64{7, 6, 2, 4, 1}},
		{"archive role", ViewFilter{Role: "archive"}, []int64{4, 3}},
		{"inboxes of account B", ViewFilter{Role: "inbox", AccountID: 2}, []int64{7}},
		{"a role no mailbox has", ViewFilter{Role: "junk"}, nil},
		{"threads: newest message of each", ViewFilter{Threads: true}, []int64{7, 6, 4, 3}},
		{"threads within a mailbox", ViewFilter{Threads: true, MailboxID: 10}, []int64{6, 4, 1}},
		{"threads of search results", ViewFilter{Threads: true, Text: "invoice"}, []int64{7, 3}},
		{"threads of unread messages", ViewFilter{Threads: true, AccountID: 1, Unread: viewBool(true)}, []int64{6, 4, 1}},
		{"threads of every inbox", ViewFilter{Threads: true, Role: "inbox"}, []int64{7, 6, 4, 1}},
	}
	for _, tc := range cases {
		got, err := d.ViewIDs(t.Context(), tc.f)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: ViewIDs = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestThreadMessageIDs(t *testing.T) {
	d := threadViewFixture(t)
	ctx := t.Context()
	cases := []struct {
		thread int64
		want   []int64
	}{
		{100, []int64{1, 3}}, // oldest first; the deleted 5 is left out
		{101, []int64{2, 6}}, // same time: lower ID first
		{102, []int64{}},     // a thread with no messages left
	}
	for _, tc := range cases {
		got, err := d.ThreadMessageIDs(ctx, tc.thread)
		if err != nil {
			t.Fatalf("thread %d: %v", tc.thread, err)
		}
		if got == nil || !slices.Equal(got, tc.want) {
			t.Errorf("ThreadMessageIDs(%d) = %#v, want %v", tc.thread, got, tc.want)
		}
	}
	if _, err := d.ThreadMessageIDs(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("ThreadMessageIDs(999) error = %v, want ErrNotFound", err)
	}
}

func TestSummaryThreadCount(t *testing.T) {
	d := threadViewFixture(t)
	ctx := t.Context()
	got, err := d.Summaries(ctx, []int64{1, 6, 4})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[int64]int64{}
	for _, s := range got {
		counts[s.ID] = s.ThreadCount
	}
	want := map[int64]int64{1: 2, 6: 2, 4: 1}
	for id, n := range want {
		if counts[id] != n {
			t.Errorf("summary %d ThreadCount = %d, want %d", id, counts[id], n)
		}
	}
	m, err := d.GetMessage(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if m.ThreadCount != 2 {
		t.Errorf("GetMessage(3).ThreadCount = %d, want 2", m.ThreadCount)
	}
	m, err = d.GetMessage(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if m.ThreadCount != 1 {
		t.Errorf("GetMessage(7).ThreadCount = %d, want 1 (no thread)", m.ThreadCount)
	}
}

func TestThreadViewLargeMailbox(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	for _, q := range []string{
		`INSERT INTO accounts (id, kind, email, auth, imap_host, imap_port, imap_tls, imap_username,
			smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
			VALUES (1, 'imap', 'a@mailtest.test', 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now')`,
		`INSERT INTO mailboxes (id, account_id, path, name, role) VALUES (10, 1, 'INBOX', 'INBOX', 'inbox')`,
		`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 10000)
			INSERT INTO threads (id, account_id, last_date, msg_count) SELECT n, 1, 'x', 5 FROM seq`,
		`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 50000)
			INSERT INTO messages (id, account_id, thread_id, internal_date)
			SELECT n, 1, 1 + n % 10000, printf('2026-%02d-%02dT%02d:%02d:00.000Z', 1 + n % 12, 1 + n % 28, n % 24, n % 60) FROM seq`,
		`INSERT INTO message_mailbox (message_id, mailbox_id, uid) SELECT id, 10, id FROM messages`,
	} {
		if _, err := d.db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	ids, err := d.ViewIDs(ctx, ViewFilter{Role: "inbox", Threads: true})
	elapsed := time.Since(start)
	if err != nil || len(ids) != 10000 {
		t.Fatalf("thread view = %d ids, %v; want 10,000", len(ids), err)
	}
	if elapsed > time.Second {
		t.Fatalf("thread view over 50,000 messages took %v", elapsed)
	}
}
