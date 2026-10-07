package store

// CONTRACT TEST for task card T-0017 (docs/tasks). Do not edit.

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// readFixture: account 1 with INBOX (10) and Archive (11), thread 5, and
//
//	message 1: every field set, in INBOX (uid 7) and Archive (uid 3), two parts
//	message 2: no Date header, thread 5, in INBOX (uid 8)
//	message 3: only a membership without a UID (a pending local move)
func readFixture(t *testing.T) *DB {
	t.Helper()
	d, _ := openTest(t)
	ctx := t.Context()
	for _, q := range []string{
		`INSERT INTO accounts (id, kind, email, auth, imap_host, imap_port, imap_tls, imap_username,
			smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
			VALUES (1, 'imap', 'a@mailtest.test', 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now')`,
		`INSERT INTO mailboxes (id, account_id, path, name) VALUES (10, 1, 'INBOX', 'INBOX'), (11, 1, 'Archive', 'Archive')`,
		`INSERT INTO threads (id, account_id, last_date) VALUES (5, 1, '2026-10-07T10:00:00.000Z')`,
		`INSERT INTO messages (id, account_id, thread_id, msgid_hdr, in_reply_to, refs_json, subject, from_name, from_addr,
			to_json, cc_json, reply_to_json, date_hdr, internal_date, size, preview, has_attachments, list_id, list_unsubscribe,
			seen, flagged, answered, forwarded, draft, flag_color, keywords_json)
			VALUES (1, 1, NULL, 'm1@mailtest.test', 'p@mailtest.test', '["r@mailtest.test","p@mailtest.test"]', 'Re: Plan',
			'Bob', 'bob@mailtest.test', '[{"name":"","addr":"a@mailtest.test"}]',
			'[{"name":"Carol","addr":"carol@mailtest.test"}]', '[{"name":"Lists","addr":"list@mailtest.test"}]',
			'2026-10-07T08:00:00.000Z', '2026-10-07T08:01:00.000Z', 4321, 'The plan', 1, 'dev.mailtest.test', '<mailto:u@x.test>',
			1, 1, 1, 0, 0, 3, '["Work"]')`,
		`INSERT INTO messages (id, account_id, thread_id, subject, internal_date, size, keywords_json)
			VALUES (2, 1, 5, 'No date', '2026-10-07T09:30:00.000Z', 10, '[]')`,
		`INSERT INTO messages (id, account_id, subject, internal_date) VALUES (3, 1, 'Moving', '2026-10-07T09:31:00.000Z')`,
		`INSERT INTO message_mailbox (message_id, mailbox_id, uid) VALUES (1, 11, 3), (1, 10, 7), (2, 10, 8), (3, 11, NULL)`,
		`INSERT INTO parts (message_id, path, content_type, charset, encoding, disposition, filename, content_id, size)
			VALUES (1, '2', 'application/pdf', '', 'base64', 'attachment', 'plan.pdf', '', 900),
			       (1, '1', 'text/plain', 'utf-8', '7bit', '', '', '', 120)`,
	} {
		if _, err := d.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return d
}

var summary1 = Summary{
	ID: 1, AccountID: 1, MailboxIDs: []int64{10, 11}, ThreadID: 0, Subject: "Re: Plan",
	From: Address{Name: "Bob", Addr: "bob@mailtest.test"}, Date: time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC),
	Preview: "The plan", Flags: Flags{Seen: true, Flagged: true, Answered: true, Color: 3, Keywords: []string{"Work"}},
	HasAttachments: true, Size: 4321,
}

var summary2 = Summary{
	ID: 2, AccountID: 1, MailboxIDs: []int64{10}, ThreadID: 5, Subject: "No date",
	Date: time.Date(2026, 10, 7, 9, 30, 0, 0, time.UTC), Size: 10,
}

func TestSummaries(t *testing.T) {
	d := readFixture(t)
	got, err := d.Summaries(t.Context(), []int64{2, 99, 1})
	if err != nil {
		t.Fatal(err)
	}
	if want := []Summary{summary2, summary1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Summaries =\n%+v\nwant\n%+v", got, want)
	}
	if got, err := d.Summaries(t.Context(), nil); err != nil || len(got) != 0 {
		t.Fatalf("Summaries(nil) = %v, %v", got, err)
	}
}

func TestGetMessage(t *testing.T) {
	d := readFixture(t)
	got, err := d.GetMessage(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	want := MessageDetail{
		Summary:         summary1,
		To:              []Address{{Addr: "a@mailtest.test"}},
		Cc:              []Address{{Name: "Carol", Addr: "carol@mailtest.test"}},
		ReplyTo:         []Address{{Name: "Lists", Addr: "list@mailtest.test"}},
		MessageID:       "m1@mailtest.test",
		InReplyTo:       "p@mailtest.test",
		References:      []string{"r@mailtest.test", "p@mailtest.test"},
		ListID:          "dev.mailtest.test",
		ListUnsubscribe: "<mailto:u@x.test>",
		Parts: []Part{
			{Path: "1", ContentType: "text/plain", Charset: "utf-8", Encoding: "7bit", Size: 120},
			{Path: "2", ContentType: "application/pdf", Encoding: "base64", Disposition: "attachment", Filename: "plan.pdf", Size: 900},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetMessage(1) =\n%+v\nwant\n%+v", got, want)
	}
	m2, err := d.GetMessage(t.Context(), 2)
	if err != nil || m2.To != nil || m2.References != nil || m2.Parts != nil || !reflect.DeepEqual(m2.Summary, summary2) {
		t.Fatalf("GetMessage(2) = %+v, %v; empty lists must be nil", m2, err)
	}
	if _, err := d.GetMessage(t.Context(), 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetMessage(99) = %v, want ErrNotFound", err)
	}
}

func TestMessageLocation(t *testing.T) {
	d := readFixture(t)
	got, err := d.MessageLocation(t.Context(), 1)
	if want := (Location{AccountID: 1, MailboxID: 10, MailboxPath: "INBOX", UID: 7}); err != nil || got != want {
		t.Fatalf("MessageLocation(1) = %+v, %v; want %+v (lowest mailbox ID with a UID)", got, err, want)
	}
	for _, id := range []int64{3, 99} {
		if _, err := d.MessageLocation(t.Context(), id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("MessageLocation(%d) = %v, want ErrNotFound", id, err)
		}
	}
}

func TestSetBody(t *testing.T) {
	d := readFixture(t)
	ctx := t.Context()
	const blobID = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.SetBody(ctx, 2, blobID) }); err != nil {
		t.Fatal(err)
	}
	m, err := d.GetMessage(ctx, 2)
	if err != nil || m.BlobID != blobID {
		t.Fatalf("BlobID = %q, %v", m.BlobID, err)
	}
	var state string
	if err := d.db.QueryRowContext(ctx, `SELECT body_state FROM messages WHERE id = 2`).Scan(&state); err != nil || state != "full" {
		t.Fatalf("body_state = %q, %v", state, err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.SetBody(ctx, 99, blobID) }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetBody(99) = %v, want ErrNotFound", err)
	}
}
