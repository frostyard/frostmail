package organize_test

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/organize"
)

func TestEveryFieldHasOperatorsWithLabels(t *testing.T) {
	if len(organize.Fields) != 22 {
		t.Errorf("%d fields, the spec lists 22", len(organize.Fields))
	}
	for _, f := range organize.Fields {
		ops := organize.OpsFor(f.Field)
		if len(ops) == 0 {
			t.Errorf("%s has no operators", f.Field)
		}
		for _, op := range ops {
			if organize.OpLabel[op] == "" {
				t.Errorf("%s has no label", op)
			}
		}
	}
}

func TestChoosingAFieldStartsIt(t *testing.T) {
	ctx := organize.Context{Today: "2026-11-10", Mailboxes: []api.Mailbox{{ID: 7}}, Accounts: []api.Account{{ID: 3}}}
	for f, want := range map[api.ConditionField]api.Condition{
		api.ConditionFieldUnread:   {Field: api.ConditionFieldUnread, Op: api.ConditionOpIs, Value: "true"},
		api.ConditionFieldSubject:  {Field: api.ConditionFieldSubject, Op: api.ConditionOpContains},
		api.ConditionFieldMailbox:  {Field: api.ConditionFieldMailbox, Op: api.ConditionOpIs, Value: "7"},
		api.ConditionFieldAccount:  {Field: api.ConditionFieldAccount, Op: api.ConditionOpIs, Value: "3"},
		api.ConditionFieldRole:     {Field: api.ConditionFieldRole, Op: api.ConditionOpIs, Value: "inbox"},
		api.ConditionFieldColor:    {Field: api.ConditionFieldColor, Op: api.ConditionOpIs, Value: "1"},
		api.ConditionFieldReceived: {Field: api.ConditionFieldReceived, Op: api.ConditionOpToday},
	} {
		if got := organize.WithField(f, ctx); got != want {
			t.Errorf("%s = %+v, want %+v", f, got, want)
		}
	}
}

func TestChoosingAnOperatorKeepsAValueOfTheSameKind(t *testing.T) {
	ctx := organize.Context{Today: "2026-11-10"}
	text := api.Condition{Field: api.ConditionFieldFrom, Op: api.ConditionOpContains, Value: "ann"}
	if got := organize.WithOp(text, api.ConditionOpIs, ctx); got.Value != "ann" {
		t.Errorf("text to text: %+v", got)
	}
	list := api.Condition{Field: api.ConditionFieldColor, Op: api.ConditionOpAnyof, Value: "3,5"}
	if got := organize.WithOp(list, api.ConditionOpIs, ctx); got.Value != "3" {
		t.Errorf("a list to one choice keeps its first: %+v", got)
	}
	day := api.Condition{Field: api.ConditionFieldSent, Op: api.ConditionOpWithin, Value: "3w"}
	if got := organize.WithOp(day, api.ConditionOpOn, ctx); got.Value != "2026-11-10" {
		t.Errorf("a duration to a day starts anew: %+v", got)
	}
	if got := organize.WithOp(day, api.ConditionOpNotwithin, ctx); got.Value != "3w" {
		t.Errorf("a duration to a duration: %+v", got)
	}
}

func TestWithKindKeepsWhatTheKindTakes(t *testing.T) {
	id := int64(4)
	move := api.RuleAction{Kind: api.RuleActionKindMove, MailboxID: &id}
	if got := organize.WithKind(move, api.RuleActionKindCopy); got.MailboxID == nil || *got.MailboxID != 4 {
		t.Errorf("move to copy: %+v", got)
	}
	if got := organize.WithKind(move, api.RuleActionKindRead); got.MailboxID != nil {
		t.Errorf("read kept a mailbox: %+v", got)
	}
	if got := organize.WithKind(move, api.RuleActionKindFlag); got.Color == nil || *got.Color != 1 {
		t.Errorf("flag starts red: %+v", got)
	}
}

func TestActionsComplete(t *testing.T) {
	boxes := []api.Mailbox{{ID: 4}}
	id, gone := int64(4), int64(9)
	for _, tc := range []struct {
		actions []api.RuleAction
		want    bool
	}{
		{nil, false},
		{[]api.RuleAction{{Kind: api.RuleActionKindMove}}, false},
		{[]api.RuleAction{{Kind: api.RuleActionKindMove, MailboxID: &gone}}, false},
		{[]api.RuleAction{{Kind: api.RuleActionKindMove, MailboxID: &id}}, true},
		{[]api.RuleAction{{Kind: api.RuleActionKindRead}}, true},
	} {
		if got := organize.ActionsComplete(tc.actions, boxes); got != tc.want {
			t.Errorf("%+v = %v", tc.actions, got)
		}
	}
}

func noClient() organize.Deps {
	return organize.Deps{Post: func(fn func()) { fn() }, Client: func() *api.Client { return nil },
		Context: func() organize.Context { return organize.Context{} }}
}

// retype replaces what the field named name holds.
func retype(t *testing.T, tt *ui.Tester, name, text string) {
	t.Helper()
	tt.Frame()
	a, ok := tt.AccessByRole(ui.RoleTextField, name)
	if !ok {
		t.Fatalf("no field %q in %q", name, tt.Texts())
	}
	tt.ClickAt(a.Bounds.X+a.Bounds.W/2, a.Bounds.Y+a.Bounds.H/2)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type(text)
}

