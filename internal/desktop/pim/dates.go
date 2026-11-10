package pim

import (
	"fmt"
	"time"
)

// viewKind is the period a calendar view shows.
type viewKind string

const (
	viewDay   viewKind = "day"
	viewWeek  viewKind = "week"
	viewMonth viewKind = "month"
)

// clock is how the app reads and writes times: its zone, the week's first
// day (0 is Sunday) and the hour cycle (docs/specs/pim-ui.md, Calendar
// module: Dates).
type clock struct {
	loc       *time.Location
	weekStart int
	h24       bool
}

// parseDate reads a YYYY-MM-DD date as UTC midnight.
func parseDate(date string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, date)
	return t, err == nil
}

// dateOf is parseDate with the zero day for text that is not a date.
func dateOf(date string) time.Time {
	t, _ := parseDate(date)
	return t
}

// addDays moves a date by whole days.
func addDays(date string, n int) string {
	return dateOf(date).AddDate(0, 0, n).Format(time.DateOnly)
}

// weekday is the date's weekday, Sunday 0.
func weekday(date string) int { return int(dateOf(date).Weekday()) }

// startOfWeek is the first day of the week holding date.
func startOfWeek(date string, weekStart int) string {
	return addDays(date, -((weekday(date) - weekStart + 7) % 7))
}

// monthGrid is six weeks starting with the week holding the 1st.
func monthGrid(date string, weekStart int) []string {
	from := startOfWeek(date[:7]+"-01", weekStart)
	out := make([]string, 42)
	for i := range out {
		out[i] = addDays(from, i)
	}
	return out
}

// viewRange is the view's first day and the day after its last.
func viewRange(view viewKind, date string, weekStart int) (from, to string) {
	switch view {
	case viewDay:
		return date, addDays(date, 1)
	case viewWeek:
		from = startOfWeek(date, weekStart)
		return from, addDays(from, 7)
	}
	from = startOfWeek(date[:7]+"-01", weekStart)
	return from, addDays(from, 42)
}

// viewDays lists the days a view shows.
func viewDays(view viewKind, date string, weekStart int) []string {
	if view == viewMonth {
		return monthGrid(date, weekStart)
	}
	from, _ := viewRange(view, date, weekStart)
	n := 1
	if view == viewWeek {
		n = 7
	}
	out := make([]string, n)
	for i := range out {
		out[i] = addDays(from, i)
	}
	return out
}

// stepDate moves a date by a view's period, clamping the day across
// months.
func stepDate(view viewKind, date string, direction int) string {
	switch view {
	case viewDay:
		return addDays(date, direction)
	case viewWeek:
		return addDays(date, 7*direction)
	}
	d := dateOf(date)
	first := time.Date(d.Year(), d.Month()+time.Month(direction), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(d.Day(), last), 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

// longDate is "Thursday, October 8, 2026".
func longDate(date string) string { return dateOf(date).Format("Monday, January 2, 2006") }

// viewTitle titles a view: the day, the month, or the months a week
// spans.
func viewTitle(view viewKind, date string, weekStart int) string {
	switch view {
	case viewDay:
		return longDate(date)
	case viewMonth:
		return dateOf(date).Format("January 2006")
	}
	first := startOfWeek(date, weekStart)
	last := addDays(first, 6)
	if first[:7] == last[:7] {
		return dateOf(first).Format("January 2006")
	}
	layout := "Jan"
	if first[:4] != last[:4] {
		layout = "Jan 2006"
	}
	return dateOf(first).Format(layout) + " – " + dateOf(last).Format("Jan 2006")
}

// zoned reads an instant's date and wall-clock minutes in a zone.
func zoned(t time.Time, loc *time.Location) (date string, minutes int) {
	t = t.In(loc)
	return t.Format(time.DateOnly), t.Hour()*60 + t.Minute()
}

// todayIn is the date now in a zone.
func todayIn(now time.Time, loc *time.Location) string {
	return now.In(loc).Format(time.DateOnly)
}

// dayStart is the instant of a date's midnight in a zone.
func dayStart(date string, loc *time.Location) time.Time { return instantAt(date, 0, loc) }

// instantAt is the instant of a wall time, in minutes after midnight, on
// a date in a zone.
func instantAt(date string, minutes int, loc *time.Location) time.Time {
	d := dateOf(date)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, minutes, 0, 0, loc)
}

// daysBetween is the whole days from one date to another.
func daysBetween(from, to string) int {
	return int(dateOf(to).Sub(dateOf(from)) / (24 * time.Hour))
}

// wallTime writes minutes after midnight as HH:MM.
func wallTime(minutes int) string { return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60) }

// minutesOf reads HH:MM as minutes after midnight.
func minutesOf(wall string) int {
	var h, m int
	if _, err := fmt.Sscanf(wall, "%d:%d", &h, &m); err != nil {
		return 0
	}
	return h*60 + m
}
