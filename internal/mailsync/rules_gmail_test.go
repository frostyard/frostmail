package mailsync

import (
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// TestGmailRulesMoveToALabel: on Gmail a rule's move takes new mail out of
// the inbox into a label, as label edits; mail of the first sync and mail
// that matched no rule stay; nothing runs twice.
func TestGmailRulesMoveToALabel(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	ctx := t.Context()
	g.add(gAll, "Old invoice", 0, `\Inbox`)
	e.pass()
	work := e.mbs["Work"]
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.CreateRule(ctx, store.Rule{Name: "Invoices", Enabled: true,
			Conditions: api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{
				{Field: api.ConditionFieldSubject, Op: api.ConditionOpContains, Value: "invoice"}}},
			Actions: []api.RuleAction{{Kind: api.RuleActionKindMove, MailboxID: &work}, {Kind: api.RuleActionKindRead}}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.a.runRules(ctx)
	eq(t, "INBOX after the first sync's rules", e.subjects("INBOX"), []string{"Old invoice"})

	g.add(gAll, "Invoice 7", 0, `\Inbox`)
	g.add(gAll, "Hello", 0, `\Inbox`)
	e.pass()
	e.a.runRules(ctx)
	eq(t, "INBOX", e.subjects("INBOX"), []string{"Hello", "Old invoice"})
	eq(t, "Work", e.subjects("Work"), []string{"Invoice 7"})
	if waiting, _ := e.db.RulesWaiting(ctx, e.a.acct.ID); len(waiting) != 0 {
		t.Errorf("still waiting: %v", waiting)
	}
	e.replay()
	eq(t, "Invoice 7's labels", sorted(g.find("Invoice 7").labels), []string{"Work"})
	if !slices.Contains(g.find("Invoice 7").flags, `\Seen`) {
		t.Errorf("Invoice 7's flags = %q, want \\Seen", g.find("Invoice 7").flags)
	}
	e.pass()
	e.a.runRules(ctx)
	eq(t, "Work after another pass", e.subjects("Work"), []string{"Invoice 7"})
}
