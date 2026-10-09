package calendar

// CONTRACT TEST for task card T-0068 (docs/tasks). Do not edit.

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func events(t *testing.T, opts Options, lines ...string) []Event {
	t.Helper()
	evs, err := Parse(ics(lines...), opts)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func expand(t *testing.T, evs []Event, from, to string) []Occurrence {
	t.Helper()
	got, err := Expand(evs, utc(from), utc(to))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// show writes occurrences as "event recurrence-id start–end" lines.
func show(occ []Occurrence) string {
	var b strings.Builder
	for _, o := range occ {
		day := ""
		if o.AllDay {
			day = " all-day"
		}
		fmt.Fprintf(&b, "%d %s %s–%s%s\n", o.Event, o.RecurrenceID,
			o.Start.UTC().Format("01-02T15:04"), o.End.UTC().Format("01-02T15:04"), day)
	}
	return b.String()
}

func want(t *testing.T, got []Occurrence, lines ...string) {
	t.Helper()
	w := strings.Join(lines, "\n")
	if len(lines) > 0 {
		w += "\n"
	}
	if s := show(got); s != w {
		t.Errorf("occurrences:\n%s\nwant:\n%s", s, w)
	}
}

func TestSingleEvents(t *testing.T) {
	evs := events(t, me,
		"BEGIN:VEVENT", "UID:a", "DTSTART:20261009T140000Z", "DTEND:20261009T150000Z", "END:VEVENT")
	want(t, expand(t, evs, "2026-10-09T00:00Z", "2026-10-10T00:00Z"), "0  10-09T14:00–10-09T15:00")
	want(t, expand(t, evs, "2026-10-09T15:00Z", "2026-10-10T00:00Z")) // ends as the range starts
	want(t, expand(t, evs, "2026-10-09T14:30Z", "2026-10-09T14:45Z"), "0  10-09T14:00–10-09T15:00")

	zero := events(t, me, "BEGIN:VEVENT", "UID:z", "DTSTART:20261009T140000Z", "END:VEVENT")
	want(t, expand(t, zero, "2026-10-09T14:00Z", "2026-10-09T15:00Z"), "0  10-09T14:00–10-09T14:00")
	want(t, expand(t, zero, "2026-10-09T13:00Z", "2026-10-09T14:00Z"))

	trip := events(t, me, "BEGIN:VEVENT", "UID:t", "DTSTART;VALUE=DATE:20261008", "DTEND;VALUE=DATE:20261012", "END:VEVENT")
	want(t, expand(t, trip, "2026-10-10T00:00Z", "2026-10-11T00:00Z"), "0  10-08T00:00–10-12T00:00 all-day")
}

// A weekly 9:00 meeting in New York stays at 9:00 when DST ends (Nov 1).
func TestSeriesAcrossDST(t *testing.T) {
	evs := events(t, me, "BEGIN:VEVENT", "UID:w", "RRULE:FREQ=WEEKLY;COUNT=4",
		"DTSTART;TZID=America/New_York:20261022T090000", "DTEND;TZID=America/New_York:20261022T100000", "END:VEVENT")
	want(t, expand(t, evs, "2026-10-01T00:00Z", "2027-01-01T00:00Z"),
		"0 2026-10-22T13:00:00.000Z 10-22T13:00–10-22T14:00",
		"0 2026-10-29T13:00:00.000Z 10-29T13:00–10-29T14:00",
		"0 2026-11-05T14:00:00.000Z 11-05T14:00–11-05T15:00",
		"0 2026-11-12T14:00:00.000Z 11-12T14:00–11-12T15:00")

	berlin, _ := time.LoadLocation("Europe/Berlin")
	floating := events(t, Options{Local: berlin}, "BEGIN:VEVENT", "UID:f", "RRULE:FREQ=WEEKLY;COUNT=2",
		"DTSTART:20261022T090000", "DTEND:20261022T093000", "END:VEVENT")
	want(t, expand(t, floating, "2026-10-01T00:00Z", "2026-11-01T00:00Z"),
		"0 2026-10-22T07:00:00.000Z 10-22T07:00–10-22T07:30",
		"0 2026-10-29T08:00:00.000Z 10-29T08:00–10-29T08:30")
}

// EXDATE removes an occurrence; overrides replace theirs, wherever they move.
func TestExceptionsAndOverrides(t *testing.T) {
	evs := events(t, me,
		"BEGIN:VEVENT", "UID:s", "RRULE:FREQ=DAILY;COUNT=6",
		"EXDATE;TZID=America/New_York:20261027T090000",
		"DTSTART;TZID=America/New_York:20261026T090000", "DTEND;TZID=America/New_York:20261026T091500", "END:VEVENT",
		"BEGIN:VEVENT", "UID:s", "RECURRENCE-ID;TZID=America/New_York:20261028T090000",
		"DTSTART;TZID=America/New_York:20261028T100000", "DTEND;TZID=America/New_York:20261028T103000", "END:VEVENT",
		"BEGIN:VEVENT", "UID:s", "RECURRENCE-ID;TZID=America/New_York:20261030T090000", "STATUS:CANCELLED",
		"DTSTART;TZID=America/New_York:20261102T090000", "DTEND;TZID=America/New_York:20261102T091500", "END:VEVENT",
	)
	want(t, expand(t, evs, "2026-10-01T00:00Z", "2026-12-01T00:00Z"),
		"0 2026-10-26T13:00:00.000Z 10-26T13:00–10-26T13:15",
		"1 2026-10-28T13:00:00.000Z 10-28T14:00–10-28T14:30",
		"0 2026-10-29T13:00:00.000Z 10-29T13:00–10-29T13:15",
		"0 2026-10-31T13:00:00.000Z 10-31T13:00–10-31T13:15",
		"2 2026-10-30T13:00:00.000Z 11-02T14:00–11-02T14:15")
	// An occurrence moved into the range shows though its original start
	// is outside it; one moved out of the range does not.
	want(t, expand(t, evs, "2026-11-02T00:00Z", "2026-11-03T00:00Z"),
		"2 2026-10-30T13:00:00.000Z 11-02T14:00–11-02T14:15")
	want(t, expand(t, evs, "2026-10-30T00:00Z", "2026-10-31T00:00Z"))
}

func TestRDates(t *testing.T) {
	evs := events(t, me, "BEGIN:VEVENT", "UID:r", "RRULE:FREQ=WEEKLY;COUNT=2",
		"RDATE;TZID=America/New_York:20261105T150000,20261112T150000",
		"DTSTART;TZID=America/New_York:20261022T090000", "DTEND;TZID=America/New_York:20261022T100000", "END:VEVENT")
	want(t, expand(t, evs, "2026-10-01T00:00Z", "2027-01-01T00:00Z"),
		"0 2026-10-22T13:00:00.000Z 10-22T13:00–10-22T14:00",
		"0 2026-10-29T13:00:00.000Z 10-29T13:00–10-29T14:00",
		"0 2026-11-05T20:00:00.000Z 11-05T20:00–11-05T21:00",
		"0 2026-11-12T20:00:00.000Z 11-12T20:00–11-12T21:00")
	only := events(t, me, "BEGIN:VEVENT", "UID:o", "RDATE;VALUE=DATE:20261224,20261231",
		"DTSTART;VALUE=DATE:20261217", "END:VEVENT")
	want(t, expand(t, only, "2026-12-01T00:00Z", "2027-01-01T00:00Z"),
		"0 2026-12-17 12-17T00:00–12-18T00:00 all-day",
		"0 2026-12-24 12-24T00:00–12-25T00:00 all-day",
		"0 2026-12-31 12-31T00:00–01-01T00:00 all-day")
}

func TestAllDaySeries(t *testing.T) {
	birthday := events(t, me, "BEGIN:VEVENT", "UID:b", "SUMMARY:Ada's birthday", "RRULE:FREQ=YEARLY",
		"DTSTART;VALUE=DATE:19800412", "DTEND;VALUE=DATE:19800413", "END:VEVENT")
	want(t, expand(t, birthday, "2026-01-01T00:00Z", "2027-01-01T00:00Z"),
		"0 2026-04-12 04-12T00:00–04-13T00:00 all-day")
	monthly := events(t, me, "BEGIN:VEVENT", "UID:m", "RRULE:FREQ=MONTHLY;BYDAY=2TU;UNTIL=20261231",
		"DTSTART;VALUE=DATE:20261013", "END:VEVENT",
		"BEGIN:VEVENT", "UID:m", "RECURRENCE-ID;VALUE=DATE:20261110", "DTSTART;VALUE=DATE:20261111", "END:VEVENT")
	want(t, expand(t, monthly, "2026-10-01T00:00Z", "2027-03-01T00:00Z"),
		"0 2026-10-13 10-13T00:00–10-14T00:00 all-day",
		"1 2026-11-10 11-11T00:00–11-12T00:00 all-day",
		"0 2026-12-08 12-08T00:00–12-09T00:00 all-day")
}

// An endless series yields only what the range holds; one that starts
// before the range yields its occurrences inside it.
func TestEndlessSeries(t *testing.T) {
	evs := events(t, me, "BEGIN:VEVENT", "UID:d", "RRULE:FREQ=DAILY;INTERVAL=2",
		"DTSTART:20200101T080000Z", "DTEND:20200101T081000Z", "END:VEVENT")
	want(t, expand(t, evs, "2026-10-08T00:00Z", "2026-10-13T00:00Z"),
		"0 2026-10-08T08:00:00.000Z 10-08T08:00–10-08T08:10",
		"0 2026-10-10T08:00:00.000Z 10-10T08:00–10-10T08:10",
		"0 2026-10-12T08:00:00.000Z 10-12T08:00–10-12T08:10")
	long := events(t, me, "BEGIN:VEVENT", "UID:l", "RRULE:FREQ=WEEKLY", "DTSTART:20261001T220000Z",
		"DTEND:20261002T020000Z", "END:VEVENT")
	want(t, expand(t, long, "2026-10-09T00:00Z", "2026-10-09T01:00Z"),
		"0 2026-10-08T22:00:00.000Z 10-08T22:00–10-09T02:00")
}

func TestOrphansAndOdd(t *testing.T) {
	orphan := events(t, me, "BEGIN:VEVENT", "UID:x", "RECURRENCE-ID:20261015T140000Z",
		"DTSTART:20261015T150000Z", "DTEND:20261015T160000Z", "END:VEVENT")
	want(t, expand(t, orphan, "2026-10-15T00:00Z", "2026-10-16T00:00Z"),
		"0 2026-10-15T14:00:00.000Z 10-15T15:00–10-15T16:00")
	bad := events(t, me, "BEGIN:VEVENT", "UID:y", "RRULE:FREQ=SOMETIMES;X-ODD=1",
		"DTSTART:20261015T140000Z", "DTEND:20261015T150000Z", "END:VEVENT")
	want(t, expand(t, bad, "2026-10-01T00:00Z", "2027-01-01T00:00Z"),
		"0 2026-10-15T14:00:00.000Z 10-15T14:00–10-15T15:00")
	// Two objects' events never mix: a second UID is its own series.
	two := events(t, me,
		"BEGIN:VEVENT", "UID:p", "DTSTART:20261015T140000Z", "END:VEVENT",
		"BEGIN:VEVENT", "UID:q", "DTSTART:20261015T130000Z", "END:VEVENT")
	want(t, expand(t, two, "2026-10-15T00:00Z", "2026-10-16T00:00Z"),
		"1  10-15T13:00–10-15T13:00",
		"0  10-15T14:00–10-15T14:00")
}
