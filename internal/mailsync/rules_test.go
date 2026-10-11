package mailsync_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/notify"
	"github.com/frostyard/frostmail/internal/store"
)

// rulesHarness is a synced account whose new mail is announced on a
// channel, with a way to deliver mail to its server's INBOX.
type rulesHarness struct {
	*harness
	mem       *imapxtest.Mem // nil against Dovecot
	appendRaw func(raw []byte) error
	announced chan []notify.Mail
}

// announcing is the test configuration with new mail announced on a
// channel.
func announcing() (mailsync.Config, chan []notify.Mail) {
	announced := make(chan []notify.Mail, 8)
	cfg := testConfig
	cfg.Announce = func(_ context.Context, _ int64, mail []notify.Mail) error {
		announced <- mail
		return nil
	}
	return cfg, announced
}

// newRulesHarness is a memory server account with a Receipts folder.
func newRulesHarness(t *testing.T) *rulesHarness {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	seed(t, mem)
	if err := mem.User.Create("Receipts", nil); err != nil {
		t.Fatal(err)
	}
	cfg, announced := announcing()
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, cfg)
	h.waitPhase(api.SyncPhaseIdle)
	return &rulesHarness{harness: h, mem: mem, announced: announced, appendRaw: func(raw []byte) error {
		_, err := mem.User.Append("INBOX", bytes.NewReader(raw), &imap.AppendOptions{})
		return err
	}}
}

func (r *rulesHarness) deliver(from, subject string) {
	r.t.Helper()
	raw := "From: " + from + "\r\nSubject: " + subject + "\r\nMessage-ID: <" + strings.ReplaceAll(subject, " ", ".") +
		"@x.test>\r\n\r\nHello.\r\n"
	if err := r.appendRaw([]byte(raw)); err != nil {
		r.t.Fatal(err)
	}
	if err := r.c.Sync().Now(r.t.Context(), &api.SyncNowParams{AccountID: r.acct}); err != nil {
		r.t.Fatal(err)
	}
}

// settledWith waits for an announcement with subject, and returns every
// message announced up to it; the rules ran on that pass's mail before it.
func (r *rulesHarness) settledWith(subject string) []notify.Mail {
	r.t.Helper()
	timeout := time.After(5 * time.Second)
	var all []notify.Mail
	for {
		select {
		case mail := <-r.announced:
			all = append(all, mail...)
			if slices.ContainsFunc(mail, func(m notify.Mail) bool { return m.Subject == subject }) {
				return all
			}
		case <-timeout:
			r.t.Fatalf("%s was not announced", subject)
		}
	}
}

func (r *rulesHarness) rule(name string, conds api.Conditions, actions ...api.RuleAction) api.Rule {
	r.t.Helper()
	got, err := r.c.Rule().Create(r.t.Context(), &api.RuleCreateParams{Name: name, Conditions: conds, Actions: actions})
	if err != nil {
		r.t.Fatal(err)
	}
	return *got
}

// summary is the one message with a subject.
func (r *rulesHarness) summary(subject string) api.MessageSummary {
	r.t.Helper()
	s, ok := r.find(subject)
	if !ok {
		r.t.Fatalf("no one message with subject %q", subject)
	}
	return s
}

// find looks for the one message with a subject, by a search.
func (r *rulesHarness) find(subject string) (api.MessageSummary, bool) {
	r.t.Helper()
	ctx := r.t.Context()
	v, err := r.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Conditions: &api.Conditions{
		Match: api.ConditionMatchAll, Conditions: []api.Condition{{Field: api.ConditionFieldSubject, Op: api.ConditionOpIs, Value: subject}}}}})
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = r.c.View().Close(ctx, &api.ViewCloseParams{ID: v.ID}) }()
	rows, err := r.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 2})
	if err != nil {
		r.t.Fatal(err)
	}
	if len(rows) != 1 {
		return api.MessageSummary{}, false
	}
	return rows[0], true
}

// flagged reports whether the message with a subject is there and flagged.
func (r *rulesHarness) flagged(subject string) bool {
	s, ok := r.find(subject)
	return ok && s.Flags.Flagged
}

