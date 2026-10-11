package search

import (
	"reflect"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestWords(t *testing.T) {
	cases := []struct {
		in   string
		want []Term
	}{
		{"lunch plans", []Term{{Column: "subject", Text: "lunch"}, {Column: "subject", Text: "plans"}}},
		{`  "lunch plans" `, []Term{{Column: "subject", Text: "lunch plans", Phrase: true}}},
		{`it's 5"x -- !`, []Term{{Column: "subject", Text: "it's"}, {Column: "subject", Text: `5"x`}}},
		{`"!!"`, nil},
		{"", nil},
	}
	for _, c := range cases {
		if got := Words(c.in, "subject"); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Words(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestToConditions(t *testing.T) {
	cond := func(f api.ConditionField, op api.ConditionOp, v string) api.Condition {
		return api.Condition{Field: f, Op: op, Value: v}
	}
	cases := []struct {
		in   string
		want []api.Condition
	}{
		{`lunch "q3 plan" -draft`, []api.Condition{
			cond("content", "contains", "lunch"), cond("content", "contains", `"q3 plan"`), cond("content", "notcontains", "draft")}},
		{"from:ann to:bob cc:carol subject:x filename:q3", []api.Condition{
			cond("from", "contains", "ann"), cond("recipient", "contains", "bob"), cond("recipient", "contains", "carol"),
			cond("subject", "contains", "x"), cond("filename", "contains", "q3")}},
		{"is:read is:flagged -has:attachment", []api.Condition{
			cond("unread", "is", "false"), cond("flagged", "is", "true"), cond("attachments", "is", "false")}},
		{"on:2026-10-07", []api.Condition{cond("received", "on", "2026-10-07")}},
		{"after:2026-09-01 before:2026/10/01", []api.Condition{
			cond("received", "since", "2026-09-01"), cond("received", "before", "2026-10-01")}},
		{"newer_than:2D older_than:1y", []api.Condition{
			cond("received", "within", "2d"), cond("received", "notwithin", "1y")}},
		{"newer_than:2d after:2026-09-01", []api.Condition{cond("received", "since", "2026-09-01")}},
		{"in:sent in:spam", []api.Condition{cond("role", "anyof", "sent,junk")}},
		{"", []api.Condition{}},
	}
	for _, c := range cases {
		got := ToConditions(Parse(c.in, now, loc), loc)
		want := api.Conditions{Match: api.ConditionMatchAll, Conditions: c.want}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ToConditions(%q) = %+v\nwant %+v", c.in, got, want)
		}
	}
}
