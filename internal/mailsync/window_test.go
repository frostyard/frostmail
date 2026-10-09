package mailsync_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/store"
)

// The sync window (ADR-0016): an account keeps the messages that arrived
// in its last syncDays days. Narrowing the window drops older mail from
// the store, not from the server; widening it fetches that mail again, and
// verify compares the window the last pass kept.
func TestSyncWindowNarrowsAndWidens(t *testing.T) {
	mem := imapxtest.StartMem(t)
	now := time.Now()
	appendArrived(t, mem, "Old", now.AddDate(0, 0, -400))
	appendArrived(t, mem, "Last month", now.AddDate(0, 0, -30))
	appendArrived(t, mem, "Today", now)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	inbox := h.inbox()
	ctx := t.Context()
	has := h.inboxHas(inbox.ID)
	if !has("Last month", "Old", "Today")() {
		t.Fatalf("without a window INBOX = %q", h.subjects(inbox.ID))
	}
	verified := func(server int) {
		t.Helper()
		r, err := h.c.Account().Verify(ctx, &api.AccountVerifyParams{ID: h.acct})
		if err != nil {
			t.Fatal(err)
		}
		for _, mb := range r.Mailboxes {
			if mb.MailboxID == inbox.ID && (!r.Ok || mb.Server != int64(server) || mb.Local != int64(server)) {
				t.Errorf("verify INBOX = %+v (ok %v), want %d on both sides", mb, r.Ok, server)
			}
		}
	}

	a, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, SyncDays: ptr(int64(365))})
	if err != nil || a.SyncDays != 365 {
		t.Fatalf("update = %+v, %v", a, err)
	}
	waitUntil(t, 5*time.Second, "the old message to leave the store", has("Last month", "Today"))
	verified(2)
	if st, err := mem.User.Status("INBOX", &imap.StatusOptions{NumMessages: true}); err != nil || *st.NumMessages != 3 {
		t.Errorf("the server holds %v messages (%v), want all 3: the window never deletes on the server", st, err)
	}

	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, SyncDays: ptr(int64(0))}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "the old message to come back", has("Last month", "Old", "Today"))
	verified(3)

	for _, bad := range []int64{-1, 36501} {
		if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, SyncDays: ptr(bad)}); !isCode(err, api.CodeInvalidParams) {
			t.Errorf("syncDays %d = %v, want invalid params", bad, err)
		}
	}
}

// TestSyncWindowSurvivesAPassInFlight: a pass that searched the old window
// and ends after the window changed does not store its sync state, which
// would undo the reset and let the new actor's pass take the fast path.
func TestSyncWindowSurvivesAPassInFlight(t *testing.T) {
	mem := imapxtest.StartMem(t)
	now := time.Now()
	appendArrived(t, mem, "Old", now.AddDate(0, 0, -400))
	appendArrived(t, mem, "Today", now)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	inbox := h.inbox()
	ctx := t.Context()
	has := h.inboxHas(inbox.ID)

	// account.update without its restart: the old actor, still keeping
	// every message, runs a whole pass after the window changed.
	err := h.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.UpdateAccount(ctx, h.acct, store.AccountUpdate{SyncDays: ptr(365)})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.srv.Sync.SyncNow(h.acct); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseListing)
	h.waitPhase(api.SyncPhaseIdle)
	if !has("Old", "Today")() {
		t.Fatalf("after the old actor's pass INBOX = %q", h.subjects(inbox.ID))
	}

	if err := h.srv.Sync.Restart(ctx, h.acct); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "the old message to leave the store", has("Today"))
}

// appendArrived puts a message that arrived at a time in the server's INBOX.
func appendArrived(t *testing.T, mem *imapxtest.Mem, subject string, arrived time.Time) {
	t.Helper()
	raw := "From: Ann <ann@x.test>\r\nSubject: " + subject + "\r\nMessage-ID: <" + strings.ReplaceAll(subject, " ", ".") +
		"@x.test>\r\n\r\nHello.\r\n"
	if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{Time: arrived}); err != nil {
		t.Fatal(err)
	}
}

// inboxHas reports, when called, whether a mailbox holds exactly these subjects.
func (h *harness) inboxHas(mailboxID int64) func(want ...string) func() bool {
	return func(want ...string) func() bool {
		return func() bool {
			got := h.subjects(mailboxID)
			slices.Sort(got)
			slices.Sort(want)
			return slices.Equal(got, want)
		}
	}
}