// synced reports whether a message with a subject is there.
func (r *rulesHarness) synced(subject string) bool {
	_, ok := r.find(subject)
	return ok
}

func cond(field api.ConditionField, op api.ConditionOp, v string) api.Conditions {
	return api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{{Field: field, Op: op, Value: v}}}
}

func color(n int64) *int64 { return &n }

// TestRulesActOnNewInboxMail: rules run in order on new inbox mail before
// it is announced; Stop ends the run for what it matched; Send
// Notification announces mail the scope would not; the server gets the
// moves.
func TestRulesActOnNewInboxMail(t *testing.T) {
	r := newRulesHarness(t)
	receipts := r.mailboxByPath("Receipts")
	r.rule("Receipts", cond(api.ConditionFieldSubject, api.ConditionOpContains, "receipt"),
		api.RuleAction{Kind: api.RuleActionKindMove, MailboxID: &receipts.ID},
		api.RuleAction{Kind: api.RuleActionKindRead},
		api.RuleAction{Kind: api.RuleActionKindStop})
	r.rule("Ann", cond(api.ConditionFieldFrom, api.ConditionOpContains, "ann"),
		api.RuleAction{Kind: api.RuleActionKindFlag, Color: color(2)})
	r.rule("Bob's receipts", cond(api.ConditionFieldFrom, api.ConditionOpContains, "bob"),
		api.RuleAction{Kind: api.RuleActionKindNotify})
	r.rule("Everything", api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{}},
		api.RuleAction{Kind: api.RuleActionKindFlag, Color: color(5)})

	r.deliver("Ann <ann@x.test>", "Your receipt")
	r.deliver("Ann <ann@x.test>", "Lunch")
	select {
	case mail := <-r.announced:
		if len(mail) != 1 || mail[0].Subject != "Lunch" {
			t.Errorf("first announcement %+v, want only Lunch: the receipt left the inbox", mail)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was announced")
	}
	receipt := r.summary("Your receipt")
	if !slices.Equal(receipt.MailboxIDs, []int64{receipts.ID}) || !receipt.Flags.Seen || receipt.Flags.Flagged {
		t.Errorf("the receipt = %+v; want in Receipts, read, not flagged (Stop)", receipt)
	}
	if lunch := r.summary("Lunch"); !lunch.Flags.Flagged || lunch.Flags.FlagColor != 5 {
		t.Errorf("Lunch = %+v; want flagged 5, the last rule's color", lunch.Flags)
	}
	waitUntil(t, 5*time.Second, "the receipt on the server in Receipts", func() bool {
		st, err := r.mem.User.Status("Receipts", &imap.StatusOptions{NumMessages: true})
		return err == nil && st.NumMessages != nil && *st.NumMessages == 1
	})
	// The read mark went to the server before the move, so it moved with
	// the message.
	other, err := imapx.Open(t.Context(), r.mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Select(t.Context(), "Receipts"); err != nil {
		t.Fatal(err)
	}
	if ups, err := other.FetchFlags(t.Context(), []uint32{1}, 0); err != nil || len(ups) != 1 || !ups[0].Flags.Seen {
		t.Errorf("the receipt's flags on the server = %+v, %v; want seen", ups, err)
	}

	// Send Notification announces mail a rule moved out of the inbox.
	r.rule("Bob to Receipts", cond(api.ConditionFieldFrom, api.ConditionOpContains, "bob"),
		api.RuleAction{Kind: api.RuleActionKindMove, MailboxID: &receipts.ID})
	if err := r.c.Rule().Move(r.t.Context(), &api.RuleMoveParams{ID: r.ruleID("Bob to Receipts"), Position: 0}); err != nil {
		t.Fatal(err)
	}
	r.deliver("Bob <bob@x.test>", "A paper")
	select {
	case mail := <-r.announced:
		if len(mail) != 1 || mail[0].Subject != "A paper" {
			t.Errorf("announced %+v, want A paper (Send Notification)", mail)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send Notification announced nothing")
	}
}

func (r *rulesHarness) ruleID(name string) int64 {
	r.t.Helper()
	list, err := r.c.Rule().List(r.t.Context(), &api.RuleListParams{})
	if err != nil {
		r.t.Fatal(err)
	}
	for _, rule := range list {
		if rule.Name == name {
			return rule.ID
		}
	}
	r.t.Fatalf("no rule %s in %+v", name, list)
	return 0
}

// TestRulesRunOnce: a rule acts on a message once, not again at later
// passes; a waiting mark left when maild stopped is honored at its next
// start; a read-only account's mail runs no rules.
func TestRulesRunOnce(t *testing.T) {
	r := newRulesHarness(t)
	ctx := t.Context()
	r.rule("Flag", cond(api.ConditionFieldSubject, api.ConditionOpContains, "flagme"),
		api.RuleAction{Kind: api.RuleActionKindFlag, Color: color(1)})
	r.deliver("Ann <ann@x.test>", "flagme one")
	waitUntil(t, 5*time.Second, "flagme one flagged", func() bool { return r.flagged("flagme one") })
	one := r.summary("flagme one")
	if err := r.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{one.ID}, Changes: api.FlagChanges{Flagged: ptr(false)}}); err != nil {
		t.Fatal(err)
	}
	r.deliver("Ann <ann@x.test>", "plain")
	r.settledWith("plain")
	if r.flagged("flagme one") {
		t.Error("the rule ran again on flagme one")
	}

	// As if maild stopped between storing it and running the rules.
	if err := r.srv.DB.Tx(ctx, func(tx *store.Tx) error { return tx.MarkRulesWaiting(ctx, []int64{one.ID}) }); err != nil {
		t.Fatal(err)
	}
	if err := r.srv.Sync.Restart(ctx, r.acct); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "the waiting message flagged after the restart", func() bool { return r.flagged("flagme one") })

	if _, err := r.c.Account().Update(ctx, &api.AccountUpdateParams{ID: r.acct, ReadOnly: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	r.waitPhase(api.SyncPhaseIdle)
	r.deliver("Ann <ann@x.test>", "flagme two")
	waitUntil(t, 5*time.Second, "flagme two synced", func() bool { return r.synced("flagme two") })
	// The mark is made with the row, so none now means none ever.
	if waiting, err := r.srv.DB.RulesWaiting(ctx, r.acct); err != nil || len(waiting) != 0 {
		t.Errorf("a read-only account's mail waits for the rules: %v, %v", waiting, err)
	}
}

// TestApplyRules: rule.apply runs the rules on stored mail in any mailbox
// and says how many matched; a read-only account's mail is a conflict.
func TestApplyRules(t *testing.T) {
	r := newRulesHarness(t)
	ctx := t.Context()
	receipts := r.mailboxByPath("Receipts")
	r.deliver("Ann <ann@x.test>", "Old receipt")
	r.settledWith("Old receipt")
	r.deliver("Ann <ann@x.test>", "Hello")
	r.settledWith("Hello")
	r.rule("Receipts", cond(api.ConditionFieldSubject, api.ConditionOpContains, "receipt"),
		api.RuleAction{Kind: api.RuleActionKindMove, MailboxID: &receipts.ID})
	old, hello := r.summary("Old receipt"), r.summary("Hello")
	got, err := r.c.Rule().Apply(ctx, &api.RuleApplyParams{IDs: []int64{old.ID, hello.ID}})
	if err != nil || got.Matched != 1 {
		t.Fatalf("rule.apply = %+v, %v; want 1 matched", got, err)
	}
	if moved := r.summary("Old receipt"); !slices.Equal(moved.MailboxIDs, []int64{receipts.ID}) {
		t.Errorf("Old receipt in %v, want Receipts", moved.MailboxIDs)
	}
	if _, err := r.c.Rule().Apply(ctx, &api.RuleApplyParams{IDs: []int64{9999}}); !isCode(err, api.CodeNotFound) {
		t.Errorf("an unknown message: %v", err)
	}
	if _, err := r.c.Account().Update(ctx, &api.AccountUpdateParams{ID: r.acct, ReadOnly: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.c.Rule().Apply(ctx, &api.RuleApplyParams{IDs: []int64{hello.ID}}); !isCode(err, api.CodeConflict) {
		t.Errorf("a read-only account's mail: %v, want conflict", err)
	}
}
