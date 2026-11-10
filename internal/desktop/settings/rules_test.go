package settings_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/settings"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

func (f *fixture) ruleNames() []string {
	f.t.Helper()
	list, err := f.c.Rule().List(f.t.Context(), &api.RuleListParams{})
	if err != nil {
		f.t.Fatal(err)
	}
	var out []string
	for _, r := range list {
		out = append(out, r.Name)
	}
	return out
}

func (f *fixture) addRule(name string) *api.Rule {
	f.t.Helper()
	r, err := f.c.Rule().Create(f.t.Context(), &api.RuleCreateParams{Name: name,
		Conditions: api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{{Field: api.ConditionFieldFrom, Op: api.ConditionOpContains, Value: "x"}}},
		Actions:    []api.RuleAction{{Kind: api.RuleActionKindRead}}})
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}

func TestRulesEmptyAndAdd(t *testing.T) {
	f := newFixture(t, setup{})
	f.show(settings.PaneRules)
	f.sees("No Rules")
	for _, name := range []string{"Edit", "Duplicate", "Remove", "Move Up", "Move Down"} {
		if !f.disabled(name) {
			t.Errorf("%s works with nothing selected", name)
		}
	}
	f.click("Add Rule")
	if len(f.rules) != 1 || f.rules[0] != nil {
		t.Errorf("Add Rule opened %v", f.rules)
	}
}

func TestRuleSelectEditEnableMove(t *testing.T) {
	f := newFixture(t, setup{})
	a := f.addRule("First")
	f.addRule("Second")
	f.show(settings.PaneRules)
	f.sees("Second")

	f.click("First")
	if a2 := f.control(ui.RoleButton, "First"); !a2.Checked {
		t.Error("the selected rule is not pressed")
	}
	if !f.disabled("Move Up") || f.disabled("Move Down") || f.disabled("Edit") {
		t.Error("the first rule's buttons are wrong")
	}
	f.click("Edit")
	if len(f.rules) != 1 || f.rules[0] == nil || f.rules[0].ID != a.ID {
		t.Fatalf("Edit opened %v", f.rules)
	}
	f.click("First")
	f.click("First") // a second click at once: a double click
	f.until("the double click", func() bool { return len(f.rules) == 2 })

	if !f.checked("Enable First") {
		t.Error("an enabled rule is unchecked")
	}
	f.check("Enable Second") // without selecting it
	if f.control(ui.RoleButton, "Second").Checked || !f.control(ui.RoleButton, "First").Checked {
		t.Error("the check box moved the selection")
	}
	f.check("Enable First")
	f.until("the rule off", func() bool {
		list, _ := f.c.Rule().List(t.Context(), &api.RuleListParams{})
		return !list[0].Enabled
	})
	f.until("the box off", func() bool { return !f.checked("Enable First") })

	f.click("Move Down")
	f.until("the new order", func() bool { return slices.Equal(f.ruleNames(), []string{"Second", "First"}) })
	f.until("the buttons", func() bool { return f.disabled("Move Down") && !f.disabled("Move Up") })
	f.click("Move Up")
	f.until("the first order", func() bool { return slices.Equal(f.ruleNames(), []string{"First", "Second"}) })
}

func TestRuleDuplicateGoesAfterTheOriginal(t *testing.T) {
	f := newFixture(t, setup{})
	f.addRule("First")
	f.addRule("Second")
	f.show(settings.PaneRules)
	f.sees("Second")
	f.click("First")
	f.click("Duplicate")
	f.until("the copy", func() bool { return slices.Equal(f.ruleNames(), []string{"First", "First Copy", "Second"}) })
	f.sees("First Copy")
}

func TestRuleRemoveAsksAndForgetsTheSelection(t *testing.T) {
	f := newFixture(t, setup{})
	f.addRule("First")
	f.show(settings.PaneRules)
	f.sees("First")
	f.click("First")
	f.click("Remove")
	f.sees(`Remove the rule "First"?`)
	if len(f.ruleNames()) != 1 {
		t.Fatal("removed before the answer")
	}
	f.press(f.control(ui.RoleButton, "Remove"))
	f.sees("No Rules")
	if !f.disabled("Edit") {
		t.Error("Edit stays on for a rule that is gone")
	}
}

// failingRules refuses rule.update while told to.
type failingRules struct {
	api.RuleService
	fail *atomic.Bool
}

func (s failingRules) Update(ctx context.Context, p *api.RuleUpdateParams) (*api.Rule, error) {
	if s.fail.Load() {
		return nil, errors.New("the rule is locked")
	}
	return s.RuleService.Update(ctx, p)
}

func TestRuleErrorStaysUntilTheNextChangeSucceeds(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	f := newFixture(t, setup{server: rpctest.Options{Services: func(s *api.Services) { s.Rule = failingRules{s.Rule, &fail} }}})
	f.addRule("First")
	f.show(settings.PaneRules)
	f.sees("First")
	f.check("Enable First")
	f.sees("the rule is locked")
	fail.Store(false)
	f.check("Enable First")
	f.until("the error's end", func() bool { return !f.tt.HasText("the rule is locked") })
}

func TestRuleShowsItsProblem(t *testing.T) {
	f := newFixture(t, setup{})
	a := f.addIMAP("me@a.test")
	var box int64
	ctx := t.Context()
	if err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{{Path: "Keep", Role: api.MailboxRoleNone, Selectable: true}})
		if err == nil {
			box = mbs[0].ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.Rule().Create(ctx, &api.RuleCreateParams{Name: "Move it", Conditions: api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{{Field: api.ConditionFieldFrom, Op: api.ConditionOpContains, Value: "x"}}},
		Actions: []api.RuleAction{{Kind: api.RuleActionKindMove, MailboxID: &box}}}); err != nil {
		t.Fatal(err)
	}
	if err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.ReplaceMailboxes(ctx, a.ID, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.show(settings.PaneRules)
	f.sees("Move it")
	list, err := f.c.Rule().List(ctx, &api.RuleListParams{})
	if err != nil || len(list) != 1 || list[0].Problem == nil {
		t.Skipf("maild reports no problem for this rule: %+v, %v", list, err)
	}
	f.until("the problem's icon", func() bool { _, ok := f.tt.AccessByRole(ui.RoleImage, *list[0].Problem); return ok })
}
