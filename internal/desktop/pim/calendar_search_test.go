package pim

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
)

// resultLabels are the results' rows as the list names them, headings
// by their text.
func (f *fixture) resultLabels() []string {
	var out []string
	for _, r := range f.m.cal.search.rows {
		if r.occ == nil {
			out = append(out, r.heading)
			continue
		}
		out = append(out, title(r.occ)+", "+f.m.clock.searchWhen(r.occ))
	}
	return out
}

// The clock reads Thursday, October 8, 2026, 10:30 UTC; Sync repeats at
// 11:00 from Monday to Friday of that week.
func TestCalendarSearchListsUpcomingThenEarlier(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	if _, ok := f.tt.Find("Search Events (Ctrl+Alt+F)"); !ok || f.m.cal.search.open {
		t.Fatal("the bar wants the Search Events button, and no field until it is asked for")
	}
	f.tt.Key(ui.Cmd|ui.Alt, ui.KeyF)
	f.until("the field in place of the paging and view controls", func() bool {
		_, button := f.tt.Find("Search Events (Ctrl+Alt+F)")
		return f.m.cal.search.focused && !button && !f.tt.HasText("Today") && !f.tt.HasText("Week") && f.tt.HasText("October 2026")
	})
	snapshot(t, f.tt, "calendar-bar")
	f.tt.Type("sync")
	f.tt.Frame()
	f.until("the results after a pause", func() bool { return f.tt.HasText("Search Results") && f.tt.HasText("5 results") })
	want := []string{
		"Upcoming",
		"Sync, Thu, Oct 8, 2026 · 11:00 – 11:30 AM",
		"Sync, Fri, Oct 9, 2026 · 11:00 – 11:30 AM",
		"Earlier",
		"Sync, Wed, Oct 7, 2026 · 11:00 – 11:30 AM",
		"Sync, Tue, Oct 6, 2026 · 11:00 – 11:30 AM",
		"Sync, Mon, Oct 5, 2026 · 11:00 – 11:30 AM",
	}
	if got := f.resultLabels(); !slices.Equal(got, want) {
		t.Fatalf("rows =\n%q\nwant\n%q", got, want)
	}
	if f.tt.HasText("Standup") {
		t.Error("the view still shows under the results")
	}

	// A result selects its occurrence and its day; Escape returns to the
	// view on that day, with the event still selected.
	f.click(want[2])
	f.until("the event pane", func() bool {
		e := f.m.shownEvent()
		return e != nil && e.Summary == "Sync" && f.tt.HasText("Friday, October 9, 2026")
	})
	if f.m.cal.date != "2026-10-09" {
		t.Errorf("date = %s", f.m.cal.date)
	}
	snapshot(t, f.tt, "calendar-search")
	// The arrows move through the rows, past a heading; Enter opens the editor.
	f.tt.Key(0, ui.KeyDown)
	f.until("the next row, under Earlier", func() bool { return f.m.cal.date == "2026-10-07" })
	f.tt.Key(0, ui.KeyUp)
	f.until("the row above the heading", func() bool { return f.m.cal.date == "2026-10-09" })
	f.until("the event", func() bool { e := f.m.shownEvent(); return e != nil && e.Summary == "Sync" })
	f.tt.Key(0, ui.KeyEnter)
	f.until("the editor", func() bool { return len(f.windows) == 1 })
	f.closeWindow(f.window("event-").spec.Key)
	f.tt.Key(0, ui.KeyEscape)
	f.until("the view", func() bool { return f.tt.HasText("Today") && !f.tt.HasText("Search Results") })
	if s := f.m.cal.search; s.draft != "" || s.query != "" {
		t.Errorf("Escape left the search %q %q", s.draft, s.query)
	}
	if f.m.cal.selected == nil || f.m.cal.date != "2026-10-09" {
		t.Errorf("Escape lost the selection or the day: %v %s", f.m.cal.selected, f.m.cal.date)
	}
	f.tt.Key(0, ui.KeyEscape)
	f.until("no selection", func() bool { return f.m.cal.selected == nil })
}

func TestCalendarSearchSaysWhereWhenAndCancelled(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	set := func(q string) {
		f.m.cal.search.draft = q
		f.m.setSearch()
	}
	set("room 4")
	f.until("a place", func() bool {
		return slices.Equal(f.resultLabels(), []string{"Earlier", "Standup, Thu, Oct 8, 2026 · 9:00 – 9:15 AM · Room 4"})
	})
	f.text("1 result")
	set("offsite")
	f.until("a cancelled event", func() bool {
		return slices.Equal(f.resultLabels(), []string{"Upcoming", "Offsite, Fri, Oct 9, 2026 · 10:00 – 11:00 AM · Cancelled"})
	})
	set("holiday")
	f.until("an all-day event", func() bool {
		return slices.Equal(f.resultLabels(), []string{"Upcoming", "Holiday, Fri, Oct 9, 2026 · All day"})
	})
	set("zzzz")
	f.until("no results", func() bool { return f.tt.HasText("No Results") && f.tt.HasText("0 results") })
}

// A change to the calendar asks the search again.
func TestCalendarSearchFollowsChanges(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	f.m.cal.search.draft = "lunch"
	f.m.setSearch()
	f.until("the result", func() bool { return len(f.m.cal.search.results.value) == 1 })
	o := f.m.cal.search.results.value[0]
	if _, err := f.c.Calendar().Delete(t.Context(), &api.CalendarDeleteParams{ID: o.EventID}); err != nil {
		t.Fatal(err)
	}
	f.until("the result gone", func() bool { return f.tt.HasText("No Results") })
}

// The field opens from the bar's button and closes when left empty.
func TestCalendarSearchFieldClosesWhenLeftEmpty(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	f.click("Search Events (Ctrl+Alt+F)")
	f.until("the open field", func() bool { return f.m.cal.search.focused })
	f.click("Standup, 9:00 – 9:15 AM, Room 4")
	f.until("the bar's controls again", func() bool { return !f.m.cal.search.open && f.tt.HasText("Today") })
	f.tt.Key(ui.Cmd|ui.Alt, ui.KeyF)
	f.until("the open field", func() bool { return f.m.cal.search.focused })
	f.tt.Key(0, ui.KeyEscape)
	f.until("Escape closes it", func() bool { return !f.m.cal.search.open && f.tt.HasText("Today") })
	if f.m.cal.selected == nil {
		t.Error("Escape in the empty field also ended the selection")
	}
}
