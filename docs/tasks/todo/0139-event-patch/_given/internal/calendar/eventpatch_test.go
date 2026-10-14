package calendar

// CONTRACT TEST for task card T-0139 (docs/tasks). Do not edit.

import (
	"strings"
	"testing"
	"time"
)

var eventNow = time.Date(2026, 10, 14, 15, 30, 0, 0, time.UTC)

const nowStamp = "20261014T153000Z"

func ev(lines ...string) string { return strings.Join(lines, "\r\n") + "\r\n" }

// googleEvent is a timed event as Google Calendar's CalDAV serves it, with
// a property and an alarm Frostmail must keep.
var googleEvent = ev(
	"BEGIN:VCALENDAR", "PRODID:-//Google Inc//Google Calendar 70.9054//EN", "VERSION:2.0", "CALSCALE:GREGORIAN",
	"BEGIN:VTIMEZONE", "TZID:America/New_York", "X-LIC-LOCATION:America/New_York",
	"BEGIN:DAYLIGHT", "TZOFFSETFROM:-0500", "TZOFFSETTO:-0400", "TZNAME:EDT", "DTSTART:19700308T020000",
	"RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU", "END:DAYLIGHT",
	"BEGIN:STANDARD", "TZOFFSETFROM:-0400", "TZOFFSETTO:-0500", "TZNAME:EST", "DTSTART:19701101T020000",
	"RRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU", "END:STANDARD", "END:VTIMEZONE",
	"BEGIN:VEVENT",
	"DTSTART;TZID=America/New_York:20261015T090000", "DTEND;TZID=America/New_York:20261015T100000",
	"DTSTAMP:20261010T120000Z", "UID:abc123@google.com", "CREATED:20261010T115900Z",
	"LAST-MODIFIED:20261010T120000Z", "SEQUENCE:0", "STATUS:CONFIRMED", "SUMMARY:Planning", "LOCATION:Room 4",
	"DESCRIPTION:Agenda in the doc", "TRANSP:OPAQUE", "X-GOOGLE-CONFERENCE:https://meet.google.com/abc-defg-hij",
	"BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:This is an event reminder", "TRIGGER:-P0DT0H10M0S", "END:VALARM",
	"BEGIN:VALARM", "ACTION:EMAIL", "DESCRIPTION:This is an event reminder", "SUMMARY:Alarm notification",
	"ATTENDEE:mailto:ann@example.com", "TRIGGER:-P0DT0H30M0S", "END:VALARM",
	"END:VEVENT", "END:VCALENDAR",
)

// appleSeries is an all-day weekly series as Apple's Calendar writes it,
// with one occurrence moved (an override, written first).
var appleSeries = ev(
	"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Apple Inc.//macOS 15.0//EN", "CALSCALE:GREGORIAN",
	"BEGIN:VEVENT", "CREATED:20261001T080000Z", "DTEND;VALUE=DATE:20261021", "DTSTAMP:20261001T080000Z",
	"DTSTART;VALUE=DATE:20261020", "LAST-MODIFIED:20261001T080000Z", "RECURRENCE-ID;VALUE=DATE:20261019",
	"SEQUENCE:0", "SUMMARY:Bins out (late)", "UID:BINS-1", "END:VEVENT",
	"BEGIN:VEVENT", "CREATED:20261001T080000Z", "DTEND;VALUE=DATE:20261013", "DTSTAMP:20261001T080000Z",
	"DTSTART;VALUE=DATE:20261012", "LAST-MODIFIED:20261001T080000Z", "RRULE:FREQ=WEEKLY;BYDAY=MO",
	"SEQUENCE:0", "SUMMARY:Bins out", "UID:BINS-1", "X-APPLE-TRAVEL-ADVISORY-BEHAVIOR:AUTOMATIC",
	"BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:Reminder", "TRIGGER:PT9H", "UID:A1", "X-WR-ALARMUID:A1",
	"END:VALARM", "END:VEVENT", "END:VCALENDAR",
)

func sp[T any](v T) *T { return &v }

