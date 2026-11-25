//go:build linux && integration

package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/nsgo-dev/nsgo/core"
)

// TestTimeGridArithmetic checks the steps of a drag on the day laid out
// by time: 15 minutes for 12 points, a day for a column, and a range that
// keeps its length when moved and its start when resized.
func TestTimeGridArithmetic(t *testing.T) {
	for dy, want := range map[core.Points]int{0: 0, 5: 0, 7: 15, 12: 15, 24: 30, -12: -15, 48: 60} {
		if got := stepsOf(dy); got != want {
			t.Errorf("a move of %v points is %d minutes, want %d", dy, got, want)
		}
	}
	if got := daysOf(130, 100); got != 1 {
		t.Errorf("a move of 130 across columns 100 wide is %d days, want 1", got)
	}
	if got := daysOf(40, 0); got != 0 {
		t.Errorf("a move across a column of no width is %d days, want 0", got)
	}
	for y, want := range map[core.Points]int{0: 0, 11: 0, 12: 15, 576: 720, 5000: dayMinutes - dragStep, -9: 0} {
		if got := minuteAt(y); got != want {
			t.Errorf("%v points down a day is minute %d, want %d", y, got, want)
		}
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 31, 9, 30, 0, 0, loc)
	end := start.Add(45 * time.Minute)
	from, to := movedBy(start, end, 2, 30, loc)
	if from.Format("2006-01-02 15:04") != "2026-11-02 10:00" || to.Sub(from) != 45*time.Minute {
		t.Errorf("moved 2 days and 30 minutes: %v, %v long", from.Format("2006-01-02 15:04"), to.Sub(from))
	}
	if from, to := movedBy(start, end, 0, 0, loc); !from.Equal(start) || !to.Equal(end) {
		t.Error("no move changed the instants")
	}
	if _, to := resizedBy(start, end, 30, loc); to.Sub(start) != 75*time.Minute {
		t.Errorf("resized by 30 minutes the range is %v long, want 75m", to.Sub(start))
	}
	if _, to := resizedBy(start, end, -120, loc); to.Sub(start) != dragStep*time.Minute {
		t.Errorf("resized to before its start the range is %v long, want a step", to.Sub(start))
	}
	day := time.Date(2026, 11, 27, 0, 0, 0, 0, loc)
	if from, to := drawnOn(day, 780, 720); from.Format("15:04") != "12:00" || to.Format("15:04") != "13:15" {
		t.Errorf("drawn from 13:00 up to 12:00: %v to %v, want 12:00 to 13:15", from.Format("15:04"), to.Format("15:04"))
	}
}

// TestBlocksThatOverlapShareTheirWidth checks the day's layout: a block
// from its start to its end, at least a step high, cut at the day's ends,
// and blocks that overlap side by side.
func TestBlocksThatOverlapShareTheirWidth(t *testing.T) {
	day := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	at := func(h, m int) time.Time { return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }
	rows := []api.Occurrence{
		{EventID: 1, Start: at(9, 0), End: at(10, 0)},
		{EventID: 2, Start: at(9, 30), End: at(11, 0)},
		{EventID: 3, Start: at(10, 0), End: at(10, 30)},
		{EventID: 4, Start: at(13, 0), End: at(13, 5)},
		{EventID: 5, Start: at(-2, 0), End: at(1, 0)},
		{EventID: 6, AllDay: true, StartDate: "2026-11-27", EndDate: "2026-11-28"},
		{EventID: 7, Start: at(30, 0), End: at(31, 0)},
	}
	got := map[int64]placed{}
	for _, p := range place(rows, day) {
		got[p.occurrence.EventID] = p
	}
	if len(got) != 5 {
		t.Fatalf("the day places %d blocks, want 5: the all-day one and the next day's are not its", len(got))
	}
	for id, want := range map[int64]placed{
		1: {from: 540, to: 600, lane: 0, lanes: 2},
		2: {from: 570, to: 660, lane: 1, lanes: 2},
		3: {from: 600, to: 630, lane: 0, lanes: 2},
		4: {from: 780, to: 795, lane: 0, lanes: 1},
		5: {from: 0, to: 60, lane: 0, lanes: 1},
	} {
		p := got[id]
		if p.from != want.from || p.to != want.to || p.lane != want.lane || p.lanes != want.lanes {
			t.Errorf("event %d is placed %d to %d in lane %d of %d, want %d to %d in lane %d of %d", id, p.from, p.to, p.lane, p.lanes, want.from, want.to, want.lane, want.lanes)
		}
	}
}

