package pim

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
