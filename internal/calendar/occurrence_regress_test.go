package calendar

import (
	"strings"
	"testing"
	"time"
)

// A series of RDATEs alone moves with its overrides: they still name its
// occurrences.
func TestPatchAllMovesASeriesOfDates(t *testing.T) {
	series := ev(
		"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Example//Dates//EN",
		"BEGIN:VEVENT", "UID:dates-1", "DTSTAMP:20261010T120000Z", "DTSTART:20261015T130000Z", "DTEND:20261015T140000Z",
		"RDATE:20261022T130000Z,20261105T130000Z", "SUMMARY:Office hours", "END:VEVENT",
		"BEGIN:VEVENT", "UID:dates-1", "DTSTAMP:20261010T120000Z", "RECURRENCE-ID:20261022T130000Z",
		"DTSTART:20261022T130000Z", "DTEND:20261022T140000Z", "SUMMARY:Office hours (Room 4)", "END:VEVENT",
		"END:VCALENDAR",
	)
	start := time.Date(2026, 10, 22, 14, 0, 0, 0, time.UTC)
	got := patchedAll(t, series, "2026-10-22T13:00:00.000Z", EventChange{AllDay: sp(false), Zone: sp("UTC"),
		Start: &start, End: sp(start.Add(time.Hour))})
	hasLines(t, got, "DTSTART:20261015T140000Z", "RDATE:20261022T140000Z,20261105T140000Z",
		"RECURRENCE-ID:20261022T140000Z", "DTSTART:20261022T140000Z")
	occs := occurrences(t, got)
	if len(occs) != 3 || occs["2026-10-22T14:00:00.000Z"].Summary != "Office hours (Room 4)" {
		t.Errorf("occurrences = %+v\n%s", occs, got)
	}
	if strings.Count(got, "BEGIN:VEVENT") != 2 {
		t.Errorf("the override is gone:\n%s", got)
	}
}
