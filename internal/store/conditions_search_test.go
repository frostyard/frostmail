package store

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/search"
)

// TestSearchAsConditions: a search saved as conditions lists exactly the
// messages the search does (docs/design/organize.md, From a search).
func TestSearchAsConditions(t *testing.T) {
	f := newConditionsFixture(t)
	for _, text := range []string{
		"lunch", "from:ann", "to:ann", "cc:bob", "-lunch", `subject:"lunch plans"`, "filename:q3", "report -draft",
		"is:unread", "is:read", "is:flagged has:attachment", "-has:attachment", "in:trash", "in:sent in:trash",
		"after:2026-10-06", "before:2026-10-06", "on:2026-10-06", "newer_than:2d", "older_than:10d", "in:nowhere",
	} {
		q := search.Parse(text, testNow, time.Local)
		// The view filter the engine builds for the search (applySearch).
		bySearch := ViewFilter{Match: q.Match(), Exclude: q.Exclude(), Unread: q.Unread, Flagged: q.Flagged,
			HasAttachment: q.HasAttachment, After: q.After, Before: q.Before}
		for _, r := range q.Roles {
			bySearch.Roles = append(bySearch.Roles, string(r))
		}
		want, err := f.d.ViewIDs(t.Context(), bySearch)
		if err != nil {
			t.Fatal(err)
		}
		c := search.ToConditions(q, time.Local)
		if err := CheckConditions(c); err != nil {
			t.Fatalf("ToConditions(%q) = %+v: %v", text, c, err)
		}
		got, err := f.d.ViewIDs(t.Context(), ViewFilter{Conditions: &c})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%q as conditions lists %v; the search lists %v", text, got, want)
		}
	}
}
