package mailsync_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/notify"
)

// TestNotifyOffAnnouncesNothing: an account with notifications off
// announces none of its new mail; turned back on, its next mail is
// announced (parity P-804).
func TestNotifyOffAnnouncesNothing(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	announced := make(chan []notify.Mail, 8)
	cfg := testConfig
	cfg.Announce = func(_ context.Context, _ int64, mail []notify.Mail) error {
		announced <- mail
		return nil
	}
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, cfg)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	deliver := func(subject string) {
		t.Helper()
		raw := "From: Ann <ann@x.test>\r\nSubject: " + subject + "\r\n\r\nHello.\r\n"
		if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, Notify: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	deliver("Quiet")
	waitUntil(t, 5*time.Second, "Quiet synced", func() bool { return slices.Contains(h.subjects(h.inbox().ID), "Quiet") })

	// Back on, the next mail is announced; had Quiet been announced it
	// would come first.
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, Notify: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	deliver("Loud")
	select {
	case mail := <-announced:
		if len(mail) != 1 || mail[0].Subject != "Loud" {
			t.Errorf("announced %+v, want only Loud (nothing while notifications were off)", mail)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mail after notifications came back on was not announced")
	}
}