// TestDragsOnTheDayLaidOutByTime checks the drags in the window, on a day
// with nothing in it: a drag on empty time draws an event, a drag of its
// block moves it by 15 minutes a step, a drag of its foot resizes it, and
// in the week a drag across moves it to another day.
func TestDragsOnTheDayLaidOutByTime(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Calendar")
	a.Press("role:tab Day")
	// A day 80 days on, which the showcase leaves empty.
	const ahead = 80
	for range ahead {
		a.Press("label:Next")
	}
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day()+ahead, 0, 0, 0, 0, time.Local)
	date := day.Format(time.DateOnly)
	a.WaitFor("id:time-"+date, 8*time.Second)
	const title = "Drawn by a drag"
	block := "id:time-grid >> label:" + title
	// span reads the block's times, in minutes of its day, from its text.
	span := func() (from, to int) {
		t.Helper()
		var fh, fm, th, tm int
		text := a.Find("id:time-grid >> text:" + title).Text
		if _, err := fmt.Sscanf(text, "%d:%d – %d:%d", &fh, &fm, &th, &tm); err != nil {
			t.Fatalf("the block's text %q has no times: %v", text, err)
		}
		return fh*60 + fm, th*60 + tm
	}
	// shows waits for the block to show a range. A write's own reload
	// builds the grid again, and a drag begun before it loses the block it
	// holds, so each step also waits for the grid to rest.
	shows := func(from, to int) {
		t.Helper()
		a.WaitFor("id:time-grid >> text:"+timeRange(from, to)+"  "+title, 8*time.Second)
		a.Advance(2 * time.Second)
		a.Settle()
	}
	// A drag of 48 points down the day's empty time, from the middle of
	// what shows of it: 3 or 4 steps begun, by where in a step it starts,
	// and so 60 or 75 minutes. Where the middle is differs between a
	// headless run and a window, so the test reads the range it drew.
	a.Drag("id:time-"+date, core.Point{Y: 48})
	a.WaitFor("label:Title", 5*time.Second)
	a.Type("label:Title", title)
	a.Press("label:Save")
	a.WaitFor(block, 8*time.Second)
	a.Advance(2 * time.Second)
	a.Settle()
	from, to := span()
	if length := to - from; length != 60 && length != 75 {
		t.Fatalf("a drag of 48 points drew %d minutes, from %s: want 60 or 75", length, timeRange(from, to))
	}
	if from%dragStep != 0 || to%dragStep != 0 {
		t.Errorf("the drawn range %s is not on steps of %d minutes", timeRange(from, to), dragStep)
	}
	if height, want := a.Find(block).Bounds().Size.Height, core.Points(float64(to-from)*minutePoints); height != want {
		t.Errorf("a block of %d minutes is %v high, want %v", to-from, height, want)
	}
	// 24 points down are 30 minutes, and 12 at its foot are 15 more.
	a.Drag(block, core.Point{Y: 24})
	shows(from+30, to+30)
	a.Drag("label:Resize "+title, core.Point{Y: 12})
	shows(from+30, to+45)
	// In the week, a drag by a column's width moves it a day: to the next
	// day, or back from the week's last.
	a.Press("role:tab Week")
	a.WaitFor(block, 8*time.Second)
	width := a.Find("id:column-" + date).Bounds().Size.Width
	other, dx := day.AddDate(0, 0, 1), width
	if day.Weekday() == time.Sunday {
		other, dx = day.AddDate(0, 0, -1), -width
	}
	a.Drag(block, core.Point{X: dx})
	a.WaitFor("id:column-"+other.Format(time.DateOnly)+" >> label:"+title, 8*time.Second)
	a.WaitForGone("id:column-"+date+" >> label:"+title, 5*time.Second)
	shows(from+30, to+45)
}

// TestPicturesOfTheCalendarByTime names pictures of the week and the day
// laid out by time, on the showcase's events.
func TestPicturesOfTheCalendarByTime(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Calendar")
	a.Press("role:tab Week")
	a.WaitFor("id:time-grid", 8*time.Second)
	a.Advance(time.Second)
	a.Settle()
	a.Screenshot("calendar-week")
	a.Press("role:tab Day")
	a.WaitFor("id:time-grid", 8*time.Second)
	a.Advance(time.Second)
	a.Settle()
	a.Screenshot("calendar-day")
}
