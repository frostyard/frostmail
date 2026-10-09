package pimsync_test

import (
	"slices"
	"testing"
)

// TestRepeatedRecurrenceIDs: an object holding two events with the same
// recurrence ID (two UIDs in one file) indexes the first instead of
// failing the calendar's sync.
func TestRepeatedRecurrenceIDs(t *testing.T) {
	now := utcAt("2026-10-08T12:00Z")
	e := newCalendarEnv(t, &now)
	cal := e.dav.Calendar("home", "Home", "")
	e.dav.Put(cal+"two.ics", ics(
		"BEGIN:VEVENT", "UID:one", "DTSTART:20261009T090000Z", "DTEND:20261009T100000Z", "SUMMARY:One", "END:VEVENT",
		"BEGIN:VEVENT", "UID:two", "DTSTART:20261009T110000Z", "DTEND:20261009T120000Z", "SUMMARY:Two", "END:VEVENT"))
	e.dav.Put(cal+"other.ics", ics("BEGIN:VEVENT", "UID:other", "DTSTART:20261009T130000Z", "DTEND:20261009T140000Z",
		"SUMMARY:Other", "END:VEVENT"))
	e.pass()
	if got := e.occurrences("2026-10-09", "2026-10-10"); !slices.Equal(got, []string{"10-09T09:00 One", "10-09T13:00 Other"}) {
		t.Errorf("occurrences = %q", got)
	}
}
