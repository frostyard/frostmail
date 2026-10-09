package calendar

import (
	"testing"
	"time"
)

func TestHostileRules(t *testing.T) {
	// Sub-hourly rules read as single events.
	evs := events(t, me, "BEGIN:VEVENT", "UID:m", "DTSTART:20000101T000000Z", "DTEND:20000101T000001Z",
		"RRULE:FREQ=SECONDLY", "END:VEVENT")
	want(t, expand(t, evs, "2026-10-09T00:00Z", "2026-10-10T00:00Z"))
	want(t, expand(t, evs, "2000-01-01T00:00Z", "2000-01-01T00:01Z"), "0 2000-01-01T00:00:00.000Z 01-01T00:00–01-01T00:00")

	// A rule whose dates never happen and one generating more starts than
	// the bound finish quickly.
	for _, rule := range []string{"RRULE:FREQ=HOURLY;BYMONTH=2;BYMONTHDAY=30", "RRULE:FREQ=HOURLY"} {
		evs := events(t, me, "BEGIN:VEVENT", "UID:h", "DTSTART:19900101T000000Z", "DTEND:19900101T010000Z", rule, "END:VEVENT")
		start := time.Now()
		want(t, expand(t, evs, "2026-10-09T00:00Z", "2026-10-09T02:00Z"))
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("%s took %v", rule, d)
		}
	}
}

func TestExceptionInObjectZone(t *testing.T) {
	// Outlook names zones only in the object's VTIMEZONE; EXDATE uses the
	// same name.
	evs := events(t, me,
		"BEGIN:VTIMEZONE", "TZID:Customized Time Zone", "BEGIN:STANDARD", "DTSTART:16010101T000000",
		"TZOFFSETFROM:+0200", "TZOFFSETTO:+0200", "END:STANDARD", "END:VTIMEZONE",
		"BEGIN:VEVENT", "UID:x", "DTSTART;TZID=Customized Time Zone:20261009T090000",
		"DTEND;TZID=Customized Time Zone:20261009T100000", "RRULE:FREQ=DAILY;COUNT=3",
		"EXDATE;TZID=Customized Time Zone:20261010T090000", "END:VEVENT")
	want(t, expand(t, evs, "2026-10-01T00:00Z", "2026-11-01T00:00Z"),
		"0 2026-10-09T07:00:00.000Z 10-09T07:00–10-09T08:00",
		"0 2026-10-11T07:00:00.000Z 10-11T07:00–10-11T08:00")
}

func TestZoneNamed(t *testing.T) {
	local := time.FixedZone("here", 3600)
	for tzid, want := range map[string]string{
		"": "here", "UTC": "UTC", "Europe/Berlin": "Europe/Berlin", "UTC+05:30": "UTC+05:30", "UTC-03:00": "UTC-03:00",
		"Nowhere/Else": "UTC", "UTC+5": "UTC",
	} {
		if got := ZoneNamed(tzid, local).String(); got != want {
			t.Errorf("ZoneNamed(%q) = %s, want %s", tzid, got, want)
		}
	}
	at := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	if _, offset := at.In(ZoneNamed("UTC+05:30", local)).Zone(); offset != 5*3600+30*60 {
		t.Errorf("UTC+05:30 offset = %d", offset)
	}
	if _, offset := at.In(ZoneNamed("UTC-03:00", local)).Zone(); offset != -3*3600 {
		t.Errorf("UTC-03:00 offset = %d", offset)
	}
	// Fixed zones Parse makes round-trip.
	evs := events(t, me, "BEGIN:VTIMEZONE", "TZID:Custom", "BEGIN:STANDARD", "DTSTART:16010101T000000",
		"TZOFFSETFROM:+0530", "TZOFFSETTO:+0530", "END:STANDARD", "END:VTIMEZONE",
		"BEGIN:VEVENT", "UID:x", "DTSTART;TZID=Custom:20261009T090000", "END:VEVENT")
	if evs[0].TZID != "UTC+05:30" || ZoneNamed(evs[0].TZID, local).String() != evs[0].Zone.String() {
		t.Errorf("TZID %q, zone %s", evs[0].TZID, evs[0].Zone)
	}
}
