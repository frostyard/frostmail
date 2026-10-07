package store

// CONTRACT TEST for task card T-0012 (docs/tasks). Do not edit.

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// addAccount inserts an account row directly.
func addAccount(t *testing.T, d *DB, email string) int64 {
	t.Helper()
	res, err := d.db.ExecContext(t.Context(), `INSERT INTO accounts (kind, email, auth, imap_host, imap_port, imap_tls,
		imap_username, smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
		VALUES ('imap', ?, 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now')`, email)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func replace(t *testing.T, d *DB, accountID int64, list []ServerMailbox) []Mailbox {
	t.Helper()
	ctx := t.Context()
	var out []Mailbox
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		out, err = tx.ReplaceMailboxes(ctx, accountID, list)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var serverList = []ServerMailbox{
	{Path: "INBOX", Delimiter: "/", Role: api.MailboxRoleInbox, Attrs: []string{`\HasNoChildren`}, Selectable: true, Subscribed: true},
	{Path: "Sent", Delimiter: "/", Role: api.MailboxRoleSent, Attrs: []string{`\Sent`}, Selectable: true, Subscribed: true},
	{Path: "Archive/2026", Delimiter: "/", Role: api.MailboxRoleNone, Selectable: true},
	{Path: "Projects", Delimiter: "", Role: api.MailboxRoleNone, Attrs: []string{`\Noselect`}},
}

func paths(mbs []Mailbox) []string {
	var out []string
	for _, mb := range mbs {
		out = append(out, mb.Path)
	}
	return out
}

func TestReplaceMailboxesInserts(t *testing.T) {
	d, events := openTest(t)
	acct := addAccount(t, d, "a@mailtest.test")
	other := addAccount(t, d, "b@mailtest.test")
	replace(t, d, other, []ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
	*events = nil

	got := replace(t, d, acct, serverList)
	if want := []string{"INBOX", "Sent", "Archive/2026", "Projects"}; !slices.Equal(paths(got), want) {
		t.Fatalf("mailboxes = %v, want %v (ListMailboxes order, this account only)", paths(got), want)
	}
	names := map[string]string{}
	for _, mb := range got {
		names[mb.Path] = mb.Name
		if mb.AccountID != acct || mb.ID == 0 {
			t.Fatalf("mailbox %+v", mb)
		}
	}
	if names["Archive/2026"] != "2026" || names["Projects"] != "Projects" || names["INBOX"] != "INBOX" {
		t.Fatalf("names = %v", names)
	}
	if got[2].Delimiter != "/" || got[3].Delimiter != "" || got[1].Role != api.MailboxRoleSent {
		t.Fatalf("fields = %+v", got)
	}
	var attrs string
	var selectable, subscribed int
	if err := d.db.QueryRowContext(t.Context(), `SELECT attrs_json, selectable, subscribed FROM mailboxes WHERE path = 'Projects'`).Scan(&attrs, &selectable, &subscribed); err != nil {
		t.Fatal(err)
	}
	if attrs != `["\\Noselect"]` || selectable != 0 || subscribed != 0 {
		t.Fatalf("Projects row: attrs %s selectable %d subscribed %d", attrs, selectable, subscribed)
	}
	if len(*events) != 4 {
		t.Fatalf("events = %+v, want 4 mailbox.changed", *events)
	}
	for _, ev := range *events {
		e, err := api.DecodeEvent(ev.Event, ev.Data)
		if err != nil || e.(api.MailboxChanged).AccountID != acct || e.(api.MailboxChanged).Deleted {
			t.Fatalf("event %+v (%v)", ev, err)
		}
	}
}

func TestReplaceMailboxesUpdatesAndDeletes(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	acct := addAccount(t, d, "a@mailtest.test")
	first := replace(t, d, acct, serverList)
	ids := map[string]int64{}
	for _, mb := range first {
		ids[mb.Path] = mb.ID
	}
	// A message only in Sent, and one in Sent and INBOX.
	err := d.Tx(ctx, func(tx *Tx) error {
		for _, q := range []string{
			`INSERT INTO messages (id, account_id, internal_date) VALUES (101, ?, 'x')`,
			`INSERT INTO messages (id, account_id, internal_date) VALUES (102, ?, 'x')`,
		} {
			if _, err := tx.ExecContext(ctx, q, acct); err != nil {
				return err
			}
		}
		for _, q := range []string{
			`INSERT INTO message_mailbox (message_id, mailbox_id, uid) VALUES (101, ?, 1)`,
			`INSERT INTO message_mailbox (message_id, mailbox_id, uid) VALUES (102, ?, 2)`,
		} {
			if _, err := tx.ExecContext(ctx, q, ids["Sent"]); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO message_mailbox (message_id, mailbox_id, uid) VALUES (102, ?, 9)`, ids["INBOX"]); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO messages_fts (rowid, subject) VALUES (101, 'gone soon')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	*events = nil

	next := []ServerMailbox{serverList[0], serverList[2], serverList[3]} // Sent disappeared
	next[1].Subscribed = true                                            // Archive/2026 changed
	got := replace(t, d, acct, next)
	if want := []string{"INBOX", "Archive/2026", "Projects"}; !slices.Equal(paths(got), want) {
		t.Fatalf("mailboxes = %v, want %v", paths(got), want)
	}
	if got[0].ID != ids["INBOX"] || got[1].ID != ids["Archive/2026"] {
		t.Fatal("existing mailboxes must keep their IDs")
	}
	var changed, deleted []int64
	var removed []int64
	for _, ev := range *events {
		e, err := api.DecodeEvent(ev.Event, ev.Data)
		if err != nil {
			t.Fatal(err)
		}
		switch e := e.(type) {
		case api.MailboxChanged:
			if e.Deleted {
				deleted = append(deleted, e.ID)
			} else {
				changed = append(changed, e.ID)
			}
		case api.MessageRemoved:
			removed = append(removed, e.IDs...)
		}
	}
	if !slices.Equal(changed, []int64{ids["Archive/2026"]}) || !slices.Equal(deleted, []int64{ids["Sent"]}) {
		t.Fatalf("changed %v deleted %v; want only Archive/2026 changed and Sent deleted", changed, deleted)
	}
	if !slices.Equal(removed, []int64{101}) {
		t.Fatalf("removed messages = %v, want [101] (102 is still in INBOX)", removed)
	}
	var n int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE id = 101`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("message 101 rows = %d, %v", n, err)
	}
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'gone'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("search entry of 101 survived: %d, %v", n, err)
	}

	*events = nil
	replace(t, d, acct, next)
	if len(*events) != 0 {
		t.Fatalf("an unchanged list emitted %+v", *events)
	}
}

func TestMailboxSyncState(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	acct := addAccount(t, d, "a@mailtest.test")
	mb := replace(t, d, acct, serverList[:1])[0]
	*events = nil

	if _, ok, err := d.MailboxSyncState(ctx, mb.ID); ok || err != nil {
		t.Fatalf("fresh state ok=%v err=%v, want ok=false", ok, err)
	}
	when := time.Date(2026, 10, 7, 21, 30, 0, 0, time.UTC)
	want := SyncState{UIDValidity: 4000000001, UIDNext: 50001, HighestModSeq: 1 << 40, ServerCount: 49990, LastSyncAt: when}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.SetMailboxSyncState(ctx, mb.ID, want) }); err != nil {
		t.Fatal(err)
	}
	got, ok, err := d.MailboxSyncState(ctx, mb.ID)
	if err != nil || !ok || got != want {
		t.Fatalf("state = %+v ok=%v err=%v, want %+v", got, ok, err, want)
	}
	noTime := SyncState{UIDValidity: 1, UIDNext: 2}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.SetMailboxSyncState(ctx, mb.ID, noTime) }); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := d.MailboxSyncState(ctx, mb.ID); got != noTime {
		t.Fatalf("state with zero LastSyncAt = %+v", got)
	}
	if len(*events) != 0 {
		t.Fatalf("sync state emitted %+v", *events)
	}
	if _, _, err := d.MailboxSyncState(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown mailbox state = %v, want ErrNotFound", err)
	}
	err = d.Tx(ctx, func(tx *Tx) error { return tx.SetMailboxSyncState(ctx, 999, want) })
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("set on unknown mailbox = %v, want ErrNotFound", err)
	}
}
