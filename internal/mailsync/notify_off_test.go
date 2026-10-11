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

// TestNotifyScope: under the VIPs scope only a VIP's new inbox mail
// notifies; under All Mailboxes new mail in any folder but Junk, Trash,
// Sent and Drafts does (docs/design/organize.md, Notifications).
func TestNotifyScope(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	if err := mem.User.Create("Lists", nil); err != nil {
		t.Fatal(err)
	}
	announced := make(chan []notify.Mail, 8)
	cfg := testConfig
	cfg.Announce = func(_ context.Context, _ int64, mail []notify.Mail) error {
		announced <- mail
		return nil
	}
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, cfg)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	deliver := func(folder, from, subject string) {
		t.Helper()
		raw := "From: " + from + "\r\nSubject: " + subject + "\r\n\r\nHello.\r\n"
		if _, err := mem.User.Append(folder, strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
			t.Fatal(err)
		}
	}
	next := func(want string) {
		t.Helper()
		select {
		case mail := <-announced:
			if len(mail) != 1 || mail[0].Subject != want {
				t.Errorf("announced %+v, want only %s", mail, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s was not announced", want)
		}
	}

	if _, err := h.c.Vip().Add(ctx, &api.VipAddParams{Addresses: []string{"Ann@X.test"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Settings().Set(ctx, &api.SettingsSetParams{NotifyScope: ptr(api.NotifyScopeVips)}); err != nil {
		t.Fatal(err)
	}
	deliver("INBOX", "Bob <bob@x.test>", "From Bob")
	deliver("INBOX", "Ann <ann@x.test>", "From Ann")
	next("From Ann") // had Bob's been announced, it would come first

	if _, err := h.c.Settings().Set(ctx, &api.SettingsSetParams{NotifyScope: ptr(api.NotifyScopeAll)}); err != nil {
		t.Fatal(err)
	}
	deliver("Lists", "Bob <bob@x.test>", "A list")
	next("A list")

	if _, err := h.c.Settings().Set(ctx, &api.SettingsSetParams{NotifyScope: ptr(api.NotifyScopeInbox)}); err != nil {
		t.Fatal(err)
	}
	deliver("Lists", "Bob <bob@x.test>", "Quiet list")
	waitUntil(t, 5*time.Second, "Quiet list synced", func() bool {
		return slices.Contains(h.subjects(h.mailboxByPath("Lists").ID), "Quiet list")
	})
	deliver("INBOX", "Bob <bob@x.test>", "Inbox again")
	next("Inbox again")
}

// mailboxByPath is the account's mailbox with this path.
func (h *harness) mailboxByPath(path string) api.Mailbox {
	h.t.Helper()
	list, err := h.c.Mailbox().List(h.t.Context(), &api.MailboxListParams{AccountID: &h.acct})
	if err != nil {
		h.t.Fatal(err)
	}
	for _, mb := range list {
		if mb.Path == path {
			return mb
		}
	}
	h.t.Fatalf("no mailbox %s in %+v", path, list)
	return api.Mailbox{}
}

// TestNotifySmartScope: under a smart mailbox's scope, new mail in any
// folder that the smart mailbox lists notifies, and nothing else does.
func TestNotifySmartScope(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	if err := mem.User.Create("Lists", nil); err != nil {
		t.Fatal(err)
	}
	announced := make(chan []notify.Mail, 8)
	cfg := testConfig
	cfg.Announce = func(_ context.Context, _ int64, mail []notify.Mail) error {
		announced <- mail
		return nil
	}
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, cfg)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	deliver := func(folder, subject string) {
		t.Helper()
		raw := "From: Bob <bob@x.test>\r\nSubject: " + subject + "\r\n\r\nHello.\r\n"
		if _, err := mem.User.Append(folder, strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
			t.Fatal(err)
		}
	}
	urgent, err := h.c.Smart().Create(ctx, &api.SmartCreateParams{Name: "Urgent", Conditions: api.Conditions{
		Match: api.ConditionMatchAll, Conditions: []api.Condition{
			{Field: api.ConditionFieldSubject, Op: api.ConditionOpContains, Value: "urgent"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Settings().Set(ctx, &api.SettingsSetParams{NotifyScope: ptr(api.NotifyScopeSmart), NotifySmartID: &urgent.ID}); err != nil {
		t.Fatal(err)
	}
	deliver("INBOX", "Lunch")
	deliver("Lists", "Urgent: the build is red")
	select {
	case mail := <-announced:
		if len(mail) != 1 || mail[0].Subject != "Urgent: the build is red" {
			t.Errorf("announced %+v, want only the urgent one (Lunch first would be wrong)", mail)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the urgent mail was not announced")
	}
}