// pick chooses an option of the drop-down named name.
func pick(t *testing.T, tt *ui.Tester, name, option string) {
	t.Helper()
	for _, step := range []struct {
		role ui.Role
		name string
	}{{ui.RolePopUpButton, name}, {ui.RoleOption, option}} {
		tt.Frame()
		a, ok := tt.AccessByRole(step.role, step.name)
		if !ok {
			t.Fatalf("no %q in %q", step.name, tt.Texts())
		}
		tt.ClickAt(a.Bounds.X+a.Bounds.W/2, a.Bounds.Y+a.Bounds.H/2)
	}
}

func click(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	tt.Frame()
	if err := tt.Click(name); err != nil {
		t.Fatalf("click %q: %v", name, err)
	}
}

func someConditions() api.Conditions {
	return api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{{Field: api.ConditionFieldFrom, Op: api.ConditionOpContains, Value: "ann"}}}
}

// The editors change the sheet's slices in place: what Edit was given, which
// a list holds, keeps what it had.
func TestCancellingARuleLeavesTheOriginal(t *testing.T) {
	box := int64(4)
	rule := api.Rule{ID: 1, Name: "R", Conditions: someConditions(), Actions: []api.RuleAction{{Kind: api.RuleActionKindMove, MailboxID: &box}}}
	s := organize.RuleSheet{Deps: noClient()}
	s.Edit(rule)
	tt := ui.NewTester(s.View, 900, 700)
	retype(t, tt, "Condition 1 value", "changed")
	pick(t, tt, "Action 1", "Mark as Read")
	click(t, tt, "Cancel")
	if got := rule.Conditions.Conditions[0].Value; got != "ann" {
		t.Errorf("the condition's value became %q", got)
	}
	if got := rule.Actions[0]; got.Kind != api.RuleActionKindMove || got.MailboxID == nil || *got.MailboxID != 4 {
		t.Errorf("the action became %+v", got)
	}
}

func TestCancellingASmartMailboxLeavesTheOriginal(t *testing.T) {
	smart := api.SmartMailbox{ID: 1, Name: "S", Conditions: someConditions()}
	s := organize.SmartSheet{Deps: noClient()}
	s.Edit(smart)
	tt := ui.NewTester(s.View, 900, 700)
	retype(t, tt, "Condition 1 value", "changed")
	click(t, tt, "Cancel")
	if got := smart.Conditions.Conditions[0].Value; got != "ann" {
		t.Errorf("the condition's value became %q", got)
	}
}

// A menu tells its items by label: two flags of one name stay two choices.
func TestPickerTellsRepeatedLabelsApart(t *testing.T) {
	ctx := organize.Context{FlagNames: []string{"Work", "Work"}}
	conds := api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{{Field: api.ConditionFieldColor, Op: api.ConditionOpIs, Value: "1"}}}
	tt := ui.NewTester(func(c *ui.Context) { organize.ConditionEditor(c, &conds, ctx, "If", "of these") }, 900, 700)
	pick(t, tt, "Condition 1 value", "Work (2)")
	if got := conds.Conditions[0].Value; got != "2" {
		t.Fatalf("the value is %q, want the second flag", got)
	}
	tt.Frame()
	if a, ok := tt.AccessByRole(ui.RolePopUpButton, "Condition 1 value"); !ok || a.Value != "Work (2)" {
		t.Errorf("the menu shows %+v, %v", a, ok)
	}
	pick(t, tt, "Condition 1 value", "Work")
	if got := conds.Conditions[0].Value; got != "1" {
		t.Errorf("the value is %q, want the first flag", got)
	}
}

// A sheet taller than the window scrolls its rows and keeps its buttons in
// view.
func TestALongSheetKeepsItsButtons(t *testing.T) {
	conds := api.Conditions{Match: api.ConditionMatchAll}
	for range 40 {
		conds.Conditions = append(conds.Conditions, organize.NewCondition())
	}
	const height = 400
	for name, open := range map[string]func() func(*ui.Context){
		"rule": func() func(*ui.Context) {
			s := &organize.RuleSheet{Deps: noClient()}
			s.Edit(api.Rule{ID: 1, Name: "R", Conditions: conds, Actions: []api.RuleAction{{Kind: api.RuleActionKindRead}}})
			return s.View
		},
		"smart": func() func(*ui.Context) {
			s := &organize.SmartSheet{Deps: noClient()}
			s.Edit(api.SmartMailbox{ID: 1, Name: "S", Conditions: conds})
			return s.View
		},
	} {
		t.Run(name, func(t *testing.T) {
			tt := ui.NewTester(open(), 900, height)
			tt.Frame()
			for _, button := range []string{"Cancel", "OK"} {
				a, ok := tt.AccessByRole(ui.RoleButton, button)
				if !ok {
					t.Fatalf("no %s in %q", button, tt.Texts())
				}
				if bottom := a.Bounds.Y + a.Bounds.H; a.Bounds.Y < 0 || bottom > height {
					t.Errorf("%s spans %v to %v in a window %d tall", button, a.Bounds.Y, bottom, height)
				}
			}
		})
	}
}
