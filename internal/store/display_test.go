package store

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestSetDisplay(t *testing.T) {
	d := readFixture(t)
	ctx := t.Context()
	const blobID = "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.SetDisplay(ctx, 2, blobID) }); err != nil {
		t.Fatal(err)
	}
	m, err := d.GetMessage(ctx, 2)
	if err != nil || m.DisplayBlobID != blobID || m.BlobID != "" {
		t.Fatalf("DisplayBlobID = %q, BlobID = %q, %v", m.DisplayBlobID, m.BlobID, err)
	}
	var state string
	if err := d.db.QueryRowContext(ctx, `SELECT body_state FROM messages WHERE id = 2`).Scan(&state); err != nil || state != "headers" {
		t.Fatalf("body_state = %q, %v; a display copy is not the whole body", state, err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.SetDisplay(ctx, 99, blobID) }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetDisplay(99) = %v, want ErrNotFound", err)
	}
}

// prefetchFixture: account 1 with INBOX (10, role inbox), Archive (11)
// and Gmail-style All Mail (12, role all), a VIP vip@mailtest.test, and
//
//	1  INBOX uid 1, Oct 6                        a candidate
//	2  INBOX uid 2, Oct 7                        a candidate, the newest
//	3  Archive uid 3 from the VIP, Oct 5         a candidate
//	4  Archive uid 4, Oct 5                      not INBOX, not a VIP
//	5  INBOX uid 5, Oct 6, body stored           has its whole body
//	6  INBOX uid 6, Oct 6, display copy stored   has a display copy
//	7  INBOX uid 7, June                         older than since
//	8  INBOX without a UID (a pending move)      not fetchable
//	9  INBOX label, All Mail uid 9, Oct 4        a candidate (Gmail)
//	10 INBOX uid 10, Oct 6, deleted              deleted
//	11 account 2's INBOX uid 1, Oct 7            another account
func prefetchFixture(t *testing.T) *DB {
	t.Helper()
	d, _ := openTest(t)
	ctx := t.Context()
	for _, q := range []string{
		`INSERT INTO accounts (id, kind, email, auth, imap_host, imap_port, imap_tls, imap_username,
			smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
			VALUES (1, 'imap', 'a@mailtest.test', 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now'),
			       (2, 'imap', 'b@mailtest.test', 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now')`,
		`INSERT INTO mailboxes (id, account_id, path, name, role) VALUES (10, 1, 'INBOX', 'INBOX', 'inbox'),
			(11, 1, 'Archive', 'Archive', 'none'), (12, 1, '[Gmail]/All Mail', 'All Mail', 'all'), (20, 2, 'INBOX', 'INBOX', 'inbox')`,
		`INSERT INTO vips (address, added_at) VALUES ('vip@mailtest.test', 'now')`,
		`INSERT INTO messages (id, account_id, from_addr, internal_date, blob_sha, display_sha, deleted) VALUES
			(1, 1, 'a@mailtest.test', '2026-10-06T08:00:00.000Z', NULL, NULL, 0),
			(2, 1, 'a@mailtest.test', '2026-10-07T08:00:00.000Z', NULL, NULL, 0),
			(3, 1, 'VIP@mailtest.test', '2026-10-05T08:00:00.000Z', NULL, NULL, 0),
			(4, 1, 'a@mailtest.test', '2026-10-05T08:00:00.000Z', NULL, NULL, 0),
			(5, 1, 'a@mailtest.test', '2026-10-06T08:00:00.000Z', 'full', NULL, 0),
			(6, 1, 'a@mailtest.test', '2026-10-06T08:00:00.000Z', NULL, 'display', 0),
			(7, 1, 'a@mailtest.test', '2026-06-01T08:00:00.000Z', NULL, NULL, 0),
			(8, 1, 'a@mailtest.test', '2026-10-06T08:00:00.000Z', NULL, NULL, 0),
			(9, 1, 'a@mailtest.test', '2026-10-04T08:00:00.000Z', NULL, NULL, 0),
			(10, 1, 'a@mailtest.test', '2026-10-06T08:00:00.000Z', NULL, NULL, 1),
			(11, 2, 'a@mailtest.test', '2026-10-07T08:00:00.000Z', NULL, NULL, 0)`,
		`INSERT INTO message_mailbox (message_id, mailbox_id, uid) VALUES
			(1, 10, 1), (2, 10, 2), (3, 11, 3), (4, 11, 4), (5, 10, 5), (6, 10, 6), (7, 10, 7),
			(8, 10, NULL), (9, 10, NULL), (9, 12, 9), (10, 10, 10), (11, 20, 1)`,
	} {
		if _, err := d.db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return d
}

func TestPrefetchCandidates(t *testing.T) {
	d := prefetchFixture(t)
	since := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	got, err := d.PrefetchCandidates(t.Context(), 1, since, 10)
	if want := []int64{2, 1, 3, 9}; err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("PrefetchCandidates = %v, %v; want %v", got, err, want)
	}
	if got, err := d.PrefetchCandidates(t.Context(), 1, since, 2); err != nil || !reflect.DeepEqual(got, []int64{2, 1}) {
		t.Fatalf("PrefetchCandidates(limit 2) = %v, %v", got, err)
	}
	if got, err := d.PrefetchCandidates(t.Context(), 3, since, 10); err != nil || len(got) != 0 {
		t.Fatalf("PrefetchCandidates(account 3) = %v, %v", got, err)
	}
}
