package store

import (
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
)

// TestRulesKeepOrder: rules keep their order, conditions and actions.
func TestRulesKeepOrder(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	color := int64(3)
	mk := func(name string) Rule {
		t.Helper()
		var out Rule
		if err := d.Tx(ctx, func(tx *Tx) error {
			var err error
			out, err = tx.CreateRule(ctx, Rule{Name: name, Enabled: true, Conditions: one("unread", "is", "true"),
				Actions: []api.RuleAction{{Kind: api.RuleActionKindFlag, Color: &color}, {Kind: api.RuleActionKindStop}}})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	a, b := mk("A"), mk("B")
	if a.Position != 0 || b.Position != 1 || len(b.Actions) != 2 || *b.Actions[0].Color != 3 || b.Actions[1].Kind != api.RuleActionKindStop {
		t.Fatalf("created = %+v, %+v", a, b)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.MoveRule(ctx, b.ID, 0) }); err != nil {
		t.Fatal(err)
	}
	list, err := d.Rules(ctx)
	if err != nil || len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID || list[0].Position != 0 {
		t.Errorf("after the move = %+v, %v", list, err)
	}
	a.Enabled, a.Name = false, "A2"
	if err := d.Tx(ctx, func(tx *Tx) error { _, err := tx.UpdateRule(ctx, a); return err }); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.Rule(ctx, a.ID); got.Enabled || got.Name != "A2" {
		t.Errorf("updated = %+v", got)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.DeleteRule(ctx, b.ID) }); err != nil {
		t.Fatal(err)
	}
	if list, _ := d.Rules(ctx); len(list) != 1 || list[0].Position != 0 {
		t.Errorf("after the delete = %+v", list)
	}
}

// TestRulesWaitingMarksInboxMail: only messages in an inbox wait for the
// rules, and clearing takes them out.
func TestRulesWaitingMarksInboxMail(t *testing.T) {
	f := newConditionsFixture(t)
	ctx := t.Context()
	all := []int64{f.lunch, f.invoice, f.old, f.report}
	if err := f.d.Tx(ctx, func(tx *Tx) error { return tx.MarkRulesWaiting(ctx, all) }); err != nil {
		t.Fatal(err)
	}
	got, err := f.d.RulesWaiting(ctx, f.acct)
	if err != nil || !slices.Equal(got, []int64{f.lunch}) {
		t.Fatalf("waiting = %v, %v; want only the inbox's lunch", got, err)
	}
	if err := f.d.Tx(ctx, func(tx *Tx) error { return tx.ClearRulesWaiting(ctx, got) }); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.d.RulesWaiting(ctx, f.acct); len(got) != 0 {
		t.Errorf("after clearing = %v", got)
	}
}
