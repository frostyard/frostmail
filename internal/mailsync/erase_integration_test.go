//go:build integration

package mailsync_test

// Runs against the frostmail-mailtest container (make engine-it) as
// test5@mailtest.test, whose Trash is empty in the clean snapshot. The other
// tests of this package deliver mail to its INBOX, which this test leaves
// alone: it looks only for the messages it appended.

import (
	"fmt"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
)

// TestDovecotEraseOld: with Erasing deleted messages set to After One Week, a
// message 8 days in Trash (by the store's clock) is gone from the server, one
// 6 days there stays, and a message that never left INBOX stays (ADR-0051,
// plan 0014 Phase 4).
func TestDovecotEraseOld(t *testing.T) {
	opts := dovecotTest2(t)
	opts.Username = "test5@mailtest.test"
	ctx := t.Context()
	other, err := imapx.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	// Each run's messages have Message-IDs and subjects of their own, so a
	// Trash that an earlier run left behind does not confuse this one.
	run := time.Now().UnixNano()
	msgid := func(kind string) string { return fmt.Sprintf("<erase-%d-%s@mailtest.test>", run, kind) }
	subject := func(kind string) string { return fmt.Sprintf("erase %s %d", kind, run) }
	for _, kind := range []string{"a", "b", "c"} {
		raw := "From: Ann <ann@mailtest.test>\r\nSubject: " + subject(kind) + "\r\nMessage-ID: " + msgid(kind) +
			"\r\nDate: Thu, 05 Nov 2026 09:00:00 +0000\r\n\r\nbody\r\n"
		if _, err := other.Append(ctx, "INBOX", []byte(raw), nil); err != nil {
			t.Fatal(err)
		}
	}

	// The test's clock: T0 is "now" for the pass, and the messages arrive in
	// INBOX (and get their added_at) 30 days before it.
	t0 := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	clock := newTestClock(t0.AddDate(0, 0, -30))
	h := newHarnessAt(t, opts, opts.Password, clock.Now)
	h.waitPhase(api.SyncPhaseIdle)
	inbox, trash := h.inbox(), h.mailboxByPath("Trash")
	if trash.Role != api.MailboxRoleTrash {
		t.Fatalf("Trash's role = %s, want trash", trash.Role)
	}
	ids := map[string]int64{}
	for _, s := range inboxSummaries(t, h, inbox.ID) {
		for _, kind := range []string{"a", "b", "c"} {
			if s.Subject == subject(kind) {
				ids[kind] = s.ID
			}
		}
	}
	if len(ids) != 3 {
		t.Fatalf("INBOX holds %v of the three messages this test appended", ids)
	}

	// A is deleted 8 days before T0 and B 6 days before it: both move to
	// Trash. C stays in INBOX.
	deleteAt := func(kind string, at time.Time) {
		t.Helper()
		clock.Set(at)
		if err := h.c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: []int64{ids[kind]}}); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, 10*time.Second, "the move of "+kind+" to Trash", func() bool {
			return !h.hasQueuedOps() && h.onlyIn(ids[kind]) == trash.ID
		})
	}
	deleteAt("a", t0.AddDate(0, 0, -8))
	deleteAt("b", t0.AddDate(0, 0, -6))

	// After One Week.
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct,
		EraseTrashDays: ptr(int64(7))}); err != nil {
		t.Fatal(err)
	}
	clock.Set(t0)
	if err := h.srv.Sync.EraseOld(ctx, t0); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	h.waitReplayed()

	// The server's Trash holds B alone, and INBOX still holds C.
	if _, err := other.Select(ctx, "Trash"); err != nil {
		t.Fatal(err)
	}
	st, err := other.Status(ctx, "Trash")
	if err != nil || st.Messages != 1 {
		t.Errorf("Trash on the server = %d messages, %v; want B alone", st.Messages, err)
	}
	for kind, want := range map[string]int{"a": 0, "b": 1} {
		uids, err := other.SearchMessageID(ctx, msgid(kind))
		if err != nil {
			t.Fatal(err)
		}
		if len(uids) != want {
			t.Errorf("%s in the server's Trash = %d messages, want %d", msgid(kind), len(uids), want)
		}
	}
	if _, err := other.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}
	uids, err := other.SearchMessageID(ctx, msgid("c"))
	if err != nil {
		t.Fatal(err)
	}
	if len(uids) != 1 {
		t.Errorf("%s in the server's INBOX = %d messages, want 1", msgid("c"), len(uids))
	}
	if _, err := h.c.Message().Get(ctx, &api.MessageGetParams{ID: ids["a"]}); err == nil {
		t.Error("the erased message is still stored")
	}
	if got := h.subjects(trash.ID); len(got) != 1 || got[0] != subject("b") {
		t.Errorf("Trash after the pass = %q, want B alone", got)
	}
}
