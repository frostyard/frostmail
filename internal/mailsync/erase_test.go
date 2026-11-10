package mailsync_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/store"
)

// testClock is a clock the test stops and moves while maild's goroutines read
// it: the store's timestamps (message_mailbox.added_at, ADR-0051) and the sync
// window (ADR-0016) follow it.
type testClock struct{ ns atomic.Int64 }

// newTestClock returns a clock stopped at t.
func newTestClock(t time.Time) *testClock {
	c := &testClock{}
	c.Set(t)
	return c
}

// Set moves the clock.
func (c *testClock) Set(t time.Time) { c.ns.Store(t.UnixNano()) }

// Now is the clock's reading.
func (c *testClock) Now() time.Time { return time.Unix(0, c.ns.Load()).UTC() }

// newHarnessAt is newHarnessWith with a clock the test stops: it sets both the
// store's timestamps (message_mailbox.added_at, ADR-0051) and the sync
// window's (ADR-0016).
func newHarnessAt(t *testing.T, opts imapx.DialOptions, password string, now func() time.Time) *harness {
	t.Helper()
	cfg := testConfig
	cfg.Now = now
	return newHarnessWith(t, opts, password, cfg)
}

// appendNew puts a message in a mailbox with a Message-ID and a subject of
// its own.
func appendNew(t *testing.T, mem *imapxtest.Mem, mailbox, subject string) {
	t.Helper()
	raw := "From: Ann <ann@x.test>\r\nSubject: " + subject + "\r\nMessage-ID: <" + subject + "@x.test>\r\n\r\nHello.\r\n"
	if _, err := mem.User.Append(mailbox, bytes.NewReader([]byte(raw)), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
}

// storedID returns the stored ID of the message with a Message-ID.
func storedID(t *testing.T, db *store.DB, msgid string) int64 {
	t.Helper()
	ctx := t.Context()
	var id int64
	err := db.Tx(ctx, func(tx *store.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT id FROM messages WHERE msgid_hdr = ?`, msgid).Scan(&id)
	})
	if err != nil {
		t.Fatalf("message %s: %v", msgid, err)
	}
	return id
}

// hidden counts the stored messages maild hides until an expunge.
func hidden(t *testing.T, db *store.DB) int {
	t.Helper()
	ctx := t.Context()
	var n int
	if err := db.Tx(ctx, func(tx *store.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE deleted = 1`).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

// members counts the messages a mailbox holds in the store.
func members(t *testing.T, db *store.DB, mailboxID int64) int {
	t.Helper()
	ctx := t.Context()
	var n int
	if err := db.Tx(ctx, func(tx *store.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?`, mailboxID).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

// storedUIDValidity is the UIDVALIDITY maild stored for a mailbox, 0 for one
// it never selected.
func storedUIDValidity(t *testing.T, db *store.DB, mailboxID int64) uint32 {
	t.Helper()
	ctx := t.Context()
	var v sql.NullInt64
	err := db.Tx(ctx, func(tx *store.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT uidvalidity FROM mailboxes WHERE id = ?`, mailboxID).Scan(&v)
	})
	if err != nil {
		t.Fatalf("stored uidvalidity of mailbox %d: %v", mailboxID, err)
	}
	if !v.Valid || v.Int64 < 0 {
		return 0
	}
	return uint32(v.Int64)
}

// holds reports whether a mailbox holds the message with this Message-ID and
// shows it (it is not hidden until an expunge).
func holds(t *testing.T, db *store.DB, mailboxID int64, msgid string) bool {
	t.Helper()
	ctx := t.Context()
	var n int
	err := db.Tx(ctx, func(tx *store.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_mailbox mm
			JOIN messages m ON m.id = mm.message_id
			WHERE mm.mailbox_id = ? AND m.msgid_hdr = ? AND m.deleted = 0`, mailboxID, msgid).Scan(&n)
	})
	if err != nil {
		t.Fatalf("message %s in mailbox %d: %v", msgid, mailboxID, err)
	}
	return n > 0
}

// addedAtIn is the added_at of the message with this Message-ID in a mailbox.
func addedAtIn(t *testing.T, db *store.DB, mailboxID int64, msgid string) string {
	t.Helper()
	ctx := t.Context()
	var v sql.NullString
	err := db.Tx(ctx, func(tx *store.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT mm.added_at FROM message_mailbox mm
			JOIN messages m ON m.id = mm.message_id
			WHERE mm.mailbox_id = ? AND m.msgid_hdr = ?`, mailboxID, msgid).Scan(&v)
	})
	if err != nil {
		t.Fatalf("added_at of %s in mailbox %d: %v", msgid, mailboxID, err)
	}
	return v.String
}

// imapxSession opens a second IMAP session to the test server, for what the
// server itself holds.
func imapxSession(t *testing.T, mem *imapxtest.Mem) *imapx.Session {
	t.Helper()
	s, err := imapx.Open(t.Context(), mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// hasQueuedOps reports whether the account has an op waiting for the server.
func (h *harness) hasQueuedOps() bool {
	busy, err := h.srv.DB.HasQueuedOps(h.t.Context(), h.acct)
	return err == nil && busy
}

// onlyIn is the mailbox a message is in, or 0 when it is in more than one.
func (h *harness) onlyIn(id int64) int64 {
	h.t.Helper()
	m, err := h.c.Message().Get(h.t.Context(), &api.MessageGetParams{ID: id})
	if err != nil {
		return 0
	}
	if len(m.Summary.MailboxIDs) != 1 {
		return 0
	}
	return m.Summary.MailboxIDs[0]
}

// TestEraseOld: with Erasing deleted messages set to After One Week and Junk
// to after 30 days, a message 8 days in Trash is erased, from the store and
// from the server; one 6 days there stays, and one that came there a day ago
// after a month in INBOX. An old message in INBOX and in a folder of the
// user's own stays, and Junk follows its own setting (ADR-0051).
func TestEraseOld(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	// The store's clock: T0 is "now" for the pass, and the account's mail is
	// 30 days old in the mailboxes it arrived in.
	t0 := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	clock := newTestClock(t0.AddDate(0, 0, -30))
	if err := mem.User.Create("Receipts", nil); err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"a", "b", "c", "d"} {
		appendNew(t, mem, "INBOX", subject)
	}
	appendNew(t, mem, "Receipts", "e")
	appendNew(t, mem, "Junk", "f")
	h := newHarnessAt(t, mem.DialOptions(), imapxtest.Password, clock.Now)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	trash, junk, receipts := h.mailboxByPath("Trash"), h.mailboxByPath("Junk"), h.mailboxByPath("Receipts")
	if trash.Role != api.MailboxRoleTrash || junk.Role != api.MailboxRoleJunk {
		t.Fatalf("roles: Trash %s, Junk %s", trash.Role, junk.Role)
	}

	// A comes to Trash 8 days before T0 and B 6 days before it.
	toTrash := func(subject string, at time.Time) {
		t.Helper()
		clock.Set(at)
		id := storedID(t, h.srv.DB, subject+"@x.test")
		if err := h.c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: []int64{id}}); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, 5*time.Second, "the move of "+subject+" to Trash", func() bool {
			return !h.hasQueuedOps() && h.onlyIn(id) == trash.ID
		})
	}
	toTrash("a", t0.AddDate(0, 0, -8))
	toTrash("b", t0.AddDate(0, 0, -6))
	a := storedID(t, h.srv.DB, "a@x.test")
	f := storedID(t, h.srv.DB, "f@x.test")
	if got := serverCount(t, mem, "Trash"); got != 2 {
		t.Fatalf("Trash on the server = %d, want 2", got)
	}

	// D comes to Trash a day before T0, after a month in INBOX: its age in
	// Trash is 1 day, not the message's.
	toTrash("d", t0.AddDate(0, 0, -1))

	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct,
		EraseTrashDays: ptr(int64(7)), EraseJunkDays: ptr(int64(30))}); err != nil {
		t.Fatal(err)
	}
	clock.Set(t0)
	if err := h.srv.Sync.EraseOld(ctx, t0); err != nil {
		t.Fatal(err)
	}
	// A goes (8 days in Trash) and F (30 days in Junk); B (6 days) and D
	// (1 day) stay.
	waitUntil(t, 5*time.Second, "A erased from the server", func() bool { return serverCount(t, mem, "Trash") == 2 })
	waitUntil(t, 5*time.Second, "F erased from the server", func() bool { return serverCount(t, mem, "Junk") == 0 })
	h.waitReplayed()
	for _, id := range []int64{a, f} {
		if _, err := h.c.Message().Get(ctx, &api.MessageGetParams{ID: id}); err == nil {
			t.Error("an erased message is still stored")
		}
	}
	if got := h.subjects(trash.ID); len(got) != 2 {
		t.Errorf("Trash after the pass = %q, want B and D", got)
	}
	for _, mb := range []struct {
		name string
		id   int64
		want []string
	}{
		{"INBOX", h.inbox().ID, []string{"c"}},
		{"Receipts", receipts.ID, []string{"e"}},
	} {
		if got := h.subjects(mb.id); len(got) != len(mb.want) || got[0] != mb.want[0] {
			t.Errorf("%s after the pass = %q, want %q", mb.name, got, mb.want)
		}
	}

	// Never erases nothing, however old the mail becomes.
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct,
		EraseTrashDays: ptr(int64(0)), EraseJunkDays: ptr(int64(0))}); err != nil {
		t.Fatal(err)
	}
	clock.Set(t0.AddDate(0, 0, 30))
	if err := h.srv.Sync.EraseOld(ctx, clock.Now()); err != nil {
		t.Fatal(err)
	}
	if n := hidden(t, h.srv.DB); n != 0 {
		t.Errorf("hidden messages with Never set = %d, want 0", n)
	}

	// A read-only account's mail is never erased, whatever its settings say.
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct,
		EraseTrashDays: ptr(int64(1)), ReadOnly: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	clock.Set(t0.AddDate(0, 0, 60))
	if err := h.srv.Sync.EraseOld(ctx, clock.Now()); err != nil {
		t.Fatal(err)
	}
	if n := hidden(t, h.srv.DB); n != 0 {
		t.Errorf("hidden messages in a read-only account = %d, want 0", n)
	}
}

