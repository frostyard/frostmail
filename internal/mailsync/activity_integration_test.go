//go:build integration

package mailsync_test

// Plan 0015's Phase 2 check against the frostmail-mailtest container: the
// activity maild records for real work. test5 sends to itself, as other
// tests send to test5, so no other test's counts move.

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// startTest5 starts maild with the test5 account, with smtpUser as its
// submission login.
func startTest5(t *testing.T, smtpUser string) *harness {
	t.Helper()
	host := itHost(t)
	cfg := itConfig()
	var log *slog.Logger
	if os.Getenv("FROSTMAIL_TEST_LOG") != "" {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, Log: log})
	h := &harness{t: t, srv: srv, c: srv.Dial(t)}
	ctx := t.Context()
	if _, err := h.c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	imapCfg := api.ServerConfig{Host: host, Port: 993, TLS: api.TLSModeTLS, Username: "test5@mailtest.test"}
	smtpCfg := api.ServerConfig{Host: host, Port: 587, TLS: api.TLSModeStartTLS, Username: smtpUser}
	a, err := h.c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "test5@mailtest.test", DisplayName: "Test Five",
		Auth: api.AuthKindPassword, IMAP: &imapCfg, SMTP: &smtpCfg,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.acct = a.ID
	if err := h.c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: itPassword}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	return h
}

// sendToSelf sends test5 a message with subject and returns its outbox id.
func (h *harness) sendToSelf(subject string) int64 {
	h.t.Helper()
	ctx := h.t.Context()
	d, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew, AccountID: &h.acct})
	if err != nil {
		h.t.Fatal(err)
	}
	c := d.Content
	c.To, c.Subject, c.HTML = []api.Address{{Name: "Test Five", Address: "test5@mailtest.test"}}, subject, "<p>Activity check.</p>"
	if _, err := h.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: c}); err != nil {
		h.t.Fatal(err)
	}
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		h.t.Fatal(err)
	}
	return item.ID
}

func TestActivityAgainstDovecot(t *testing.T) {
	h := startTest5(t, "test5@mailtest.test")
	ctx := t.Context()
	subject := "Activity check " + time.Now().Format("150405.000")

	// A mailbox pass: the first sync reconciled INBOX.
	inbox := h.inbox()
	h.waitActivity(api.ActivityKindMailboxsynced, func(e api.ActivityEntry) bool { return *e.MailboxID == inbox.ID })

	// A rule for the message test5 sends itself.
	cond := api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{
		{Field: api.ConditionFieldSubject, Op: api.ConditionOpContains, Value: subject}}}
	if _, err := h.c.Rule().Create(ctx, &api.RuleCreateParams{Name: "Activity", Conditions: cond,
		Actions: []api.RuleAction{{Kind: api.RuleActionKindRead}}}); err != nil {
		t.Fatal(err)
	}

	// A send, the IDLE wake its delivery causes, and the rule run.
	id := h.sendToSelf(subject)
	h.waitFor(30*time.Second, "outbox sent", func(_ api.EventEnvelope, ev api.Event) bool {
		o, ok := ev.(api.OutboxChanged)
		return ok && o.ID == id && o.State == api.OutboxStateSent && !o.Deleted
	})
	h.waitActivity(api.ActivityKindSent, nil)
	h.waitActivity(api.ActivityKindIdle, func(e api.ActivityEntry) bool { return *e.MailboxID == inbox.ID })
	h.waitActivity(api.ActivityKindRulesapplied, nil)

	// A flag change pushed to the server.
	v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 1})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %v, %v", rows, err)
	}
	if err := h.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{rows[0].ID}, Changes: api.FlagChanges{Flagged: ptr(true)}}); err != nil {
		t.Fatal(err)
	}
	h.waitActivity(api.ActivityKindPushed, nil)

	for _, e := range h.srv.Activity.Recent(activity.MaxRecent) {
		if e.AccountID == nil || *e.AccountID != h.acct {
			t.Errorf("entry %d (%s) names account %v, want %d", e.Seq, e.Kind, e.AccountID, h.acct)
		}
	}
	logLines(t, h)
}

// A send the submission server refuses: a login it does not know.
func TestActivityOfARefusedSendAgainstDovecot(t *testing.T) {
	h := startTest5(t, "nobody@mailtest.test")
	h.sendToSelf("Activity refused " + time.Now().Format("150405.000"))
	e := h.waitActivity(api.ActivityKindSendfailed, nil)
	if e.Level != api.ActivityLevelError || *e.ErrorClass != api.ErrorClassAuth {
		t.Errorf("refused send = %+v, want an error of class auth", e)
	}
	logLines(t, h)
}

// logLines writes the entries as mailctl activity prints them, for the
// plan's record.
func logLines(t *testing.T, h *harness) {
	t.Helper()
	n := names{h.acct: "test5@mailtest.test"}
	for _, e := range h.srv.Activity.Recent(activity.MaxRecent) {
		source, text := activity.Line(e, n)
		t.Logf("%s  %-5s  %s  %s", e.At.Format("15:04:05.000"), e.Level, source, text)
	}
}

// names names test5's account; mailboxes go by their IDs.
type names map[int64]string

func (n names) Account(id int64) string { return n[id] }
func (n names) Mailbox(int64) string    { return "" }
