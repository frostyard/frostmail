package mailsync

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/store"
)

// TestReconcileBeforeAMoveReplays: a pass over a mailbox that a queued
// move is taking a message out of does not take the message, still there
// on the server, for new mail; the move then replays as usual.
func TestReconcileBeforeAMoveReplays(t *testing.T) {
	ctx := t.Context()
	mem := imapxtest.StartMemFull(t)
	if err := mem.User.Create("Receipts", nil); err != nil {
		t.Fatal(err)
	}
	raw := "From: Ann <ann@x.test>\r\nSubject: Receipt\r\nMessage-ID: <r1@x.test>\r\n\r\nPaid.\r\n"
	if _, err := mem.User.Append("INBOX", bytes.NewReader([]byte(raw)), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := New(db, nil, blob.New(filepath.Join(dir, "blobs")), slog.New(slog.DiscardHandler), Config{Chunk: 10}, func([]api.EventEnvelope) {})
	o := mem.DialOptions()
	server := store.ServerConfig{Host: o.Host, Port: o.Port, TLS: o.TLS, Username: o.Username}
	var acct store.Account
	mbs := map[string]store.Mailbox{}
	err = db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		if acct, err = tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "ann@x.test",
			Auth: api.AuthKindPassword, IMAP: server, SMTP: server}); err != nil {
			return err
		}
		list, err := tx.ReplaceMailboxes(ctx, acct.ID, []store.ServerMailbox{
			{Path: "INBOX", Delimiter: "/", Role: api.MailboxRoleInbox, Selectable: true},
			{Path: "Receipts", Delimiter: "/", Role: api.MailboxRoleNone, Selectable: true},
		})
		for _, mb := range list {
			mbs[mb.Path] = mb
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	a := &actor{m: m, acct: acct, unsaved: map[int64]time.Time{}}
	cmd, err := imapx.Open(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	defer cmd.Close()
	count := func() int {
		var n int
		if err := db.Tx(ctx, func(tx *store.Tx) error {
			return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	reconcile := func(path string) {
		t.Helper()
		if err := a.reconcile(ctx, cmd, mbs[path]); err != nil {
			t.Fatal(err)
		}
	}
	reconcile("INBOX")
	uids, err := db.MailboxUIDs(ctx, mbs["INBOX"].ID)
	if err != nil || len(uids) != 1 || count() != 1 {
		t.Fatalf("after the first pass: %v, %v, %d messages", uids, err, count())
	}
	var id int64
	if err := db.Tx(ctx, func(tx *store.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT id FROM messages`).Scan(&id)
	}); err != nil {
		t.Fatal(err)
	}

	if err := m.Move(ctx, []int64{id}, 0, mbs["Receipts"].ID); err != nil {
		t.Fatal(err)
	}
	reconcile("INBOX") // the move has not replayed: the server still has the message in INBOX
	if n := count(); n != 1 {
		t.Fatalf("%d messages after a pass before the move replayed, want 1", n)
	}
	if err := a.replay(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	reconcile("INBOX")
	reconcile("Receipts")
	if n := count(); n != 1 {
		t.Fatalf("%d messages after the replay, want 1", n)
	}
	if got, err := db.MailboxUIDs(ctx, mbs["Receipts"].ID); err != nil || len(got) != 1 {
		t.Fatalf("Receipts = %v, %v", got, err)
	}
}
