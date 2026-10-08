package search

// CONTRACT TEST for task card T-0048 (docs/tasks). Do not edit.

import (
	"reflect"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

var (
	loc = time.FixedZone("EDT", -4*3600)
	now = time.Date(2026, 10, 8, 15, 0, 0, 0, loc)
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, loc) }

func ptr(b bool) *bool { return &b }

func TestParseTerms(t *testing.T) {
	cases := []struct {
		in   string
		want []Term
	}{
		{"lunch", []Term{{Text: "lunch"}}},
		{"  lunch   plans ", []Term{{Text: "lunch"}, {Text: "plans"}}},
		{`"q3 plan" report`, []Term{{Text: "q3 plan", Phrase: true}, {Text: "report"}}},
		{"report -draft", []Term{{Text: "report"}, {Text: "draft", Not: true}}},
		{`-"old news"`, []Term{{Text: "old news", Phrase: true, Not: true}}},
		{"from:ann To:bob cc:carol", []Term{{Column: "from_text", Text: "ann"}, {Column: "to_text", Text: "bob"}, {Column: "to_text", Text: "carol"}}},
		{`subject:"q3 plan" filename:pdf`, []Term{{Column: "subject", Text: "q3 plan", Phrase: true}, {Column: "attachment_names", Text: "pdf"}}},
		{"-from:bob", []Term{{Column: "from_text", Text: "bob", Not: true}}},
		{`say "hi`, []Term{{Text: "say"}, {Text: "hi", Phrase: true}}},
		{`it's 5"x`, []Term{{Text: "it's"}, {Text: `5"x`}}},
		{`- -- !!! "..." ""`, nil},
		{"size:3 in:nowhere after:yesterday from:", []Term{{Text: "size:3"}, {Text: "in:nowhere"}, {Text: "after:yesterday"}, {Text: "from:"}}},
		{"is:important has:pdf", []Term{{Text: "is:important"}, {Text: "has:pdf"}}},
		{"-in:trash -after:2026-01-01", []Term{{Text: "in:trash", Not: true}, {Text: "after:2026-01-01", Not: true}}},
		{"after:2026-02-30 on:2026-13-01", []Term{{Text: "after:2026-02-30"}, {Text: "on:2026-13-01"}}},
		{"newer_than:0d newer_than:2x older_than:d", []Term{{Text: "newer_than:0d"}, {Text: "newer_than:2x"}, {Text: "older_than:d"}}},
	}
	for _, c := range cases {
		q := Parse(c.in, now, loc)
		if !reflect.DeepEqual(q.Terms, c.want) {
			t.Errorf("Parse(%q).Terms = %#v, want %#v", c.in, q.Terms, c.want)
		}
	}
}

func TestParseFlags(t *testing.T) {
	cases := []struct {
		in                      string
		unread, flagged, attach *bool
	}{
		{"is:unread", ptr(true), nil, nil},
		{"is:read", ptr(false), nil, nil},
		{"-is:unread", ptr(false), nil, nil},
		{"-is:read", ptr(true), nil, nil},
		{"IS:Unread is:read", ptr(false), nil, nil},
		{"is:flagged", nil, ptr(true), nil},
		{"is:starred", nil, ptr(true), nil},
		{"-is:flagged", nil, ptr(false), nil},
		{"has:attachment", nil, nil, ptr(true)},
		{"-has:attachment", nil, nil, ptr(false)},
		{"is:unread is:flagged has:attachment", ptr(true), ptr(true), ptr(true)},
	}
	for _, c := range cases {
		q := Parse(c.in, now, loc)
		if !reflect.DeepEqual(q.Unread, c.unread) || !reflect.DeepEqual(q.Flagged, c.flagged) || !reflect.DeepEqual(q.HasAttachment, c.attach) {
			t.Errorf("Parse(%q): unread %v flagged %v attachment %v", c.in, q.Unread, q.Flagged, q.HasAttachment)
		}
		if len(q.Terms) != 0 {
			t.Errorf("Parse(%q) left terms %#v", c.in, q.Terms)
		}
	}
}

func TestParseDates(t *testing.T) {
	cases := []struct {
		in            string
		after, before time.Time
	}{
		{"after:2026-09-01", day(2026, 9, 1), time.Time{}},
		{"before:2026/10/01", time.Time{}, day(2026, 10, 1)},
		{"after:2026-09-01 before:2026/10/01", day(2026, 9, 1), day(2026, 10, 1)},
		{"on:2026-10-07", day(2026, 10, 7), day(2026, 10, 8)},
		{"after:2026-01-01 after:2026-02-01", day(2026, 2, 1), time.Time{}},
		{"on:2026-10-07 before:2026-12-01", day(2026, 10, 7), day(2026, 12, 1)},
		{"newer_than:2d", day(2026, 10, 6), time.Time{}},
		{"older_than:1w", time.Time{}, day(2026, 10, 1)},
		{"newer_than:1m", day(2026, 9, 8), time.Time{}},
		{"older_than:1y", time.Time{}, day(2025, 10, 8)},
	}
	for _, c := range cases {
		q := Parse(c.in, now, loc)
		if !q.After.Equal(c.after) || !q.Before.Equal(c.before) {
			t.Errorf("Parse(%q): after %v before %v; want %v and %v", c.in, q.After, q.Before, c.after, c.before)
		}
		if !q.After.IsZero() && q.After.Location() != loc {
			t.Errorf("Parse(%q): after is in %v, want %v", c.in, q.After.Location(), loc)
		}
	}
}

func TestParseRoles(t *testing.T) {
	q := Parse("in:sent in:Inbox in:sent in:spam in:drafts in:trash in:archive in:junk", now, loc)
	want := []api.MailboxRole{api.MailboxRoleSent, api.MailboxRoleInbox, api.MailboxRoleJunk, api.MailboxRoleDrafts, api.MailboxRoleTrash, api.MailboxRoleArchive}
	if !reflect.DeepEqual(q.Roles, want) {
		t.Errorf("Roles = %v, want %v", q.Roles, want)
	}
}

func TestMatchAndExclude(t *testing.T) {
	cases := []struct {
		in, match, exclude string
	}{
		{"lunch", `"lunch"*`, ""},
		{`from:ann subject:"q3 plan"`, `from_text : "ann"* subject : "q3 plan"`, ""},
		{"report -draft", `"report"*`, `"draft"*`},
		{"-from:bob -spam", "", `(from_text : "bob"* OR "spam"*)`},
		{`it's 5"x`, `"it's"* "5""x"*`, ""},
		{`filename:"a b" to:c`, `attachment_names : "a b" to_text : "c"*`, ""},
		{"is:unread has:attachment", "", ""},
		{"size:3", `"size:3"*`, ""},
	}
	for _, c := range cases {
		q := Parse(c.in, now, loc)
		if got := q.Match(); got != c.match {
			t.Errorf("Parse(%q).Match() = %q, want %q", c.in, got, c.match)
		}
		if got := q.Exclude(); got != c.exclude {
			t.Errorf("Parse(%q).Exclude() = %q, want %q", c.in, got, c.exclude)
		}
	}
}

func TestEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "- !!!", `""`} {
		if !Parse(in, now, loc).Empty() {
			t.Errorf("Parse(%q) is not empty", in)
		}
	}
	for _, in := range []string{"a", "-a", "is:read", "has:attachment", "after:2026-01-01", "in:inbox", "-is:flagged"} {
		if Parse(in, now, loc).Empty() {
			t.Errorf("Parse(%q) is empty", in)
		}
	}
}
