package calendar

import (
	"strings"
	"testing"
)

func TestRaiseSequence(t *testing.T) {
	if got := MaxSequence([]byte(googleSeries)); got != 0 {
		t.Errorf("MaxSequence = %d", got)
	}
	raised, err := RaiseSequence([]byte(googleSeries), 3)
	if err != nil {
		t.Fatal(err)
	}
	// Every VEVENT, the master and its override, goes past the floor, and
	// nothing else changes.
	same(t, string(raised), strings.ReplaceAll(googleSeries, "SEQUENCE:0\r\n", "SEQUENCE:4\r\n"))
	if got := MaxSequence(raised); got != 4 {
		t.Errorf("MaxSequence after = %d", got)
	}
	// A VEVENT without SEQUENCE gets one, before its alarms.
	bare := strings.Replace(googleEvent, "SEQUENCE:0\r\n", "", 1)
	raised, err = RaiseSequence([]byte(bare), 0)
	if err != nil || !strings.Contains(string(raised), "SEQUENCE:1\r\nBEGIN:VALARM") {
		t.Errorf("RaiseSequence of a bare event = %v\n%s", err, raised)
	}
}

func TestKeepOverrides(t *testing.T) {
	// The restored source no longer moves October 29; the server keeps its
	// exception, which goes back as a plain occurrence.
	restored := strings.Replace(googleSeries, googleOverride, "", 1)
	got, err := KeepOverrides([]byte(restored), []byte(googleSeries), eventNow)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "RECURRENCE-ID;TZID=America/New_York:20261029T090000\r\n") ||
		!strings.Contains(string(got), "DTSTART;TZID=America/New_York:20261029T090000\r\n") ||
		strings.Contains(string(got), "Standup (moved)") {
		t.Errorf("the kept override:\n%s", got)
	}
	occs := occurrences(t, string(got))
	if o := occs["2026-10-29T13:00:00.000Z"]; len(occs) != 10 || o.Summary != "Standup" || o.Start.Hour() != 13 {
		t.Errorf("occurrences = %+v", occs)
	}
	// Overrides the restored source has, or none at all, change nothing.
	same2, err := KeepOverrides([]byte(googleSeries), []byte(googleSeries), eventNow)
	if err != nil || string(same2) != googleSeries {
		t.Errorf("KeepOverrides of the same source = %v\n%s", err, same2)
	}
}