func patchedEvent(t *testing.T, src string, c EventChange) string {
	t.Helper()
	out, err := PatchEvent([]byte(src), c, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(out, Options{Local: time.UTC}); err != nil {
		t.Fatalf("the patched event does not parse: %v\n%s", err, out)
	}
	return string(out)
}

func same(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// stamped is googleEvent with DTSTAMP and LAST-MODIFIED now, as every
// change leaves them.
func stamped(src string, pairs ...string) string {
	return strings.NewReplacer(append([]string{
		"DTSTAMP:20261010T120000Z", "DTSTAMP:" + nowStamp, "LAST-MODIFIED:20261010T120000Z", "LAST-MODIFIED:" + nowStamp,
	}, pairs...)...).Replace(src)
}

// stampedMaster is appleSeries with the master's DTSTAMP and
// LAST-MODIFIED now; the override keeps its own.
func stampedMaster(pairs ...string) string {
	return strings.NewReplacer(append([]string{
		"DTEND;VALUE=DATE:20261013\r\nDTSTAMP:20261001T080000Z", "DTEND;VALUE=DATE:20261013\r\nDTSTAMP:" + nowStamp,
		"DTSTART;VALUE=DATE:20261012\r\nLAST-MODIFIED:20261001T080000Z", "DTSTART;VALUE=DATE:20261012\r\nLAST-MODIFIED:" + nowStamp,
	}, pairs...)...).Replace(appleSeries)
}

func master(t *testing.T, src string) Event {
	t.Helper()
	events, err := Parse([]byte(src), Options{Local: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.RecurrenceID == "" {
			return e
		}
	}
	t.Fatal("no master")
	return Event{}
}

func TestPatchEventTextKeepsTheRest(t *testing.T) {
	got := patchedEvent(t, googleEvent, EventChange{Summary: sp("Planning, Q4"), Location: sp("")})
	same(t, got, stamped(googleEvent, "SUMMARY:Planning\r\n", "SUMMARY:Planning\\, Q4\r\n", "LOCATION:Room 4\r\n", ""))
	if got := patchedEvent(t, googleEvent, EventChange{}); got != googleEvent {
		t.Errorf("an empty change rewrote the event:\n%s", got)
	}
}

func TestPatchEventMovesTheTimeInItsZone(t *testing.T) {
	got := patchedEvent(t, googleEvent, EventChange{AllDay: sp(false), Zone: sp("America/New_York"),
		Start: sp(time.Date(2026, 10, 16, 14, 0, 0, 0, time.UTC)), End: sp(time.Date(2026, 10, 16, 15, 30, 0, 0, time.UTC))})
	same(t, got, stamped(googleEvent,
		"DTSTART;TZID=America/New_York:20261015T090000", "DTSTART;TZID=America/New_York:20261016T100000",
		"DTEND;TZID=America/New_York:20261015T100000", "DTEND;TZID=America/New_York:20261016T113000",
		"SEQUENCE:0", "SEQUENCE:1"))
	if e := master(t, got); !e.Start.Equal(time.Date(2026, 10, 16, 14, 0, 0, 0, time.UTC)) || e.TZID != "America/New_York" {
		t.Errorf("read back %+v", e)
	}
}

func TestPatchEventInANewZoneAddsItsVTimezone(t *testing.T) {
	start := time.Date(2026, 10, 16, 7, 0, 0, 0, time.UTC)
	got := patchedEvent(t, googleEvent, EventChange{AllDay: sp(false), Zone: sp("Europe/Berlin"),
		Start: &start, End: sp(start.Add(time.Hour))})
	berlin, err := VTimezone("Europe/Berlin", start, "\r\n")
	if err != nil {
		t.Fatal(err)
	}
	same(t, got, stamped(googleEvent,
		"END:VTIMEZONE\r\nBEGIN:VEVENT\r\n", "END:VTIMEZONE\r\n"+berlin+"BEGIN:VEVENT\r\n",
		"DTSTART;TZID=America/New_York:20261015T090000", "DTSTART;TZID=Europe/Berlin:20261016T090000",
		"DTEND;TZID=America/New_York:20261015T100000", "DTEND;TZID=Europe/Berlin:20261016T100000",
		"SEQUENCE:0", "SEQUENCE:1"))
}

func TestPatchEventToAllDayAndToUTC(t *testing.T) {
	got := patchedEvent(t, googleEvent, EventChange{AllDay: sp(true), StartDate: sp("2026-10-16"), EndDate: sp("2026-10-18")})
	same(t, got, stamped(googleEvent,
		"DTSTART;TZID=America/New_York:20261015T090000", "DTSTART;VALUE=DATE:20261016",
		"DTEND;TZID=America/New_York:20261015T100000", "DTEND;VALUE=DATE:20261018",
		"SEQUENCE:0", "SEQUENCE:1"))
	if e := master(t, got); !e.AllDay || e.End.Sub(e.Start) != 48*time.Hour {
		t.Errorf("read back %+v", e)
	}
	got = patchedEvent(t, googleEvent, EventChange{AllDay: sp(false), Zone: sp("UTC"),
		Start: sp(time.Date(2026, 10, 16, 14, 0, 0, 0, time.UTC)), End: sp(time.Date(2026, 10, 16, 15, 0, 0, 0, time.UTC))})
	same(t, got, stamped(googleEvent,
		"DTSTART;TZID=America/New_York:20261015T090000", "DTSTART:20261016T140000Z",
		"DTEND;TZID=America/New_York:20261015T100000", "DTEND:20261016T150000Z",
		"SEQUENCE:0", "SEQUENCE:1"))
}

func TestPatchEventRefusesWhatCannotBeWritten(t *testing.T) {
	start := time.Date(2026, 10, 16, 14, 0, 0, 0, time.UTC)
	for name, c := range map[string]EventChange{
		"a start alone":      {Start: &start},
		"an end before":      {AllDay: sp(false), Zone: sp("UTC"), Start: &start, End: sp(start.Add(-time.Hour))},
		"no zone":            {AllDay: sp(false), Start: &start, End: sp(start.Add(time.Hour))},
		"an unknown zone":    {AllDay: sp(false), Zone: sp("Mars/Olympus"), Start: &start, End: sp(start.Add(time.Hour))},
		"no end date":        {AllDay: sp(true), StartDate: sp("2026-10-16")},
		"an end date before": {AllDay: sp(true), StartDate: sp("2026-10-16"), EndDate: sp("2026-10-16")},
		"a bad date":         {AllDay: sp(true), StartDate: sp("2026-10-32"), EndDate: sp("2026-11-01")},
		"a bad rule":         {RRule: sp("FREQ=SOMETIMES")},
	} {
		if _, err := PatchEvent([]byte(googleEvent), c, eventNow); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	for _, src := range []string{"not a calendar", ev("BEGIN:VCALENDAR", "VERSION:2.0", "END:VCALENDAR")} {
		if _, err := PatchEvent([]byte(src), EventChange{Summary: sp("x")}, eventNow); err == nil {
			t.Errorf("patched %q", src)
		}
	}
}

func TestPatchEventAlarms(t *testing.T) {
	got := patchedEvent(t, googleEvent, EventChange{Alarms: &[]int{15, 0}})
	display := "BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:This is an event reminder\r\nTRIGGER:-P0DT0H10M0S\r\nEND:VALARM\r\n"
	added := "BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Planning\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\n" +
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Planning\r\nTRIGGER:PT0S\r\nEND:VALARM\r\n"
	same(t, got, stamped(googleEvent, display, "", "END:VEVENT\r\n", added+"END:VEVENT\r\n"))
	e := master(t, got)
	if len(e.Alarms) != 2 || e.Alarms[0].Offset != -15*time.Minute || e.Alarms[1].Offset != 0 {
		t.Errorf("alarms read back as %+v", e.Alarms)
	}
	if !strings.Contains(got, "ACTION:EMAIL") {
		t.Error("the server's email alarm went")
	}
	for minutes, trigger := range map[int]string{1440: "-P1D", 60: "-PT1H", 900: "-PT15H", 2340: "-P1DT15H",
		-540: "PT9H", 90: "-PT1H30M", 10080: "-P7D"} {
		got := patchedEvent(t, googleEvent, EventChange{Alarms: &[]int{minutes}})
		if !strings.Contains(got, "TRIGGER:"+trigger+"\r\n") {
			t.Errorf("%d minutes before is not TRIGGER:%s", minutes, trigger)
		}
	}
	if got := patchedEvent(t, googleEvent, EventChange{Alarms: &[]int{}}); strings.Contains(got, "ACTION:DISPLAY") {
		t.Error("no alarms left a display alarm")
	}
}

// The master is the VEVENT without RECURRENCE-ID, wherever it is; the
// override stays as it was.
func TestPatchEventRuleAndTransparencyOnTheMaster(t *testing.T) {
	got := patchedEvent(t, appleSeries, EventChange{RRule: sp("FREQ=WEEKLY;BYDAY=MO,TH"), Transparent: sp(true)})
	same(t, got, stampedMaster(
		"RRULE:FREQ=WEEKLY;BYDAY=MO\r\nSEQUENCE:0", "RRULE:FREQ=WEEKLY;BYDAY=MO,TH\r\nSEQUENCE:1",
		"X-APPLE-TRAVEL-ADVISORY-BEHAVIOR:AUTOMATIC\r\nBEGIN:VALARM", "X-APPLE-TRAVEL-ADVISORY-BEHAVIOR:AUTOMATIC\r\nTRANSP:TRANSPARENT\r\nBEGIN:VALARM"))
	if !strings.Contains(got, "SUMMARY:Bins out (late)\r\nUID:BINS-1\r\nEND:VEVENT") || !strings.Contains(got, "DTSTAMP:20261001T080000Z") {
		t.Errorf("the override changed:\n%s", got)
	}
	if got := patchedEvent(t, appleSeries, EventChange{RRule: sp("")}); strings.Contains(got, "FREQ=WEEKLY") {
		t.Errorf("RRULE not removed:\n%s", got)
	}
}

func TestNewEvent(t *testing.T) {
	start := time.Date(2026, 10, 16, 7, 0, 0, 0, time.UTC)
	out, err := NewEvent("new-1", EventChange{Summary: sp("Lunch, with Ann"), Location: sp("Café"),
		Description: sp("Bring\nthe plan"), AllDay: sp(false), Zone: sp("Europe/Berlin"), Start: &start,
		End: sp(start.Add(time.Hour)), RRule: sp("FREQ=WEEKLY"), Alarms: &[]int{10}, Transparent: sp(false)}, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	berlin, _ := VTimezone("Europe/Berlin", start, "\r\n")
	same(t, string(out), ev("BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Frostyard//Frostmail//EN")+berlin+ev(
		"BEGIN:VEVENT", "UID:new-1", "DTSTAMP:"+nowStamp, "CREATED:"+nowStamp, "LAST-MODIFIED:"+nowStamp,
		"SEQUENCE:0", "DTSTART;TZID=Europe/Berlin:20261016T090000", "DTEND;TZID=Europe/Berlin:20261016T100000",
		"SUMMARY:Lunch\\, with Ann", "LOCATION:Café", "DESCRIPTION:Bring\\nthe plan", "RRULE:FREQ=WEEKLY",
		"TRANSP:OPAQUE", "BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:Lunch\\, with Ann", "TRIGGER:-PT10M",
		"END:VALARM", "END:VEVENT", "END:VCALENDAR"))

	out, err = NewEvent("new-2", EventChange{Summary: sp(""), AllDay: sp(true), StartDate: sp("2026-12-24"),
		EndDate: sp("2026-12-27")}, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	same(t, string(out), ev("BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Frostyard//Frostmail//EN",
		"BEGIN:VEVENT", "UID:new-2", "DTSTAMP:"+nowStamp, "CREATED:"+nowStamp, "LAST-MODIFIED:"+nowStamp,
		"SEQUENCE:0", "DTSTART;VALUE=DATE:20261224", "DTEND;VALUE=DATE:20261227", "END:VEVENT", "END:VCALENDAR"))

	if _, err := NewEvent("new-3", EventChange{Summary: sp("No time")}, eventNow); err == nil {
		t.Error("NewEvent took an event without a time")
	}
}
