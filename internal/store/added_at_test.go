package store

import (
	"database/sql"
	"testing"
	"time"
)

// addedAt reads one membership's added_at.
func addedAt(t *testing.T, d *DB, messageID, mailboxID int64) sql.NullString {
	t.Helper()
	var v sql.NullString
	err := d.db.QueryRowContext(t.Context(),
		`SELECT added_at FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?`, messageID, mailboxID).Scan(&v)
	if err != nil {
		t.Fatalf("added_at of message %d in mailbox %d: %v", messageID, mailboxID, err)
	}
	return v
}

// An insert sets added_at to now: it is when maild first saw the message in
// that mailbox (ADR-0051).
func TestInsertHeadersSetsAddedAt(t *testing.T) {
	d, _ := openTest(t)
	acct, inbox, _ := mailboxFixture(t, d)
	ids := insertHeaders(t, d, acct, inbox, header(1, "a"))
	if got := addedAt(t, d, ids[0], inbox); got.String != FormatTime(testNow) {
		t.Fatalf("added_at after an insert = %q, want %q", got.String, FormatTime(testNow))
	}
}

// A row that comes to another mailbox restarts added_at; one that stays keeps
// the time it had: a pending row that gets its UID, for one (ADR-0051).
func TestMembershipMovesRestartAddedAt(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, archive := mailboxFixture(t, d)
	ids := insertHeaders(t, d, acct, inbox, header(1, "a"))
	msg := ids[0]
	later := testNow.Add(48 * time.Hour)
	d.Now = func() time.Time { return later }
	if err := d.Tx(ctx, func(tx *Tx) error {
		return tx.MoveMembership(ctx, msg, inbox, archive)
	}); err != nil {
		t.Fatal(err)
	}
	// MoveMembership updates the row in place, so the row is in Archive now.
	if got := addedAt(t, d, msg, archive); got.String != FormatTime(later) {
		t.Fatalf("added_at after a local move = %q, want %q", got.String, FormatTime(later))
	}

	// The server confirmed the move under the same mailbox: the time stands.
	evenLater := testNow.Add(72 * time.Hour)
	d.Now = func() time.Time { return evenLater }
	if err := d.Tx(ctx, func(tx *Tx) error {
		return tx.SetMembership(ctx, msg, archive, 42)
	}); err != nil {
		t.Fatal(err)
	}
	if got := addedAt(t, d, msg, archive); got.String != FormatTime(later) {
		t.Fatalf("added_at after a confirmed move to the same mailbox = %q, want %q", got.String, FormatTime(later))
	}

	// A move the server put in another mailbox than the pending one restarts
	// the time: the message is new in the mailbox it reached.
	if err := d.Tx(ctx, func(tx *Tx) error {
		return tx.MoveMembership(ctx, msg, archive, inbox)
	}); err != nil {
		t.Fatal(err)
	}
	latest := testNow.Add(96 * time.Hour)
	d.Now = func() time.Time { return latest }
	if err := d.Tx(ctx, func(tx *Tx) error {
		return tx.SetMembership(ctx, msg, archive, 7)
	}); err != nil {
		t.Fatal(err)
	}
	if got := addedAt(t, d, msg, archive); got.String != FormatTime(latest) {
		t.Fatalf("added_at after a confirmed move to another mailbox = %q, want %q", got.String, FormatTime(latest))
	}
}