// TestEraseOldBatch: one pass erases at most 1,000 messages of a mailbox, and
// the next pass takes the rest (ADR-0051). It runs the pass with no actor, so
// that what it queued is what the test reads.
func TestEraseOldBatch(t *testing.T) {
	ctx := t.Context()
	t0 := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.Now = func() time.Time { return t0.AddDate(0, 0, -8) }

	var trash int64
	err = db.Tx(ctx, func(tx *store.Tx) error {
		server := store.ServerConfig{Host: "imap.example", Port: 993, TLS: api.TLSModeTLS, Username: "u"}
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "ann@x.test",
			Auth: api.AuthKindPassword, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, acct.ID, []store.ServerMailbox{
			{Path: "INBOX", Delimiter: "/", Role: api.MailboxRoleInbox, Selectable: true},
			{Path: "Trash", Delimiter: "/", Role: api.MailboxRoleTrash, Selectable: true},
		})
		if err != nil {
			return err
		}
		for _, mb := range mbs {
			if mb.Role == api.MailboxRoleTrash {
				trash = mb.ID
			}
		}
		const n = 1001
		for i := range n {
			hs := []store.MessageHeader{{UID: uint32(100000 + i), MessageID: fmt.Sprintf("batch-%d@x.test", i),
				InternalDate: t0.AddDate(0, 0, -8), Size: 10, Subject: fmt.Sprintf("old %d", i),
				From: store.Address{Addr: "ann@x.test"}}}
			if _, err := tx.InsertHeaders(ctx, acct.ID, trash, hs); err != nil {
				return err
			}
		}
		days := 7
		_, err = tx.UpdateAccount(ctx, acct.ID, store.AccountUpdate{EraseTrashDays: &days})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := members(t, db, trash); got != 1001 {
		t.Fatalf("Trash holds %d messages, want 1001", got)
	}

	m := mailsync.New(db, nil, blob.New(filepath.Join(dir, "blobs")), slog.New(slog.DiscardHandler), mailsync.Config{},
		func([]api.EventEnvelope) {})
	// No actor runs, so nothing replays: the pass hides what it takes.
	if err := m.EraseOld(ctx, t0); err != nil {
		t.Fatal(err)
	}
	if got := hidden(t, db); got != 1000 {
		t.Fatalf("one pass hid %d messages, want 1,000", got)
	}
	if err := m.EraseOld(ctx, t0); err != nil {
		t.Fatal(err)
	}
	if got := hidden(t, db); got != 1001 {
		t.Fatalf("two passes hid %d messages, want 1001", got)
	}
}

// TestEraseOldStaleUIDValidity: an expunge op records its mailbox's
// UIDVALIDITY when it is queued, and replays only while the server still has
// it. Here the mailbox was renumbered: the stored UID names a message the
// user never saw in Frostmail, so the op erases nothing on the server, its
// message shows again, and the next full pass resyncs the mailbox (ADR-0051).
func TestEraseOldStaleUIDValidity(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	t0 := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	// A message that arrives in Trash 8 days before T0, synced at once: it is
	// what the erase pass picks.
	appendNew(t, mem, "Trash", "trashed")
	clock := newTestClock(t0.AddDate(0, 0, -8))
	h := newHarnessAt(t, mem.DialOptions(), imapxtest.Password, clock.Now)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	trash := h.mailboxByPath("Trash")
	if got := serverCount(t, mem, "Trash"); got != 1 {
		t.Fatalf("the server's Trash holds %d messages, want 1", got)
	}
	storedID(t, h.srv.DB, "trashed@x.test") // synced, with an added_at of 8 days ago

	// Wait until the account's running actor is idle, so that no pass is in
	// flight: one that reconciles Trash after the forgery below would store
	// the server's UIDVALIDITY again and the op would not be stale.
	waitUntil(t, 10*time.Second, "the account's actor to be idle", func() bool {
		st, err := h.c.Sync().Status(ctx, &api.SyncStatusParams{AccountID: &h.acct})
		return err == nil && len(st) == 1 && st[0].Phase == api.SyncPhaseIdle
	})
	// After One Week, stored without account.update: that call would restart
	// the account's actor and run a pass of its own over Trash.
	days := 7
	if err := h.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.UpdateAccount(ctx, h.acct, store.AccountUpdate{EraseTrashDays: &days})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// The mailbox the server renumbered: the stored UIDVALIDITY is one the
	// server does not have, so its stored UID would name another message.
	err := h.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE mailboxes SET uidvalidity = 999 WHERE id = ?`, trash.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if st, ok, err := h.srv.DB.MailboxSyncState(ctx, trash.ID); err != nil || !ok || st.UIDValidity != 999 {
		t.Fatalf("Trash's stored uidvalidity = %d (%v), %v; want the forged 999", st.UIDValidity, ok, err)
	}

	clock.Set(t0)
	if err := h.srv.Sync.EraseOld(ctx, t0); err != nil {
		t.Fatal(err)
	}
	// The op fails at replay, before any STORE or EXPUNGE, and the pass's
	// hiding is undone in that same transaction: once no op is queued, the
	// message is shown again. It is asserted by Message-ID, not by row ID, so
	// that a resync that drops and fetches the row cannot matter here.
	waitUntil(t, 5*time.Second, "the refused expunge op", func() bool {
		return !h.hasQueuedOps()
	})
	if !holds(t, h.srv.DB, trash.ID, "trashed@x.test") {
		t.Error("the message the refused op hid is not shown again")
	}
	// The guard held: the message the stale UID would have named is still on
	// the server.
	if got := serverCount(t, mem, "Trash"); got != 1 {
		t.Fatalf("the server's Trash holds %d messages, want 1: the stale UID erased a message", got)
	}

	// Before the next pass the stored UIDVALIDITY is still the forged one and
	// the message's added_at is still the one from 8 days ago.
	if got := storedUIDValidity(t, h.srv.DB, trash.ID); got != 999 {
		t.Fatalf("Trash's stored uidvalidity before the next pass = %d, want the forged 999", got)
	}
	if got := addedAtIn(t, h.srv.DB, trash.ID, "trashed@x.test"); got != store.FormatTime(t0.AddDate(0, 0, -8)) {
		t.Fatalf("added_at before the next pass = %q, want the 8-day-old one", got)
	}

	// The next full pass resyncs the renumbered mailbox: the stored
	// UIDVALIDITY is the server's again, the message is stored in Trash, and
	// its added_at is the clock's time at the resync, not the 8-day-old one
	// (ADR-0051: a renumbered mailbox makes new rows).
	server := imapxSession(t, mem)
	defer server.Close()
	sel, err := server.Select(ctx, "Trash")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 10*time.Second, "the resynced Trash", func() bool {
		return storedUIDValidity(t, h.srv.DB, trash.ID) == sel.UIDValidity &&
			addedAtIn(t, h.srv.DB, trash.ID, "trashed@x.test") == store.FormatTime(t0)
	})
	if !holds(t, h.srv.DB, trash.ID, "trashed@x.test") {
		t.Error("the server's message is not in the store's Trash after the resync")
	}
}

// TestEraseOldReadsSettingsInTheTransaction: the pass takes each mailbox's
// cutoff from the account's settings as they are in the transaction that
// erases, so an account.update between the pass's list read and that
// transaction wins. A message 8 days in Trash survives when the user turns
// erasing off, or sets 30 days, in between (ADR-0051; AGENTS.md, the rule that
// an update of optional fields reads the row in the transaction that acts on
// it).
func TestEraseOldReadsSettingsInTheTransaction(t *testing.T) {
	for _, c := range []struct {
		name string
		days int
	}{{"Never", 0}, {"30 days", 30}} {
		t.Run(c.name, func(t *testing.T) {
			mem := imapxtest.StartMemFull(t)
			t0 := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
			// A message that came to Trash 8 days before T0, and After One
			// Week as the setting: the pass would erase it.
			appendNew(t, mem, "Trash", "trashed")
			clock := newTestClock(t0.AddDate(0, 0, -8))
			h := newHarnessAt(t, mem.DialOptions(), imapxtest.Password, clock.Now)
			h.waitPhase(api.SyncPhaseIdle)
			ctx := t.Context()
			if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct,
				EraseTrashDays: ptr(int64(7))}); err != nil {
				t.Fatal(err)
			}
			h.waitPhase(api.SyncPhaseIdle)
			storedID(t, h.srv.DB, "trashed@x.test") // synced, 8 days in Trash

			// The user changes the setting while the pass runs: the hook runs
			// after the pass has listed the mailboxes and before the
			// transaction that erases.
			days := c.days
			mailsync.SetEraseHook(h.srv.Sync, func(ctx context.Context, accountID int64) {
				mailsync.SetEraseHook(h.srv.Sync, nil) // this pass only
				err := h.srv.DB.Tx(ctx, func(tx *store.Tx) error {
					_, err := tx.UpdateAccount(ctx, accountID, store.AccountUpdate{EraseTrashDays: &days})
					return err
				})
				if err != nil {
					t.Errorf("change the erase setting in the pass: %v", err)
				}
			})
			clock.Set(t0)
			if err := h.srv.Sync.EraseOld(ctx, t0); err != nil {
				t.Fatal(err)
			}
			if n := hidden(t, h.srv.DB); n != 0 {
				t.Errorf("hidden messages = %d, want 0: the pass erased with the setting from before the update", n)
			}
			if h.hasQueuedOps() {
				t.Error("an expunge op was queued")
			}
			if got := serverCount(t, mem, "Trash"); got != 1 {
				t.Errorf("the server's Trash holds %d messages, want 1", got)
			}
		})
	}
}
