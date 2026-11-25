//go:build linux && integration

package main

import (
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// TestShiftedDaysKeepsTheTimes checks the arithmetic of a move by whole
// days: an all-day occurrence moves by its dates, a timed one keeps its
// wall time across a change of the clocks and its length, and no move
// leaves the instants as they are.
func TestShiftedDaysKeepsTheTimes(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	allDay := shiftedDays(api.Occurrence{AllDay: true, StartDate: "2026-11-27", EndDate: "2026-11-29"}, 3, loc, "")
	if !*allDay.AllDay || *allDay.StartDate != "2026-11-30" || *allDay.EndDate != "2026-12-02" || allDay.Start != nil {
		t.Errorf("an all-day occurrence moved 3 days is %v to %v, all-day %v", *allDay.StartDate, *allDay.EndDate, *allDay.AllDay)
	}
	// 9:30 to 10:15 on the Saturday before the clocks go back, moved to the
	// Monday after: still 9:30 on the wall, which is another offset.
	start := time.Date(2026, 10, 31, 9, 30, 0, 0, loc)
	timed := api.Occurrence{Start: start, End: start.Add(45 * time.Minute)}
	moved := shiftedDays(timed, 2, loc, "America/New_York")
	if got := moved.Start.In(loc); got.Format("2006-01-02 15:04") != "2026-11-02 09:30" {
		t.Errorf("the start is %v on the wall, want 2026-11-02 09:30", got.Format("2006-01-02 15:04"))
	}
	if length := moved.End.Sub(*moved.Start); length != 45*time.Minute {
		t.Errorf("the occurrence is %v long after the move, want 45m", length)
	}
	if *moved.AllDay || *moved.TimeZone != "America/New_York" {
		t.Errorf("the move writes all-day %v in zone %q", *moved.AllDay, *moved.TimeZone)
	}
	if zone := *shiftedDays(timed, 1, loc, "").TimeZone; zone != "UTC" {
		t.Errorf("an event with no zone is written in %q, want UTC", zone)
	}
	same := shiftedDays(timed, 0, loc, "America/New_York")
	if !same.Start.Equal(timed.Start) || !same.End.Equal(timed.End) {
		t.Errorf("no move changed the instants to %v and %v", same.Start, same.End)
	}
	if got := daysBetween("2026-11-30", "2026-11-27"); got != -3 {
		t.Errorf("from the 30th to the 27th is %d days, want -3", got)
	}
}

// TestOnlyOwnWritableOccurrencesMove checks who may be dragged: an
// occurrence of a writable calendar that is no invitation.
func TestOnlyOwnWritableOccurrencesMove(t *testing.T) {
	writable := map[int64]bool{4: true}
	accepted := api.PartStat("accepted")
	for _, tc := range []struct {
		name string
		o    api.Occurrence
		want bool
	}{
		{"own, writable", api.Occurrence{CalendarID: 4}, true},
		{"a read-only calendar's", api.Occurrence{CalendarID: 5}, false},
		{"an invitation", api.Occurrence{CalendarID: 4, Answer: &accepted}, false},
	} {
		if got := canMove(tc.o, writable); got != tc.want {
			t.Errorf("%s: may move = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestDragMovesAnEventToAnotherDay checks the drag in the window: in the
// week, an event dragged to another day's cell is in that cell after, and
// no longer in its own.
func TestDragMovesAnEventToAnotherDay(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Calendar")
	a.Press("label:New event")
	const title = "A lunch to move"
	a.Type("label:Title", title)
	a.Press("label:Save")
	a.WaitFor("label:Undo calendar change", 5*time.Second)
	// New event starts an hour from now: today, or tomorrow in the day's
	// last hour.
	from := time.Now().Add(time.Hour)
	// Another day of the same week, which starts on Monday.
	to := from.AddDate(0, 0, 1)
	if from.Weekday() == time.Sunday {
		to = from.AddDate(0, 0, -1)
	}
	cell := func(day time.Time) string { return "id:day-" + day.Format(time.DateOnly) }
	// The week shows every event of a day; the month shows 3 and counts
	// the rest.
	a.Press("role:tab Week")
	a.WaitFor(cell(from)+" >> label:"+title, 5*time.Second)
	a.DragTo("label:Drag "+title, cell(to))
	a.WaitFor(cell(to)+" >> label:"+title, 8*time.Second)
	a.WaitForGone(cell(from)+" >> label:"+title, 5*time.Second)
}
