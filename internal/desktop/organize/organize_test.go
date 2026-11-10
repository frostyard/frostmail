package organize_test

import (
	"testing"

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
