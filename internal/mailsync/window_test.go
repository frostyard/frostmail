package mailsync_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
)

// The sync window (ADR-0016): an account keeps the messages that arrived
// in its last syncDays days. Narrowing the window drops older mail from
// the store, not from the server; widening it fetches that mail again, and
// verify compares the window the last pass kept.
func TestSyncWindowNarrowsAndWidens(t *testing.T) {
	mem := imapxtest.StartMem(t)
	now := time.Now()
	for _, m := range []struct {
		subject string
		arrived time.Time
	}{{"Old", now.AddDate(0, 0, -400)}, {"Last month", now.AddDate(0, 0, -30)}, {"Today", now}} {
		raw := "From: Ann <ann@x.test>\r\nSubject: " + m.subject + "\r\nMessage-ID: <" + strings.ReplaceAll(m.subject, " ", ".") +
			"@x.test>\r\n\r\nHello.\r\n"
		if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{Time: m.arrived}); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	inbox := h.inbox()
	ctx := t.Context()
	has := func(want ...string) func() bool {
		return func() bool {
			got := h.subjects(inbox.ID)
			slices.Sort(got)
			slices.Sort(want)
			return slices.Equal(got, want)
		}
	}
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
