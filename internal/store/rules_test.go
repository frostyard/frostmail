package store

import (
	"errors"
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
	name, off := "A2", false
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.UpdateRule(ctx, a.ID, RuleUpdate{Name: &name, Enabled: &off})
		return err
	}); err != nil {
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

// TestUpdateRuleKeepsFieldsNotGiven: a partial enabled update leaves the
// name, conditions and actions unchanged, and a partial name update leaves
// the enabled flag and the rest unchanged.
func TestUpdateRuleKeepsFieldsNotGiven(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	color := int64(3)
	actions := []api.RuleAction{{Kind: api.RuleActionKindFlag, Color: &color}, {Kind: api.RuleActionKindStop}}
	sameActions := func(as []api.RuleAction) bool {
		if len(as) != len(actions) {
			return false
		}
		for i := range as {
			if as[i].Kind != actions[i].Kind || (as[i].Color == nil) != (actions[i].Color == nil) {
				return false
			}
			if as[i].Color != nil && *as[i].Color != *actions[i].Color {
				return false
			}
		}
		return true
	}
	unread := one("unread", "is", "true")
	var created Rule
	if err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		created, err = tx.CreateRule(ctx, Rule{Name: "Flag unread", Enabled: true, Conditions: unread, Actions: actions})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	update := func(id int64, u RuleUpdate) (Rule, error) {
		t.Helper()
		var out Rule
		err := d.Tx(ctx, func(tx *Tx) error {
			var err error
			out, err = tx.UpdateRule(ctx, id, u)
			return err
		})
		return out, err
	}

	off := false
	got, err := update(created.ID, RuleUpdate{Enabled: &off})
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.Name != "Flag unread" || got.Conditions.Match != unread.Match ||
		!slices.Equal(got.Conditions.Conditions, unread.Conditions) || !sameActions(got.Actions) {
		t.Fatalf("enabled-only update = %+v", got)
	}

	name := "Flag anything unread"
	got, err = update(created.ID, RuleUpdate{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.Name != name || got.Conditions.Match != unread.Match ||
		!slices.Equal(got.Conditions.Conditions, unread.Conditions) || !sameActions(got.Actions) {
		t.Fatalf("name-only update = %+v", got)
	}

	if _, err := update(9999, RuleUpdate{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update of a missing rule: %v", err)
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
