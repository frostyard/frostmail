//go:build integration

package mailsync_test

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/notify"
)

// TestDovecotRules: against Dovecot (test2, after
// TestDovecotSyncAndLiveChanges), a rule marks, flags and files new inbox
// mail before it would notify, the server follows, and Apply Rules does the
// same to stored mail.
func TestDovecotRules(t *testing.T) {
	opts := dovecotTest2(t)
	ctx := t.Context()
	other, err := imapx.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	cfg, announced := announcing()
	h := newHarnessWith(t, opts, opts.Password, cfg)
	h.waitPhase(api.SyncPhaseIdle)
	r := &rulesHarness{harness: h, announced: announced, appendRaw: func(raw []byte) error {
		_, err := other.Append(ctx, "INBOX", raw, nil)
		return err
	}}
	archive := r.mailboxByPath("Archive")
	archived := func() uint32 {
		st, err := other.Status(ctx, "Archive")
		if err != nil {
			t.Fatal(err)
		}
		return st.Messages
	}
	before := archived()

	r.rule("Receipts", cond(api.ConditionFieldSubject, api.ConditionOpContains, "rules-it receipt"),
		api.RuleAction{Kind: api.RuleActionKindRead},
		api.RuleAction{Kind: api.RuleActionKindFlag, Color: color(3)},
		api.RuleAction{Kind: api.RuleActionKindMove, MailboxID: &archive.ID},
		api.RuleAction{Kind: api.RuleActionKindStop})
	r.deliver("Shop <shop@mailtest.test>", "rules-it receipt")
	r.deliver("Ann <ann@mailtest.test>", "rules-it hello")
	if mail := r.settledWith("rules-it hello"); slices.ContainsFunc(mail, func(m notify.Mail) bool { return m.Subject == "rules-it receipt" }) {
		t.Errorf("the filed receipt was announced: %+v", mail)
	}
	receipt := r.summary("rules-it receipt")
	if !slices.Equal(receipt.MailboxIDs, []int64{archive.ID}) || !receipt.Flags.Seen || receipt.Flags.FlagColor != 3 {
		t.Errorf("the receipt = %+v; want in Archive, read, flagged 3", receipt)
	}
	waitUntil(t, 5*time.Second, "the receipt in Archive on the server", func() bool { return archived() == before+1 })
	if _, err := other.Select(ctx, "Archive"); err != nil {
		t.Fatal(err)
	}
	uids, err := other.UIDs(ctx)
	if err != nil || len(uids) == 0 {
		t.Fatalf("Archive UIDs = %v, %v", uids, err)
	}
	ups, err := other.FetchFlags(ctx, uids[len(uids)-1:], 0)
	if err != nil || len(ups) != 1 || !ups[0].Flags.Seen || !ups[0].Flags.Flagged || ups[0].Flags.Color != 3 {
		t.Errorf("the receipt's flags on the server = %+v, %v; want seen, flagged 3", ups, err)
	}
	// STATUS is for mailboxes other than the selected one.
	if _, err := other.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}

	// Apply Rules on stored mail.
	r.deliver("Ann <ann@mailtest.test>", "rules-it stored")
	r.settledWith("rules-it stored")
	stored := r.summary("rules-it stored")
	r.rule("Stored", cond(api.ConditionFieldSubject, api.ConditionOpContains, "rules-it stored"),
		api.RuleAction{Kind: api.RuleActionKindMove, MailboxID: &archive.ID})
	got, err := r.c.Rule().Apply(ctx, &api.RuleApplyParams{IDs: []int64{stored.ID, r.summary("rules-it hello").ID}})
	if err != nil || got.Matched != 1 {
		t.Fatalf("rule.apply = %+v, %v; want 1 matched", got, err)
	}
	waitUntil(t, 5*time.Second, "the stored message in Archive on the server", func() bool { return archived() == before+2 })
}
