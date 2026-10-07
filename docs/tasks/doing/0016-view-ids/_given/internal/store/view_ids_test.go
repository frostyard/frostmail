package store

// CONTRACT TEST for task card T-0016 (docs/tasks). Do not edit.

import (
	"slices"
	"testing"
	"time"
)

// viewFixture builds two accounts. Messages 1-6 belong to account A, 7 to
// account B; ids are explicit so expectations are readable.
//
//	id  mailbox      internal date  seen flagged deleted  indexed text
//	1   A/INBOX      10:01          0    0       0        invoice march
//	2   A/INBOX      10:05          1    1       0        lunch
//	3   A/Archive    10:03          1    0       0        invoice april
//	4   A/INBOX+Arc  10:04          0    1       0        receipts
//	5   A/INBOX      10:02          0    0       1        invoice deleted
//	6   A/INBOX      10:05          0    0       0        tie with 2
//	7   B/INBOX      10:06          0    0       0        invoice other account
func viewFixture(t *testing.T) (d *DB, acctA, acctB, inboxA, archiveA int64) {
	t.Helper()
	d, _ = openTest(t)
	ctx := t.Context()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for i, email := range []string{"a@mailtest.test", "b@mailtest.test"} {
		exec(`INSERT INTO accounts (id, kind, email, auth, imap_host, imap_port, imap_tls, imap_username,
			smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
			VALUES (?, 'imap', ?, 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now')`, i+1, email)
	}
	acctA, acctB = 1, 2
	exec(`INSERT INTO mailboxes (id, account_id, path, name) VALUES (10, 1, 'INBOX', 'INBOX'), (11, 1, 'Archive', 'Archive'), (20, 2, 'INBOX', 'INBOX')`)
	inboxA, archiveA = 10, 11
	rows := []struct {
		id, acct               int64
		minute                 int
		seen, flagged, deleted int
		mailboxes              []int64
		text                   string
	}{
		{1, 1, 1, 0, 0, 0, []int64{10}, "invoice march"},
		{2, 1, 5, 1, 1, 0, []int64{10}, "lunch"},
		{3, 1, 3, 1, 0, 0, []int64{11}, "invoice april"},
		{4, 1, 4, 0, 1, 0, []int64{10, 11}, "receipts"},
		{5, 1, 2, 0, 0, 1, []int64{10}, "invoice deleted"},
		{6, 1, 5, 0, 0, 0, []int64{10}, "tie with two"},
		{7, 2, 6, 0, 0, 0, []int64{20}, "invoice other account"},
	}
	for _, r := range rows {
		when := FormatTime(time.Date(2026, 10, 7, 10, r.minute, 0, 0, time.UTC))
		exec(`INSERT INTO messages (id, account_id, internal_date, seen, flagged, deleted) VALUES (?, ?, ?, ?, ?, ?)`,
			r.id, r.acct, when, r.seen, r.flagged, r.deleted)
		for uid, mb := range r.mailboxes {
			exec(`INSERT INTO message_mailbox (message_id, mailbox_id, uid) VALUES (?, ?, ?)`, r.id, mb, r.id*10+int64(uid))
		}
		exec(`INSERT INTO messages_fts (rowid, subject) VALUES (?, ?)`, r.id, r.text)
	}
	return d, acctA, acctB, inboxA, archiveA
}

func viewBool(b bool) *bool { return &b }

func TestViewIDsFilters(t *testing.T) {
	d, acctA, acctB, inboxA, archiveA := viewFixture(t)
	cases := []struct {
		name string
		f    ViewFilter
		want []int64
	}{
		{"everything, newest first, id breaks ties", ViewFilter{}, []int64{7, 6, 2, 4, 3, 1}},
		{"account A", ViewFilter{AccountID: acctA}, []int64{6, 2, 4, 3, 1}},
		{"account B", ViewFilter{AccountID: acctB}, []int64{7}},
		{"A inbox", ViewFilter{MailboxID: inboxA}, []int64{6, 2, 4, 1}},
		{"A archive", ViewFilter{MailboxID: archiveA}, []int64{4, 3}},
		{"unread", ViewFilter{AccountID: acctA, Unread: viewBool(true)}, []int64{6, 4, 1}},
		{"read", ViewFilter{AccountID: acctA, Unread: viewBool(false)}, []int64{2, 3}},
		{"flagged", ViewFilter{Flagged: viewBool(true)}, []int64{2, 4}},
		{"unflagged in archive", ViewFilter{MailboxID: archiveA, Flagged: viewBool(false)}, []int64{3}},
		{"search", ViewFilter{Text: "invoice"}, []int64{7, 3, 1}},
		{"search prefix in mailbox", ViewFilter{MailboxID: inboxA, Text: "inv"}, []int64{1}},
		{"search and unread", ViewFilter{AccountID: acctA, Text: "invoice", Unread: viewBool(true)}, []int64{1}},
		{"nothing searchable means no text filter", ViewFilter{AccountID: acctB, Text: "!!!"}, []int64{7}},
		{"no match", ViewFilter{Text: "zebra"}, nil},
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

func TestViewIDsLargeMailbox(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	for _, q := range []string{
		`INSERT INTO accounts (id, kind, email, auth, imap_host, imap_port, imap_tls, imap_username,
			smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
			VALUES (1, 'imap', 'a@mailtest.test', 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now')`,
		`INSERT INTO mailboxes (id, account_id, path, name) VALUES (10, 1, 'INBOX', 'INBOX')`,
		`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 50000)
			INSERT INTO messages (id, account_id, internal_date, seen)
			SELECT n, 1, printf('2026-%02d-%02dT%02d:%02d:00.000Z', 1 + n % 12, 1 + n % 28, n % 24, n % 60), n % 2 FROM seq`,
		`INSERT INTO message_mailbox (message_id, mailbox_id, uid) SELECT id, 10, id FROM messages`,
	} {
		if _, err := d.db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	ids, err := d.ViewIDs(ctx, ViewFilter{MailboxID: 10})
	elapsed := time.Since(start)
	if err != nil || len(ids) != 50000 {
		t.Fatalf("ViewIDs = %d ids, %v", len(ids), err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("ViewIDs over 50,000 messages took %v", elapsed)
	}
	unread, err := d.ViewIDs(ctx, ViewFilter{MailboxID: 10, Unread: viewBool(true)})
	if err != nil || len(unread) != 25000 {
		t.Fatalf("unread = %d, %v", len(unread), err)
	}
}
