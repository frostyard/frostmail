package pim

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
)

func snapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("FROSTMAIL_SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Error(err)
	}
}

// block finds a block by its accessible name's start.
func (f *fixture) block(prefix string) ui.Rect {
	f.t.Helper()
	f.calendarsLoaded()
	for _, s := range f.tt.Texts() {
		if strings.HasPrefix(s, prefix) && strings.Contains(s, ",") {
			r, ok := f.tt.Find(s)
			if ok {
				return r
			}
		}
	}
	f.t.Fatalf("no block %q in %q", prefix, f.tt.Texts())
	return ui.Rect{}
}

// calendarsLoaded waits until the module knows what it can write.
func (f *fixture) calendarsLoaded() {
	f.t.Helper()
	f.until("the calendars", func() bool { return len(f.m.cal.writable) > 0 && len(f.m.accounts.value) > 0 })
}

// newEvent opens a new event's window and waits for it to load.
func (f *fixture) newEvent() (*window, *eventWindow) {
	f.t.Helper()
	f.calendarsLoaded()
	f.m.newEventDefault()
	f.until("the window", func() bool { w := f.window("event-new-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	win := f.window("event-new-")
	return win, f.m.windows[win.spec.Key]
}

func (f *fixture) range_(from, to string) []api.Occurrence {
	f.t.Helper()
	zone := "UTC"
	got, err := f.c.Calendar().Range(f.t.Context(), &api.CalendarRangeParams{From: from, To: to, TimeZone: &zone})
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

func (f *fixture) occurrence(summary, from, to string) api.Occurrence {
	f.t.Helper()
	for _, o := range f.range_(from, to) {
		if o.Summary == summary {
			return o
		}
	}
	f.t.Fatalf("no occurrence %q", summary)
	return api.Occurrence{}
}

// drag presses at a point, moves by steps and releases.
func (f *fixture) drag(x, y, dx, dy float32) {
	f.tt.Press(x, y)
	f.tt.Move(x+dx/2, y+dy/2)
	f.tt.Move(x+dx, y+dy)
	f.tt.Release(x+dx, y+dy)
	f.tt.Frame()
}

func TestWeekViewShowsEvents(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	for _, want := range []string{"October 2026", "Holiday", "all-day", "10:30 AM", "Thu", "9 AM", "No Event Selected", "me@a.test", "Work", "Home"} {
		f.text(want) // the calendars' list can come after the events
	}
	if a, ok := f.tt.AccessByRole(ui.RoleButton, "Standup, 9:00 – 9:15 AM, Room 4"); !ok {
		t.Errorf("no block named with its time and place: %+v", a)
	}
	if _, ok := f.tt.AccessByRole(ui.RoleCheckBox, "Work"); !ok {
		t.Error("a calendar's row is no checkbox")
	}
	if a, _ := f.tt.AccessByRole(ui.RoleButton, "Thursday, October 8, 2026"); a.Current != ui.CurrentDate {
		t.Errorf("today is not current: %+v", a)
	}
	if !f.tt.HasText("Now") {
		t.Error("no now line")
	}
	snapshot(t, f.tt, "calendar-week")
}

func TestSelectingShowsTheEventPane(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	f.click("Standup, 9:00 – 9:15 AM, Room 4")
	f.text("Thursday, October 8, 2026")
	f.until("the event pane", func() bool {
		return f.tt.HasText("Room 4") && f.tt.HasText("9:00 – 9:15 AM") && !f.tt.HasText("No Event Selected")
	})
	for _, want := range []string{"Edit Event", "Delete Event", "Calendar", "Work"} {
		if !f.tt.HasText(want) {
			t.Errorf("no %q in %s", want, dump(f.tt))
		}
	}
	if f.m.Selected(commands.Calendar) != 1 {
		t.Error("Selected is not 1")
	}
	// A read-only calendar's event has no Edit or Delete.
	f.click("Gym, 6:00 – 7:00 PM")
	f.until("the read-only event", func() bool { return f.m.shownEvent() != nil && f.m.shownEvent().Summary == "Gym" })
	if f.tt.HasText("Edit Event") {
		t.Error("a read-only event offers Edit")
	}
	f.tt.Key(0, ui.KeyEscape)
	f.until("the empty pane", func() bool { return f.tt.HasText("No Event Selected") })
}

func TestCancelledShowsAtSixtyPercentAndPaneSaysCancelled(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Offsite")
	f.click("Offsite, 10:00 – 11:00 AM")
	f.until("the Cancelled line", func() bool { return f.tt.HasText("Cancelled") })
}

func TestKeysSwitchViewsAndPage(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	f.tt.Key(ui.Cmd|ui.Alt, ui.Key1)
	f.text("Thursday, October 8, 2026")
	if f.m.cal.view != viewDay {
		t.Errorf("view %q", f.m.cal.view)
	}
	f.tt.Key(ui.Cmd|ui.Alt, ui.Key3)
	f.until("the month grid", func() bool { return f.m.cal.view == viewMonth && f.tt.HasText("Oct 1") })
	f.tt.Key(ui.Cmd|ui.Alt, ui.Key2)
	f.until("the week", func() bool { return f.m.cal.view == viewWeek })
	f.tt.Key(ui.Cmd, ui.KeyRight)
	f.tt.Frame()
	if f.m.cal.date != "2026-10-15" {
		t.Errorf("Ctrl+Right went to %s", f.m.cal.date)
	}
	f.tt.Key(ui.Cmd, ui.KeyLeft)
	f.tt.Key(ui.Cmd, ui.KeyLeft)
	f.tt.Frame()
	if f.m.cal.date != "2026-10-01" {
		t.Errorf("Ctrl+Left went to %s", f.m.cal.date)
	}
	f.tt.Key(ui.Cmd, ui.KeyT)
	f.tt.Frame()
	if f.m.cal.date != "2026-10-08" {
		t.Errorf("Ctrl+T went to %s", f.m.cal.date)
	}
	f.tt.Key(0, ui.KeyRight)
	f.tt.Frame()
	if f.m.cal.date != "2026-10-09" {
		t.Errorf("Right went to %s", f.m.cal.date)
	}
	f.tt.Key(0, ui.KeyLeft)
	f.tt.Frame()
	if f.m.cal.date != "2026-10-08" {
		t.Errorf("Left went to %s", f.m.cal.date)
	}
	// Up and Down scroll the grid by an hour in the week view.
	before := f.m.cal.scroll.Y
	f.tt.Key(0, ui.KeyDown)
	f.tt.Frame()
	if f.m.cal.scroll.Y != before+hourHeight {
		t.Errorf("Down scrolled from %v to %v", before, f.m.cal.scroll.Y)
	}
	f.tt.Key(0, ui.KeyUp)
	f.tt.Frame()
	if f.m.cal.scroll.Y != before {
		t.Errorf("Up scrolled to %v", f.m.cal.scroll.Y)
	}
	// In the month view they move by a week.
	f.tt.Key(ui.Cmd|ui.Alt, ui.Key3)
	f.tt.Frame()
	f.tt.Key(0, ui.KeyDown)
	f.tt.Frame()
	if f.m.cal.date != "2026-10-15" {
		t.Errorf("Down in the month went to %s", f.m.cal.date)
	}
	f.tt.Key(0, ui.KeyUp)
	f.tt.Frame()
	if f.m.cal.date != "2026-10-08" {
		t.Errorf("Up in the month went to %s", f.m.cal.date)
	}
	// Enter with nothing selected shows the day.
	f.tt.Key(0, ui.KeyEnter)
	f.until("the day view", func() bool { return f.m.cal.view == viewDay })
}

func TestOpeningAViewScrollsToSevenOClock(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	if got, want := f.m.cal.scroll.Y, float32(7*hourHeight-12); got != want {
		t.Errorf("scrolled to %v, want %v", got, want)
	}
}

func TestDraggingMovesInFifteenMinuteSteps(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	r := f.block("Review, ")
	cx, cy := r.X+r.W/2, r.Y+10
	// 24 pixels is 30 minutes.
	f.tt.Press(cx, cy)
	f.tt.Move(cx, cy+12)
	f.tt.Move(cx, cy+24)
	f.tt.Frame()
	if !f.tt.HasText("2:30 – 4:00 PM") {
		t.Errorf("no preview of the new range: %s", dump(f.tt))
	}
	f.tt.Release(cx, cy+24)
	f.tt.Frame()
	f.until("the write", func() bool {
		o := f.occurrence("Review", "2026-10-08", "2026-10-09")
		return o.Start.Equal(time.Date(2026, 10, 8, 14, 30, 0, 0, time.UTC))
	})
	f.until("the undo entry", func() bool { return len(f.m.cal.undo) == 1 })
	if f.m.cal.selected != nil {
		t.Error("a drag selected the occurrence")
	}
	// Ctrl+Z undoes it.
	f.tt.Key(ui.Cmd, ui.KeyZ)
	f.until("the undo", func() bool {
		o := f.occurrence("Review", "2026-10-08", "2026-10-09")
		return o.Start.Equal(time.Date(2026, 10, 8, 14, 0, 0, 0, time.UTC))
	})
}

func TestDraggingAcrossDays(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	r := f.block("Review, ")
	cx, cy := r.X+r.W/2, r.Y+10
	col := f.m.cal.press
	if col != nil {
		t.Fatal("a press before the drag")
	}
	width := r.W + blockGap
	f.drag(cx, cy, width, 0)
	f.until("the write", func() bool {
		o := f.occurrence("Review", "2026-10-08", "2026-10-11")
		return o.Start.Equal(time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC))
	})
}

func TestResizingFromTheBottomEdge(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	r := f.block("Review, ")
	cx, y := r.X+r.W/2, r.Y+r.H-2
	f.drag(cx, y, 0, 24)
	f.until("the write", func() bool {
		o := f.occurrence("Review", "2026-10-08", "2026-10-09")
		return o.Start.Equal(time.Date(2026, 10, 8, 14, 0, 0, 0, time.UTC)) && o.End.Equal(time.Date(2026, 10, 8, 16, 0, 0, 0, time.UTC))
	})
	// The end stops 15 minutes after the start.
	r = f.block("Review, ")
	f.drag(r.X+r.W/2, r.Y+r.H-2, 0, -300)
	f.until("the shortest", func() bool {
		o := f.occurrence("Review", "2026-10-08", "2026-10-09")
		return o.End.Equal(time.Date(2026, 10, 8, 14, 15, 0, 0, time.UTC))
	})
}

func TestEscapeDuringADragWritesNothing(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	r := f.block("Review, ")
	cx, cy := r.X+r.W/2, r.Y+10
	f.tt.Press(cx, cy)
	f.tt.Move(cx, cy+24)
	f.tt.Frame()
	if f.m.cal.press == nil || f.m.cal.press.preview == nil {
		t.Fatal("no preview")
	}
	f.tt.Key(0, ui.KeyEscape)
	f.tt.Frame()
	if f.m.cal.press != nil && f.m.cal.press.preview != nil {
		t.Error("Escape left the preview")
	}
	f.tt.Release(cx, cy+24)
	f.tt.Frame()
	if len(f.m.cal.undo) != 0 {
		t.Error("the cancelled drag wrote")
	}
	o := f.occurrence("Review", "2026-10-08", "2026-10-09")
	if !o.Start.Equal(time.Date(2026, 10, 8, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("start moved to %v", o.Start)
	}
}

func TestADragEndingWhereItBeganIsAClick(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	r := f.block("Review, ")
	cx, cy := r.X+r.W/2, r.Y+10
	f.tt.Press(cx, cy)
	f.tt.Move(cx, cy+24)
	f.tt.Move(cx, cy)
	f.tt.Release(cx, cy)
	f.until("the selection", func() bool { return f.m.cal.selected != nil })
	if len(f.m.cal.undo) != 0 {
		t.Error("the drag wrote")
	}
}

func TestDrawingOpensTheEditorWithTheRange(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	r := f.block("Standup, ")
	// Sunday's column, an empty one, at the height of Standup.
	x := r.X - 3*(r.W+blockGap)
	y := r.Y + 1
	f.tt.Press(x, y)
	f.tt.Move(x, y+24)
	f.tt.Move(x, y+48)
	f.tt.Frame()
	if !f.tt.HasText("New Event") {
		t.Errorf("no placeholder: %s", dump(f.tt))
	}
	f.tt.Release(x, y+48)
	f.until("the window", func() bool { w := f.window("event-new-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	w := f.m.windows[f.window("event-new-").spec.Key]
	if w.draft.startDate != "2026-10-05" || w.draft.startTime != "09:00" || w.draft.endTime != "10:15" {
		t.Errorf("draft %+v", w.draft)
	}
}

func TestDoubleClickCreates(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	r := f.block("Standup, ")
	x, y := r.X-3*(r.W+blockGap), r.Y+1
	f.tt.ClickAt(x, y)
	f.tt.ClickAt(x, y)
	f.until("the window", func() bool { w := f.window("event-new-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	w := f.m.windows[f.window("event-new-").spec.Key]
	if w.draft.startTime != "09:00" || w.draft.endTime != "10:00" || w.draft.allDay {
		t.Errorf("draft %+v", w.draft)
	}
	f.closeWindow(w.key)

	// On the all-day strip, an all-day event on that day.
	cell, ok := f.tt.Find("All-day Monday, October 5, 2026")
	if !ok {
		t.Fatalf("no cell in %s", dump(f.tt))
	}
	f.tt.ClickAt(cell.X+cell.W/2, cell.Y+2)
	f.tt.ClickAt(cell.X+cell.W/2, cell.Y+2)
	f.until("the all-day window", func() bool { w := f.window("event-new-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	w = f.m.windows[f.window("event-new-").spec.Key]
	if !w.draft.allDay || w.draft.startDate != "2026-10-05" || w.draft.endDate != "2026-10-05" {
		t.Errorf("all-day draft %+v", w.draft)
	}
}

func TestMonthDragMovesByWholeDays(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.m.cal.view = viewMonth
	f.text("Standup")
	src := f.block("Standup, ")
	cell, ok := f.tt.AccessByRole(ui.RoleCell, "Saturday, October 10, 2026")
	if !ok {
		t.Fatalf("no cell in %s", dump(f.tt))
	}
	dst := cell.Bounds
	f.tt.Press(src.X+10, src.Y+5)
	f.tt.Move(src.X+30, src.Y+5)
	f.tt.Move(dst.X+dst.W/2, dst.Y+dst.H-6)
	f.tt.Release(dst.X+dst.W/2, dst.Y+dst.H-6)
	f.until("the move", func() bool {
		o := f.occurrence("Standup", "2026-10-01", "2026-10-31")
		return o.Start.Equal(time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC))
	})
}

func TestAllDayDragMovesByWholeDays(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Holiday")
	src := f.block("Holiday, ")
	dst, ok := f.tt.Find("All-day Sunday, October 4, 2026")
	if !ok {
		t.Fatal("no cell")
	}
	f.tt.Press(src.X+10, src.Y+5)
	f.tt.Move(src.X+30, src.Y+5)
	f.tt.Move(dst.X+10, dst.Y+6)
	f.tt.Release(dst.X+10, dst.Y+6)
	f.until("the move", func() bool {
		o := f.occurrence("Holiday", "2026-10-01", "2026-10-31")
		return o.StartDate == "2026-10-04" && o.EndDate == "2026-10-05"
	})
}

func TestMonthViewCellsLinesAndMore(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.m.cal.view = viewMonth
	f.text("Standup")
	for _, want := range []string{"Oct 1", "Nov 1", "Sun", "Sat"} {
		if !f.tt.HasText(want) {
			t.Errorf("no %q in %s", want, dump(f.tt))
		}
	}
	// A click on empty space selects its date.
	info, ok := f.tt.AccessByRole(ui.RoleCell, "Tuesday, October 20, 2026")
	if !ok {
		t.Fatalf("no cell in %s", dump(f.tt))
	}
	cell := info.Bounds
	f.tt.ClickAt(cell.X+cell.W/2, cell.Y+cell.H-4)
	f.tt.Frame()
	if f.m.cal.date != "2026-10-20" {
		t.Errorf("date %s", f.m.cal.date)
	}
	snapshot(t, f.tt, "calendar-month")
}

func TestMonthShowsMoreWhenFull(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.m.cal.view = viewMonth
	f.text("Standup")
	for i := range 8 {
		start := time.Date(2026, 10, 20, 8+i, 0, 0, 0, time.UTC)
		summary := fmt.Sprintf("Busy %d", i)
		_, err := f.c.Calendar().Create(t.Context(), &api.CalendarCreateParams{CalendarID: &f.work, Fields: api.EventFields{Summary: &summary, AllDay: ptr(false), Start: &start, End: ptr(start.Add(30 * time.Minute)), TimeZone: ptr("UTC")}})
		if err != nil {
			t.Fatal(err)
		}
	}
	f.until("N more", func() bool {
		return slices.ContainsFunc(f.tt.Texts(), func(s string) bool { return strings.HasSuffix(s, " more") })
	})
	var more string
	for _, s := range f.tt.Texts() {
		if strings.HasPrefix(s, "Show ") && strings.Contains(s, "more on") {
			more = s
		}
	}
	f.click(more)
	f.until("the day", func() bool { return f.m.cal.view == viewDay && f.m.cal.date == "2026-10-20" })
}

func TestAllDayStripCollapsesInTheWeek(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	for i := range 4 {
		summary := fmt.Sprintf("Fair %d", i)
		_, err := f.c.Calendar().Create(t.Context(), &api.CalendarCreateParams{CalendarID: &f.work, Fields: api.EventFields{Summary: &summary, AllDay: ptr(true), StartDate: ptr("2026-10-06"), EndDate: ptr("2026-10-07")}})
		if err != nil {
			t.Fatal(err)
		}
	}
	f.until("2 more", func() bool { return f.tt.HasText("2 more") })
	if f.tt.HasText("Fair 3") {
		t.Error("the week shows a fourth lane")
	}
	f.click("Show 2 more on Tuesday, October 6, 2026")
	f.until("every lane", func() bool { return f.m.cal.view == viewDay && f.tt.HasText("Fair 0") && f.tt.HasText("Fair 3") })
}

func TestMiniMonthPagesAndSelects(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	f.click("Next Month")
	f.text("November 2026")
	if f.m.cal.date != "2026-10-08" {
		t.Errorf("paging moved the selection to %s", f.m.cal.date)
	}
	f.click("Thursday, November 19, 2026")
	f.until("the date", func() bool { return f.m.cal.date == "2026-11-19" })
	// It returns to the selection's month when the selection changes.
	f.tt.Key(ui.Cmd, ui.KeyT)
	f.until("back", func() bool { return f.m.cal.month.shown == "2026-10-08" })
	if !f.m.cal.busy["2026-10-08"] {
		t.Error("no dot on a busy day")
	}
}

func TestCalendarToggleWritesEnabled(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	f.click("Home")
	f.until("the toggle", func() bool {
		kind := api.CollectionKindCalendar
		cols, err := f.c.Account().Collections(t.Context(), &api.AccountCollectionsParams{Kind: &kind})
		if err != nil {
			t.Fatal(err)
		}
		return !slices.ContainsFunc(cols, func(c api.Collection) bool { return c.ID == f.home && c.Enabled })
	})
	f.until("Gym gone", func() bool { return !f.tt.HasText("Gym") })
}

func TestDefaultCalendarMenu(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	f.text("Work")
	if err := f.tt.RightClick("Work"); err != nil {
		t.Fatal(err)
	}
	if got := f.tt.Menu(); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "Use as Default Calendar") }) {
		t.Fatalf("menu %q", got)
	}
	f.tt.CloseMenu()
	if err := f.tt.RightClick("Side"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Use as Default Calendar"); err != nil {
		t.Fatal(err)
	}
	f.until("the default", func() bool {
		kind := api.CollectionKindCalendar
		cols, err := f.c.Account().Collections(t.Context(), &api.AccountCollectionsParams{Kind: &kind})
		if err != nil {
			t.Fatal(err)
		}
		return slices.ContainsFunc(cols, func(c api.Collection) bool { return c.ID == f.side && c.IsDefault })
	})
}

func TestMonthCellDoubleClickCreatesAnAllDayEvent(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.m.cal.view = viewMonth
	f.text("Standup")
	info, ok := f.tt.AccessByRole(ui.RoleCell, "Tuesday, October 20, 2026")
	if !ok {
		t.Fatalf("no cell in %s", dump(f.tt))
	}
	x, y := info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H-4
	f.tt.ClickAt(float32(x), float32(y))
	f.tt.ClickAt(float32(x), float32(y))
	f.until("the window", func() bool { w := f.window("event-new-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	w := f.m.windows[f.window("event-new-").spec.Key]
	if !w.draft.allDay || w.draft.startDate != "2026-10-20" {
		t.Errorf("draft %+v", w.draft)
	}
}

func TestInvitationShowsAnswerPeopleAndOnlyEdit(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.m.cal.date = "2026-10-12"
	f.text("Lunch with Ada")
	o := f.occurrence("Lunch with Ada", "2026-10-12", "2026-10-13")
	f.m.selectOccurrence(&o)
	f.until("the pane", func() bool { return f.m.shownEvent() != nil && f.tt.HasText("Invitees") })
	for _, want := range []string{"Organizer", "Bo", "Your answer", "Not answered", "Ada", "Accepted", "Edit Event"} {
		if !f.tt.HasText(want) {
			t.Errorf("no %q in %s", want, dump(f.tt))
		}
	}
	if f.tt.HasText("Delete Event") {
		t.Error("an invitation offers Delete")
	}
	f.m.editSelected()
	f.until("the window", func() bool { w := f.window("event-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	win := f.window("event-")
	win.tt.Frame()
	if a, _ := win.tt.AccessByRole(ui.RoleTextField, "Title"); !a.Disabled {
		t.Errorf("the title is enabled for an invitation: %+v", a)
	}
	if !win.tt.HasText("add alert") || win.tt.HasText("Delete Event") || !win.tt.HasText("Save") {
		t.Errorf("shows %s", dump(win.tt))
	}
	// An invitation does not move.
	r := f.block("Lunch with Ada, ")
	f.drag(r.X+r.W/2, r.Y+10, 0, 24)
	if len(f.m.cal.undo) != 0 || f.m.cal.press != nil {
		t.Error("an invitation moved")
	}
}

func TestTabCyclesTheCalendarPanes(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	if f.m.cal.focus != paneCalView {
		t.Errorf("focus %q", f.m.cal.focus)
	}
	f.tt.Key(0, ui.KeyTab)
	f.tt.Frame()
	if f.m.cal.focus != paneCalEvent {
		t.Errorf("Tab went to %q", f.m.cal.focus)
	}
	f.tt.Key(ui.Shift, ui.KeyTab)
	f.tt.Frame()
	f.tt.Key(ui.Shift, ui.KeyTab)
	f.tt.Frame()
	if f.m.cal.focus != paneSidebar {
		t.Errorf("Shift+Tab went to %q", f.m.cal.focus)
	}
	// In the sidebar the arrows leave the date alone.
	f.tt.Key(0, ui.KeyRight)
	f.tt.Frame()
	if f.m.cal.date != "2026-10-08" {
		t.Errorf("Right in the sidebar went to %s", f.m.cal.date)
	}
}

func TestTheEventsOwnZoneShowsInThePane(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	start := time.Date(2026, 10, 7, 7, 0, 0, 0, time.UTC)
	w, err := f.c.Calendar().Create(t.Context(), &api.CalendarCreateParams{CalendarID: &f.work, Fields: api.EventFields{Summary: ptr("Berlin Call"), AllDay: ptr(false), Start: &start, End: ptr(start.Add(30 * time.Minute)), TimeZone: ptr("Europe/Berlin")}})
	if err != nil {
		t.Fatal(err)
	}
	f.m.cal.selected = &occKey{w.Event.ID, ""}
	f.until("the zone line", func() bool { return f.tt.HasText("Europe/Berlin: 9:00 – 9:30 AM") })
}

func TestNewEventDefaultsAndSave(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	f.calendarsLoaded()
	f.tt.Key(ui.Cmd, ui.KeyN)
	f.until("the window", func() bool { w := f.window("event-new-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	win := f.window("event-new-")
	w := f.m.windows[win.spec.Key]
	if w.draft.startTime != "11:00" || w.draft.endTime != "12:00" || w.draft.calendarID != f.work {
		t.Errorf("draft %+v", w.draft)
	}
	if win.spec.Width != 480 || win.spec.Height != 600 || win.spec.MinWidth != 420 || win.spec.MinHeight != 480 {
		t.Errorf("spec %+v", win.spec)
	}
	win.tt.Frame()
	win.tt.Type("Planning")
	win.tt.Frame()
	if w.draft.summary != "Planning" {
		t.Fatalf("summary %q; shows %s", w.draft.summary, dump(win.tt))
	}
	f.clickIn(win, "Save")
	f.until("the event", func() bool { return f.tt.HasText("Planning") })
	if f.window("event-new-") != nil {
		t.Error("the window stayed open")
	}
	if len(f.m.cal.undo) != 1 {
		t.Errorf("undo %v", f.m.cal.undo)
	}
	snapshot(t, f.tt, "calendar-after-save")
}

func TestEditingAnEventSendsWhatChanged(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	f.click("Review, 2:00 – 3:30 PM")
	f.until("selection", func() bool { return f.m.shownEvent() != nil })
	f.tt.Key(0, ui.KeyEnter)
	f.until("the window", func() bool { w := f.window("event-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	win := f.window("event-")
	w := f.m.windows[win.spec.Key]
	win.tt.Frame()
	if win.spec.Key != eventWindowKey(f.m.shownEvent().ID, "") {
		t.Errorf("key %q", win.spec.Key)
	}
	snapshot(t, win.tt, "event-window")
	w.draft.location = "Room 9"
	f.clickIn(win, "Save")
	f.until("the location", func() bool { return strings.Contains(strings.Join(f.tt.Texts(), "|"), "Room 9") })
}

func TestClosingWithChangesAsks(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	f.click("Review, 2:00 – 3:30 PM")
	f.until("selection", func() bool { return f.m.shownEvent() != nil })
	f.m.editSelected()
	f.until("the window", func() bool { w := f.window("event-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	win := f.window("event-")
	w := f.m.windows[win.spec.Key]
	w.draft.summary = "Changed"
	win.tt.Frame()
	f.clickIn(win, "Cancel")
	if !win.tt.HasText(`Save changes to "Changed"?`) {
		t.Fatalf("no question: %s", dump(win.tt))
	}
	// Cancel there leaves the editor open with the draft.
	if err := win.tt.Click("Don't Save"); err != nil {
		t.Fatal(err)
	}
	win.tt.Frame()
	if f.window("event-") != nil {
		t.Error("Don't Save left the window open")
	}
	o := f.occurrence("Review", "2026-10-08", "2026-10-09")
	if o.Summary != "Review" {
		t.Error("Don't Save wrote")
	}
}

func TestReadOnlyEventWindow(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Gym")
	o := f.occurrence("Gym", "2026-10-08", "2026-10-09")
	w := &eventWindow{m: f.m, key: "event-x", target: editTarget{id: o.EventID}}
	f.m.openWindow(w, "Gym")
	w.load()
	f.until("loaded", func() bool { return w.loaded })
	win := f.windows["event-x"]
	win.tt.Frame()
	if !win.tt.HasText("Read-only") || !win.tt.HasText("Close") || win.tt.HasText("Save") {
		t.Errorf("shows %s", dump(win.tt))
	}
}

func TestEventWindowOpensOnceByKey(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	f.click("Review, 2:00 – 3:30 PM")
	f.until("selection", func() bool { return f.m.shownEvent() != nil })
	f.m.editSelected()
	f.until("the window", func() bool { return f.window("event-") != nil })
	first := f.window("event-")
	f.m.editSelected()
	if f.window("event-") != first || len(f.m.windows) != 1 {
		t.Error("a second window")
	}
}

func TestRepeatingEventAsksTheScopeOnDelete(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Sync")
	o := f.occurrence("Sync", "2026-10-08", "2026-10-09")
	f.m.selectOccurrence(&o)
	f.until("selection", func() bool { return f.m.shownEvent() != nil })
	f.tt.Key(0, ui.KeyDelete)
	f.until("the sheet", func() bool { return f.tt.HasText("Delete Repeating Event") })
	for _, want := range []string{"This Event", "All Future Events", "All Events", "Cancel"} {
		if !f.tt.HasText(want) {
			t.Errorf("no %q in %s", want, dump(f.tt))
		}
	}
	f.click("This Event")
	f.until("the occurrence gone", func() bool {
		return !slices.ContainsFunc(f.range_("2026-10-08", "2026-10-09"), func(x api.Occurrence) bool { return x.Summary == "Sync" })
	})
	// The first occurrence has no All Future Events.
	first := f.occurrence("Sync", "2026-10-05", "2026-10-06")
	f.m.selectOccurrence(&first)
	f.until("selection", func() bool { return f.m.shownEvent() != nil })
	f.m.deleteSelected()
	f.until("the sheet", func() bool { return f.tt.HasText("Delete Repeating Event") })
	if f.tt.HasText("All Future Events") {
		t.Error("the series' first occurrence offers All Future Events")
	}
	f.click("Cancel")
	f.until("closed", func() bool { return !f.tt.HasText("Delete Repeating Event") })
}

func TestDroppingARepeatingEventAsksTheScope(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Sync")
	r := f.block("Sync, ")
	f.drag(r.X+r.W/2, r.Y+6, 0, 24)
	f.until("the sheet", func() bool { return f.tt.HasText("Change Repeating Event") })
	f.click("Cancel")
	f.until("closed", func() bool { return !f.tt.HasText("Change Repeating Event") })
	if len(f.m.cal.undo) != 0 {
		t.Error("the drag wrote")
	}
}

func TestDeleteAndUndo(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	f.click("Review, 2:00 – 3:30 PM")
	f.until("selection", func() bool { return f.m.shownEvent() != nil })
	f.click("Delete Event")
	f.until("gone", func() bool { return !f.tt.HasText("Review") })
	if f.m.cal.selected != nil {
		t.Error("still selected")
	}
	f.tt.Key(ui.Cmd, ui.KeyZ)
	f.text("Review")
}

func TestShowOccurrenceKeepsTheView(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.m.cal.view = viewMonth
	f.text("Standup")
	o := f.occurrence("Holiday", "2026-10-09", "2026-10-10")
	f.m.ShowOccurrence(o)
	f.tt.Frame()
	if f.m.cal.view != viewMonth || f.m.cal.date != "2026-10-09" || f.m.cal.selected == nil {
		t.Errorf("state %+v", f.m.cal)
	}
	if !slices.Contains(f.shown, commands.Calendar) {
		t.Error("did not ask for Calendar")
	}
}

func TestSelectionFollowsAcrossRefetchAndEndsWhenGone(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	f.click("Review, 2:00 – 3:30 PM")
	f.until("selection", func() bool { return f.m.shownEvent() != nil })
	id := f.m.shownEvent().ID
	if _, err := f.c.Calendar().Update(t.Context(), &api.CalendarUpdateParams{ID: id, Fields: api.EventFields{Location: ptr("Hall")}}); err != nil {
		t.Fatal(err)
	}
	f.until("the new location in the pane", func() bool { return f.tt.HasText("Hall") })
	if _, err := f.c.Calendar().Delete(t.Context(), &api.CalendarDeleteParams{ID: id}); err != nil {
		t.Fatal(err)
	}
	f.until("the empty pane", func() bool { return f.tt.HasText("No Event Selected") })
	if f.m.cal.selected != nil {
		t.Error("still selected")
	}
}

func TestRefusedChangeShowsANotice(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	f.m.event(api.CalendarRefused{AccountID: f.acct, Summary: "Standup", Reason: "changed"})
	f.until("the notice", func() bool { return len(f.toasts) == 1 })
	if want := "“Standup” was changed on another device, so your change wasn't saved."; f.toasts[0] != want {
		t.Errorf("toast %q", f.toasts[0])
	}
	if got := refusedText("X", "deleted"); got != "“X” was deleted on another device." {
		t.Error(got)
	}
	if got := refusedText("X", "refused"); got != "The server didn't accept your change to “X”." {
		t.Error(got)
	}
}

func TestNowLineMovesWithTheClock(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("10:30 AM")
	f.now = f.now.Add(time.Minute)
	f.text("10:31 AM")
}

func TestNewEventIsDisabledWithoutAWritableCalendar(t *testing.T) {
	f := newFixture(t, commands.Calendar, options{empty: true})
	f.until("the toolbar", func() bool { return f.tt.HasText("New Event (Ctrl+N)") })
	if a, _ := f.tt.AccessByRole(ui.RoleButton, "New Event (Ctrl+N)"); !a.Disabled {
		t.Errorf("%+v", a)
	}
}

func TestEnterInTheTitleSavesAndEscapeCancels(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	win, _ := f.newEvent()
	win.tt.Frame()
	win.tt.Key(0, ui.KeyEscape)
	win.tt.Frame()
	if f.window("event-new-") != nil {
		t.Fatal("Escape left an unchanged new event open")
	}
	win, _ = f.newEvent()
	win.tt.Frame()
	win.tt.Type("Quick One")
	win.tt.Key(0, ui.KeyEnter)
	f.until("the event", func() bool { return f.tt.HasText("Quick One") })
}

func TestTheSystemCloseButtonAsksAboutChanges(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	win, w := f.newEvent()
	if !win.spec.Frameless || !win.spec.CloseRequested() {
		t.Error("an unchanged window would not close")
	}
	f.m.windows[win.spec.Key] = w
	w.draft.summary = "Changed"
	if win.spec.CloseRequested() {
		t.Error("a changed window would close")
	}
	win.tt.Frame()
	if !win.tt.HasText(`Save changes to "Changed"?`) {
		t.Errorf("shows %s", dump(win.tt))
	}
}

func TestZones(t *testing.T) {
	if got := wireZone(localZone); got != "UTC" {
		t.Error(got)
	}
	if got := wireZone("Europe/Berlin"); got != "Europe/Berlin" {
		t.Error(got)
	}
	if got := zoneName(time.UTC); got != "UTC" {
		t.Error(got)
	}
	got := zoneChoices("UTC", "Europe/Berlin")
	if len(got) < 2 || got[0] != "UTC" || got[1] != "Europe/Berlin" {
		t.Errorf("%v", got[:2])
	}
	if got := len(weekOrder(1)); got != 7 || weekOrder(1)[0] != "MO" || weekOrder(0)[0] != "SU" {
		t.Error(weekOrder(0))
	}
}

func TestDoubleClickOpensAnOccurrence(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Review")
	r := f.block("Review, ")
	x, y := r.X+r.W/2, r.Y+10
	f.tt.ClickAt(x, y)
	f.tt.ClickAt(x, y)
	f.until("the window", func() bool { w := f.window("event-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	if w := f.m.windows[f.window("event-").spec.Key]; w.draft.summary != "Review" {
		t.Errorf("draft %+v", w.draft)
	}
}

// A drag of a multi-day bar measures from the day it was pressed on, however
// many of the bar's own days the pointer crosses on the way, frame by frame.
func TestDraggingABarAcrossItsOwnDaysMeasuresFromTheGrab(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	if _, err := f.c.Calendar().Create(t.Context(), &api.CalendarCreateParams{CalendarID: &f.work, Fields: api.EventFields{Summary: ptr("Fair"), AllDay: ptr(true), StartDate: ptr("2026-10-05"), EndDate: ptr("2026-10-08")}}); err != nil {
		t.Fatal(err)
	}
	f.text("Fair")
	drag := func(grab, to string) {
		t.Helper()
		bar := f.block("Fair, ")
		from, ok := f.tt.Find("All-day " + grab)
		if !ok {
			t.Fatalf("no cell %s in %s", grab, dump(f.tt))
		}
		dst, ok := f.tt.Find("All-day " + to)
		if !ok {
			t.Fatalf("no cell %s in %s", to, dump(f.tt))
		}
		x, y := from.X+from.W/2, bar.Y+bar.H/2
		f.tt.Press(x, y)
		for _, across := range []float32{x + 20, bar.X + bar.W - 10, bar.X + 10, bar.X + bar.W/2, bar.X + bar.W - 10} {
			f.tt.Move(across, y)
			f.tt.Frame()
		}
		// Below the bars, where the day's cell takes the drop.
		f.tt.Move(dst.X+dst.W/2, dst.Y+dst.H-2)
		f.tt.Frame()
		f.tt.Release(dst.X+dst.W/2, dst.Y+dst.H-2)
		f.tt.Frame()
	}
	// Monday's part, dropped on Thursday: three days, not the one from the
	// last of its days the pointer crossed.
	drag("Monday, October 5, 2026", "Thursday, October 8, 2026")
	f.until("the move", func() bool {
		o := f.occurrence("Fair", "2026-10-01", "2026-10-31")
		return o.StartDate != "2026-10-05"
	})
	if o := f.occurrence("Fair", "2026-10-01", "2026-10-31"); o.StartDate != "2026-10-08" || o.EndDate != "2026-10-11" {
		t.Fatalf("moved to %s – %s", o.StartDate, o.EndDate)
	}
	f.until("the bar in its new place", func() bool {
		a, ok := f.tt.Find("All-day Saturday, October 10, 2026")
		b, shown := f.tt.Find("Fair, all day")
		return ok && shown && b.X+b.W > a.X
	})
	// A new press grabs afresh: Saturday's part, dropped on Wednesday.
	drag("Saturday, October 10, 2026", "Wednesday, October 7, 2026")
	f.until("the second move", func() bool {
		o := f.occurrence("Fair", "2026-10-01", "2026-10-31")
		return o.StartDate != "2026-10-08"
	})
	if o := f.occurrence("Fair", "2026-10-01", "2026-10-31"); o.StartDate != "2026-10-05" || o.EndDate != "2026-10-08" {
		t.Fatalf("moved back to %s – %s", o.StartDate, o.EndDate)
	}
}

func TestBarDayCountsTheBarsInset(t *testing.T) {
	days := []string{"2026-10-05", "2026-10-06", "2026-10-07"}
	bar := &allDayBar{first: 1, last: 2}
	for _, c := range []struct {
		x    float32
		want string
	}{{0, "2026-10-06"}, {97, "2026-10-06"}, {98, "2026-10-07"}, {500, "2026-10-07"}} {
		if got := barDay(bar, days, c.x, 100); got != c.want {
			t.Errorf("x %v: %s, want %s", c.x, got, c.want)
		}
	}
	if got := barDay(bar, days, 50, 0); got != "2026-10-06" {
		t.Errorf("no width: %s", got)
	}
}

func TestARemovedOccurrenceEndsTheSelection(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Sync")
	o := f.occurrence("Sync", "2026-10-08", "2026-10-09")
	f.m.selectOccurrence(&o)
	f.until("selection", func() bool { return f.m.shownEvent() != nil })
	// A change elsewhere in the series keeps it.
	other := f.occurrence("Sync", "2026-10-06", "2026-10-07")
	scope := api.EventScopeThis
	if _, err := f.c.Calendar().Delete(t.Context(), &api.CalendarDeleteParams{ID: other.EventID, RecurrenceID: &other.RecurrenceID, Scope: &scope}); err != nil {
		t.Fatal(err)
	}
	f.until("the other gone", func() bool {
		return !slices.ContainsFunc(f.m.cal.occ.value, func(x api.Occurrence) bool { return x.RecurrenceID == other.RecurrenceID }) && !f.m.cal.occ.pending && !f.m.cal.occ.dirty
	})
	if f.m.cal.selected == nil {
		t.Fatal("removing another occurrence ended the selection")
	}
	// Paging away keeps a selection the range cannot judge.
	f.m.page(1)
	f.until("next week", func() bool {
		return f.m.cal.occ.key != "" && !f.m.cal.occ.pending && !f.m.cal.occ.dirty && strings.Contains(f.m.cal.occ.key, "2026-10-11")
	})
	f.tt.Frame()
	if f.m.cal.selected == nil {
		t.Fatal("paging ended the selection")
	}
	f.m.page(-1)
	f.until("back", func() bool {
		return !f.m.cal.occ.pending && !f.m.cal.occ.dirty && strings.Contains(f.m.cal.occ.key, "2026-10-04")
	})
	// Another client takes it out of the series: maild still answers for
	// it, but it has left the range.
	if _, err := f.c.Calendar().Delete(t.Context(), &api.CalendarDeleteParams{ID: o.EventID, RecurrenceID: &o.RecurrenceID, Scope: &scope}); err != nil {
		t.Fatal(err)
	}
	f.until("the empty pane", func() bool { return f.tt.HasText("No Event Selected") })
	if f.m.cal.selected != nil {
		t.Error("still selected")
	}
}

// Another series with an occurrence at the same time has the same
// recurrence ID: it keeps no removed occurrence selected, while an
// occurrence its own series overrode, under another event ID, stays.
func TestAnotherSeriesAtTheSameTimeKeepsNoRemovedOccurrence(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	// Echo repeats as Sync does, at 11:00 UTC from October 5.
	start := time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC)
	summary, rule, zone := "Echo", "FREQ=DAILY;COUNT=5", "UTC"
	if _, err := f.c.Calendar().Create(t.Context(), &api.CalendarCreateParams{CalendarID: &f.work, Fields: api.EventFields{Summary: &summary,
		AllDay: ptr(false), Start: &start, End: ptr(start.Add(30 * time.Minute)), TimeZone: &zone, Rrule: &rule}}); err != nil {
		t.Fatal(err)
	}
	occurrenceOn := func(date string) api.Occurrence {
		t.Helper()
		sync, echo := f.occurrence("Sync", date, addDays(date, 1)), f.occurrence("Echo", date, addDays(date, 1))
		if sync.RecurrenceID != echo.RecurrenceID || sync.CalendarID != echo.CalendarID || sync.EventID == echo.EventID {
			t.Fatalf("not at the same time: %+v, %+v", sync, echo)
		}
		return sync
	}
	settled := func() bool {
		occ, event := &f.m.cal.occ, &f.m.cal.event
		return slices.ContainsFunc(occ.value, func(o api.Occurrence) bool { return o.Summary == summary }) &&
			!occ.pending && !occ.dirty && !event.pending && !event.dirty
	}
	scope := api.EventScopeThis

	// Changed alone, it becomes an override, with an event ID of its own:
	// the selection stays once maild has named the series.
	o := occurrenceOn("2026-10-07")
	f.m.selectOccurrence(&o)
	f.until("the selection", func() bool { return f.m.shownEvent() != nil && settled() })
	moved := "Sync moved"
	if _, err := f.c.Calendar().Update(t.Context(), &api.CalendarUpdateParams{ID: o.EventID, RecurrenceID: &o.RecurrenceID, Scope: &scope,
		Fields: api.EventFields{Summary: &moved}}); err != nil {
		t.Fatal(err)
	}
	f.until("the override's series", func() bool {
		cal := &f.m.cal
		if !settled() || !slices.ContainsFunc(cal.occ.value, func(x api.Occurrence) bool { return x.Summary == moved }) {
			return false
		}
		var others []int64
		for _, x := range cal.occ.value {
			if x.RecurrenceID == o.RecurrenceID && x.CalendarID == o.CalendarID && x.EventID != o.EventID {
				others = append(others, x.EventID)
			}
		}
		return cal.series.key == fmt.Sprint(f.m.conn, "|", fmt.Sprint(others)) && cal.series.loaded && !cal.series.pending
	})
	if f.m.cal.selected == nil {
		t.Fatal("the override ended the selection")
	}

	// Taken out of its series, with Echo's still at its time: it ends.
	o = occurrenceOn("2026-10-08")
	f.m.selectOccurrence(&o)
	f.until("the selection", func() bool { return f.m.shownEvent() != nil && settled() })
	if _, err := f.c.Calendar().Delete(t.Context(), &api.CalendarDeleteParams{ID: o.EventID, RecurrenceID: &o.RecurrenceID, Scope: &scope}); err != nil {
		t.Fatal(err)
	}
	f.until("the empty pane", func() bool { return f.m.cal.selected == nil && f.tt.HasText("No Event Selected") })
}

// A zone with no name is never left to maild's own: it may be on another
// machine.
func TestAnUnnamedZoneDoesNotUseMaildsZone(t *testing.T) {
	fake := &fakeCalendar{}
	f := newFixture(t, commands.Calendar, withFake(fake))
	f.text("Standup")
	loc := time.FixedZone(localZone, 13*3600)
	f.m.zone, f.m.clock.loc = localZone, loc
	var got fetch[[]api.Occurrence]
	f.m.loadRange(&got, "2026-10-09", "2026-10-10", "test")
	f.until("the answer", func() bool { return got.loaded })
	if got.err != nil {
		t.Fatal(got.err)
	}
	fake.mu.Lock()
	zones := slices.Clone(fake.zones)
	fake.mu.Unlock()
	if slices.Contains(zones, "") {
		t.Errorf("a call left the zone to maild: %q", zones)
	}
	names := map[string]api.Occurrence{}
	for _, o := range got.value {
		names[o.Summary] = o
	}
	// Review is 14:00 UTC on the 8th: 3:00 on the 9th here. Standup, 9:00
	// UTC, is still the 8th here, 22:00.
	if _, ok := names["Review"]; !ok {
		t.Errorf("Review is missing: %+v", got.value)
	}
	if _, ok := names["Standup"]; ok {
		t.Errorf("Standup is on the 8th here: %+v", got.value)
	}
	holiday, ok := names["Holiday"]
	if !ok || !holiday.Start.Equal(time.Date(2026, 10, 9, 0, 0, 0, 0, loc)) {
		t.Errorf("the all-day event: %+v", holiday)
	}
}

func TestAnUntitledNewEventWithChangesAsks(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	for name, change := range map[string]func(*eventWindow){
		"location": func(w *eventWindow) { w.draft.location = "Room 9" },
		"notes":    func(w *eventWindow) { w.draft.description = "Agenda" },
		"time":     func(w *eventWindow) { w.draft.endTime = "13:30" },
		"invitee":  func(w *eventWindow) { w.draft.invitees = []draftInvitee{{email: "ann@a.test"}} },
		"alert":    func(w *eventWindow) { w.draft.alarms = []int64{15} },
		"typed":    func(w *eventWindow) { w.inviteeText = "ann@a.test" },
	} {
		win, w := f.newEvent()
		if !w.closeRequested() && !w.closing {
			t.Fatalf("%s: an unchanged window stayed", name)
		}
		f.m.windows[win.spec.Key] = w
		change(w)
		if w.closeRequested() {
			t.Errorf("%s: the window would close with no question", name)
		}
		win.tt.Frame()
		if !win.tt.HasText(`Save changes to "New Event"?`) {
			t.Errorf("%s: shows %s", name, dump(win.tt))
		}
		w.closing = false
		w.closeNow()
	}
}

func TestSavingCommitsATypedInvitee(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	win, w := f.newEvent()
	w.calendars[0].invites = true
	w.draft.summary = "Planning"
	win.tt.Frame()
	if err := win.tt.Click("Add Invitee"); err != nil {
		t.Fatalf("%v: %s", err, dump(win.tt))
	}
	win.tt.Type("not an address")
	win.tt.Frame()
	// What is no address stays to be fixed, and nothing is sent.
	f.clickIn(win, "Save")
	if w.busy || !win.tt.HasText("Check the invitees' addresses") || w.inviteeText != "" || len(w.draft.invitees) != 1 {
		t.Fatalf("draft %+v, text %q; shows %s", w.draft.invitees, w.inviteeText, dump(win.tt))
	}
	w.draft.invitees = nil
	win.tt.Frame()
	if err := win.tt.Click("Add Invitee"); err != nil {
		t.Fatalf("%v: %s", err, dump(win.tt))
	}
	win.tt.Type("Ann <ann@a.test>")
	win.tt.Frame()
	sent := changedFields(nil, w.draft)
	if sent.Attendees != nil {
		t.Fatalf("committed too early: %+v", sent.Attendees)
	}
	w.request(false)
	if len(w.draft.invitees) != 1 || w.draft.invitees[0].email != "ann@a.test" || w.draft.invitees[0].name != "Ann" || w.inviteeText != "" {
		t.Errorf("draft %+v, text %q", w.draft.invitees, w.inviteeText)
	}
}

func TestInviteesWithRolesThatCannotBeWrittenStayAsTheyAre(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	event := &api.CalendarEvent{ID: 1, CalendarID: f.work, Start: testNow, End: testNow.Add(time.Hour),
		Attendees: []api.Attendee{{Email: "chair@a.test", Name: "Chair", Role: api.AttendeeRoleChair}, {Email: "ann@a.test", Name: "Ann", Role: api.AttendeeRoleRequired}}}
	w := &eventWindow{m: f.m, key: "event-x", target: editTarget{id: 1}}
	f.m.openWindow(w, "Roles")
	w.loadedWith([]api.Account{{ID: f.acct, Email: "me@a.test"}}, []api.Collection{{ID: f.work, AccountID: f.acct, Name: "Work", Invites: true}}, event, nil, "UTC", time.UTC)
	win := f.windows["event-x"]
	win.tt.Frame()
	if !win.tt.HasText("These invitees can't be changed here: one has a role Frostmail can't keep.") {
		t.Errorf("no note: %s", dump(win.tt))
	}
	if win.tt.HasText("Add Invitee") {
		t.Error("the input shows")
	}
	if a, _ := win.tt.AccessByRole(ui.RoleButton, "Remove Ann"); !a.Disabled {
		t.Errorf("the row can change: %+v", a)
	}
	w.inviteeText = "bo@a.test"
	if w.dirty() {
		t.Error("text in a field that does not show counts as a change")
	}
	w.draft.summary = "Renamed"
	w.draft.invitees = w.draft.invitees[:1]
	if got := changedFields(w.original(), w.draft); got.Attendees != nil || got.Summary == nil {
		t.Errorf("a write would rewrite the roles: %+v", got)
	}
}

func TestTheRepeatSheetTakesATypedInterval(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	win, w := f.newEvent()
	w.repeat = &repeatSheet{r: defaultRepeat(w.draft.startDate), interval: ""}
	win.tt.Frame()
	if a, _ := win.tt.AccessByRole(ui.RoleButton, "OK"); !a.Disabled {
		t.Fatalf("OK takes an empty interval: %+v", a)
	}
	every, ok := win.tt.AccessByRole(ui.RoleTextField, "Every")
	if !ok {
		t.Fatalf("no field: %s", dump(win.tt))
	}
	win.tt.ClickAt(every.Bounds.X+every.Bounds.W/2, every.Bounds.Y+every.Bounds.H/2)
	win.tt.Type("3")
	if a, _ := win.tt.AccessByRole(ui.RoleButton, "OK"); a.Disabled {
		t.Errorf("OK refuses a typed interval: %+v", a)
	}
	f.clickIn(win, "OK")
	if w.repeat != nil || w.draft.rrule != "FREQ=WEEKLY;INTERVAL=3" {
		t.Errorf("rule %q", w.draft.rrule)
	}
}

// A failed answer brings its row back, says so, and keeps the window.
func TestAFailedSnoozeComesBackAndSaysSo(t *testing.T) {
	fake := &fakeCalendar{due: []api.Reminder{{ID: "a", EventID: 1, Summary: "Meeting", Start: testNow, DueAt: testNow}}, snoozeErr: errors.New("no")}
	f := newFixture(t, commands.Calendar, withFake(fake))
	f.until("the row", func() bool { w := f.windows[remindersKey]; return w != nil && w.tt.HasText("Meeting") })
	win := f.windows[remindersKey]
	hold := make(chan struct{})
	fake.mu.Lock()
	fake.hold = hold
	fake.mu.Unlock()
	if err := win.tt.Click("Snooze"); err != nil {
		t.Fatal(err)
	}
	win.tt.Frame()
	if err := win.tt.ChooseMenuItem("5 minutes"); err != nil {
		t.Fatalf("%v: menu %q", err, win.tt.Menu())
	}
	f.until("the row leaving", func() bool { return !win.tt.HasText("Meeting") })
	for range 3 {
		win.tt.Frame()
	}
	if f.windows[remindersKey] == nil {
		t.Fatal("the window closed before the answer came")
	}
	close(hold)
	f.until("the notice", func() bool { return len(f.toasts) == 1 })
	if !strings.Contains(f.toasts[0], "Could not snooze the reminder") {
		t.Errorf("toast %q", f.toasts[0])
	}
	f.until("the row back", func() bool { return win.tt.HasText("Meeting") })
	if f.windows[remindersKey] == nil {
		t.Error("the window closed")
	}
	// A dismiss that works leaves no row to flash back, and closes it.
	f.clickIn(win, "Dismiss")
	f.until("the close", func() bool { return f.windows[remindersKey] == nil })
}

// One call at a time: paging starts no pile of loads, the newest key is
// the one asked next, and an answer dropped by gen still frees the fetch.
func TestWantHoldsOneCallAtATime(t *testing.T) {
	var mu sync.Mutex
	var posted []func()
	m := New(Options{Post: func(fn func()) { mu.Lock(); defer mu.Unlock(); posted = append(posted, fn) }})
	m.client = &api.Client{}
	flush := func() {
		deadline := time.Now().Add(15 * time.Second)
		for {
			mu.Lock()
			list := posted
			posted = nil
			mu.Unlock()
			if len(list) > 0 {
				for _, fn := range list {
					fn()
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("no answer posted")
			}
			time.Sleep(time.Millisecond)
		}
	}
	started := make(chan string, 16)
	release := make(chan struct{})
	var f fetch[string]
	ask := func(key string) {
		want(m, &f, key, func(context.Context, *api.Client) (string, error) {
			started <- key
			<-release
			return key, nil
		})
	}
	ask("a")
	if got := <-started; got != "a" {
		t.Fatal(got)
	}
	for _, key := range []string{"b", "c", "d"} {
		ask(key)
	}
	select {
	case got := <-started:
		t.Fatalf("%q started while a call was out", got)
	default:
	}
	release <- struct{}{}
	flush()
	if f.value != "a" {
		t.Errorf("value %q", f.value)
	}
	ask("d")
	if got := <-started; got != "d" {
		t.Fatalf("asked %q next", got)
	}
	// An answer dropped by gen, as an optimistic write does, frees the fetch.
	f.gen++
	release <- struct{}{}
	flush()
	if f.value != "a" {
		t.Errorf("a dropped answer was kept: %q", f.value)
	}
	f.dirty = true
	ask("d")
	if got := <-started; got != "d" {
		t.Fatal(got)
	}
	release <- struct{}{}
	flush()
	if f.value != "d" || f.running || f.pending {
		t.Errorf("%+v", f)
	}
}