// The erase pass's query takes members before a time, oldest first, and never
// one without added_at or without a UID: a safe failure (ADR-0051).
func TestOldMailboxMembers(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, archive := mailboxFixture(t, d)
	// A Trash mailbox with the trash role, and one member old, one new.
	if _, err := d.db.ExecContext(ctx, `UPDATE mailboxes SET role = 'trash' WHERE id = ?`, archive); err != nil {
		t.Fatal(err)
	}
	d.Now = func() time.Time { return testNow.Add(-8 * 24 * time.Hour) }
	old := insertHeaders(t, d, acct, archive, header(1, "old"))
	d.Now = func() time.Time { return testNow }
	fresh := insertHeaders(t, d, acct, archive, header(2, "fresh"))
	// One with no added_at, and one with no UID (a pending move).
	noTime := insertHeaders(t, d, acct, archive, header(3, "notime"))
	pending := insertHeaders(t, d, acct, inbox, header(4, "pending"))
	if _, err := d.db.ExecContext(ctx, `UPDATE message_mailbox SET added_at = NULL WHERE message_id = ?`, noTime[0]); err != nil {
		t.Fatal(err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error {
		return tx.MoveMembership(ctx, pending[0], inbox, archive)
	}); err != nil {
		t.Fatal(err)
	}

	var got []MailboxMember
	if err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		got, err = tx.OldMailboxMembers(ctx, archive, testNow.Add(-7*24*time.Hour), 1000)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Message != old[0] || got[0].UID != 1 {
		t.Fatalf("old members = %+v, want only message %d (uid 1); fresh %v, no added_at %v, pending %v",
			got, old[0], fresh[0], noTime[0], pending[0])
	}

	// The batch is a limit, oldest first.
	if err := d.Tx(ctx, func(tx *Tx) error {
		got, err := tx.OldMailboxMembers(ctx, archive, testNow, 1)
		if err != nil {
			return err
		}
		if len(got) != 1 || got[0].Message != old[0] {
			t.Fatalf("one per pass = %+v, want message %d", got, old[0])
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestMigration22GivesExistingRowsATime: the migration adds added_at with the
// migration's time for the rows that exist, so nothing is erased before a
// full period after the upgrade, and the two settings start at 0 (Never).
func TestMigration22GivesExistingRowsATime(t *testing.T) {
	ctx := t.Context()
	path, raw := schemaAt(t, 21)
	for _, q := range []string{
		`INSERT INTO accounts (id, kind, email, display_name, auth, imap_host, imap_port, imap_tls, imap_username,
			smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
			VALUES (1, 'imap', 'a@x.test', '', 'password', 'h', 993, 'tls', 'u', 'h', 465, 'tls', 'u', 'x')`,
		`INSERT INTO mailboxes (id, account_id, path, delimiter, name, role, attrs_json, selectable, subscribed)
			VALUES (1, 1, 'INBOX', '/', 'INBOX', 'inbox', '[]', 1, 1)`,
		`INSERT INTO messages (id, account_id, subject, from_name, from_addr, internal_date, preview, size)
			VALUES (7, 1, 's', '', 'b@x.test', '2026-10-01T00:00:00Z', '', 1)`,
		`INSERT INTO message_mailbox (message_id, mailbox_id, uid, modseq) VALUES (7, 1, 3, 1)`,
	} {
		if _, err := raw.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	_ = raw.Close()

	d, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	added := addedAt(t, d, 7, 1)
	if !added.Valid {
		t.Fatal("added_at after the migration is NULL")
	}
	when, err := ParseTime(added.String)
	if err != nil {
		t.Fatalf("added_at %q: %v", added.String, err)
	}
	// It is the migration's own time, not a fixed value: a day old at the
	// latest, and not in the future.
	if age := time.Since(when); age < 0 || age > time.Hour {
		t.Fatalf("added_at = %s, %v old", added.String, age)
	}
	var trash, junk int
	if err := d.db.QueryRowContext(ctx, `SELECT erase_trash_days, erase_junk_days FROM accounts WHERE id = 1`).
		Scan(&trash, &junk); err != nil || trash != 0 || junk != 0 {
		t.Fatalf("settings after the migration = %d, %d, %v; want 0, 0", trash, junk, err)
	}
	// The column's rule: only 0, 1, 7 and 30.
	if _, err := d.db.ExecContext(ctx, `UPDATE accounts SET erase_trash_days = 5 WHERE id = 1`); err == nil {
		t.Fatal("erase_trash_days 5 was accepted")
	}
}
