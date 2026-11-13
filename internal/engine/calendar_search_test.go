package engine_test

import (
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func (e *calendarEnv) search(t *testing.T, p api.CalendarSearchParams) []api.Occurrence {
	t.Helper()
	if p.TimeZone == nil {
		p.TimeZone = ptr("UTC")
	}
	occ, err := e.c.Calendar().Search(t.Context(), &p)
	if err != nil {
		t.Fatalf("search %q: %v", p.Query, err)
	}
	return occ
}

func TestCalendarSearch(t *testing.T) {
	e := newCalendarEnv(t)
	series := []string{"10-09T07:00 Standup", "10-11T07:00 Standup", "10-12T07:00 Standup", "10-13T07:00 Standup",
		"10-14T07:00 Standup", "10-15T07:00 Standup"}
	for _, tc := range []struct {
		name  string
		query string
		want  []string
	}{
		{"a title, whatever its case, with the override under its own title", "STANDUP",
			[]string{"10-09T07:00 Standup", "10-10T08:00 Standup (late)", "10-11T07:00 Standup", "10-12T07:00 Standup",
				"10-13T07:00 Standup", "10-14T07:00 Standup", "10-15T07:00 Standup"}},
		{"part of a word, in two events", "late", []string{"10-08T23:00 Late call", "10-10T08:00 Standup (late)"}},
		{"every word must match", "late standup", []string{"10-10T08:00 Standup (late)"}},
		{"notes, which the override lacks", "daily", series},
		{"location", "room", series},
		{"an invitee's address", "bob@example.com", series},
		{"the organizer's name", "maria", append([]string{"10-09T07:00 Standup", "10-10T08:00 Standup (late)"}, series[1:]...)},
		{"a read-only calendar", "dentist", []string{"10-09T14:00 Dentist"}},
		{"an all-day event", "holiday", []string{"10-12T00:00 Holiday"}},
		{"nothing", "zeppelin", nil},
	} {
		if got := lines(e.search(t, api.CalendarSearchParams{Query: tc.query})); !slices.Equal(got, tc.want) {
			t.Errorf("%s: search %q =\n%q\nwant\n%q", tc.name, tc.query, got, tc.want)
		}
	}

	if got := e.search(t, api.CalendarSearchParams{Query: "zeppelin"}); got == nil {
		t.Error("no match answers null, want an empty list")
	}
	if got := lines(e.search(t, api.CalendarSearchParams{Query: "dentist", CalendarIDs: []int64{e.work}})); len(got) != 0 {
		t.Errorf("dentist in Work = %q", got)
	}
	holiday := e.search(t, api.CalendarSearchParams{Query: "holiday", TimeZone: ptr("Europe/Berlin")})
	if len(holiday) != 1 || holiday[0].StartDate != "2026-10-12" || lines(holiday)[0] != "10-11T22:00 Holiday" {
		t.Errorf("an all-day occurrence in Berlin = %+v", holiday)
	}
}

// The weekly Review, on Mondays at 15:00 UTC since January 2026, has more
// occurrences than any limit; the clock reads Thursday 2026-10-08 12:00.
func TestCalendarSearchKeepsTheNearest(t *testing.T) {
	e := newCalendarEnv(t)
	got := lines(e.search(t, api.CalendarSearchParams{Query: "review", Limit: ptr(int64(3))}))
	if want := []string{"09-28T15:00 Review", "10-05T15:00 Review", "10-12T15:00 Review"}; !slices.Equal(got, want) {
		t.Errorf("the 3 nearest = %q, want %q", got, want)
	}
	all := e.search(t, api.CalendarSearchParams{Query: "carol@example.com"})
	if len(all) != 100 {
		t.Fatalf("the default limit kept %d", len(all))
	}
	if !slices.IsSortedFunc(all, func(a, b api.Occurrence) int { return a.Start.Compare(b.Start) }) {
		t.Error("not in start order")
	}
	if first := all[0].Start.Format("2006-01-02"); first != "2026-01-05" {
		t.Errorf("the first kept = %s, want the series' start: 40 before now and 60 after", first)
	}
	most := e.search(t, api.CalendarSearchParams{Query: "review", Limit: ptr(int64(500))})
	// Mondays from 2026-01-05 to 2028-10-02, inside the 2 years ahead.
	if len(most) != 144 || most[143].Start.Format("2006-01-02") != "2028-10-02" {
		t.Errorf("all = %d, the last %s", len(most), most[len(most)-1].Start.Format("2006-01-02"))
	}
}

func TestCalendarSearchParams(t *testing.T) {
	e := newCalendarEnv(t)
	for _, p := range []api.CalendarSearchParams{
		{Query: ""},
		{Query: "  \t "},
		{Query: "standup", Limit: ptr(int64(0))},
		{Query: "standup", Limit: ptr(int64(501))},
		{Query: "standup", TimeZone: ptr("Mars/Olympus")},
	} {
		if _, err := e.c.Calendar().Search(t.Context(), &p); code(err) != api.CodeInvalidParams {
			t.Errorf("search %+v: %v", p, err)
		}
	}
}
