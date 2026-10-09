package calendar

// CONTRACT TEST for task card T-0067 (docs/tasks). Do not edit.

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func ics(lines ...string) []byte {
	return []byte(strings.Join(append(append([]string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//test//EN"}, lines...),
		"END:VCALENDAR"), "\r\n") + "\r\n")
}

func utc(s string) time.Time {
	t, err := time.Parse("2006-01-02T15:04Z", s)
	if err != nil {
		panic(err)
	}
	return t
}

func parseOne(t *testing.T, raw []byte, opts Options) Event {
	t.Helper()
	evs, err := Parse(raw, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("events = %+v, want one", evs)
	}
	return evs[0]
}

var me = Options{UserEmails: []string{"ann@example.com"}}

// A Google Calendar event with a zone, attendees and alarms.
func TestGoogleEvent(t *testing.T) {
	ev := parseOne(t, ics(
		"BEGIN:VEVENT",
		"DTSTART;TZID=America/New_York:20261022T090000",
		"DTEND;TZID=America/New_York:20261022T100000",
		"DTSTAMP:20261001T120000Z",
		"ORGANIZER;CN=Maria Lopez:mailto:maria@example.com",
		"UID:abc123@google.com",
		`ATTENDEE;CUTYPE=INDIVIDUAL;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;CN="Lopez, Maria";X-NUM-GUESTS=0:mailto:maria@example.com`,
		"ATTENDEE;CUTYPE=INDIVIDUAL;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;CN=ann@example.com:mailto:Ann@Example.com",
		"ATTENDEE;ROLE=OPT-PARTICIPANT;PARTSTAT=DECLINED:mailto:bob@example.com",
		`DESCRIPTION:Agenda:\n1. Launch\, then review`,
		`LOCATION:Room 4\; Building B`,
		"SEQUENCE:2",
		"STATUS:CONFIRMED",
		"SUMMARY:Launch review",
		"TRANSP:OPAQUE",
		"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER:-PT10M", "DESCRIPTION:Reminder", "END:VALARM",
		"BEGIN:VALARM", "ACTION:EMAIL", "TRIGGER:-P1D", "SUMMARY:x", "DESCRIPTION:x", "ATTENDEE:mailto:ann@example.com", "END:VALARM",
		"END:VEVENT",
	), me)
	ny, _ := time.LoadLocation("America/New_York")
	want := Event{
		UID: "abc123@google.com", Summary: "Launch review", Location: "Room 4; Building B",
		Description: "Agenda:\n1. Launch, then review",
		Start:       utc("2026-10-22T13:00Z"), End: utc("2026-10-22T14:00Z"), TZID: "America/New_York",
		Status: "confirmed", Sequence: 2, PartStat: "needsaction",
		Organizer: &Attendee{Email: "maria@example.com", Name: "Maria Lopez", Role: "chair", PartStat: "accepted"},
		Attendees: []Attendee{
			{Email: "ann@example.com", Name: "ann@example.com", Role: "required", PartStat: "needsaction"},
			{Email: "bob@example.com", Role: "optional", PartStat: "declined"},
		},
		Alarms: []Alarm{{Action: "display", Offset: -10 * time.Minute}},
		Zone:   ny,
	}
	check(t, ev, want)
}

// An Outlook invitation: a Windows zone name, MAILTO in capitals, a series.
func TestOutlookInvitation(t *testing.T) {
	ev := parseOne(t, ics(
		"METHOD:REQUEST",
		"BEGIN:VTIMEZONE", "TZID:Eastern Standard Time",
		"BEGIN:STANDARD", "DTSTART:16010101T020000", "TZOFFSETFROM:-0400", "TZOFFSETTO:-0500",
		"RRULE:FREQ=YEARLY;BYDAY=1SU;BYMONTH=11", "END:STANDARD",
		"BEGIN:DAYLIGHT", "DTSTART:16010101T020000", "TZOFFSETFROM:-0500", "TZOFFSETTO:-0400",
		"RRULE:FREQ=YEARLY;BYDAY=2SU;BYMONTH=3", "END:DAYLIGHT",
		"END:VTIMEZONE",
		"BEGIN:VEVENT",
		"ORGANIZER;CN=Peter Brown:MAILTO:peter@example.com",
		"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=TENTATIVE;RSVP=TRUE;CN=Ann:MAILTO:ANN@EXAMPLE.COM",
		"RRULE:FREQ=WEEKLY;BYDAY=TH;COUNT=6",
		"EXDATE;TZID=Eastern Standard Time:20261105T090000",
		"UID:040000008200E00074C5B7101A82E008",
		"SUMMARY;LANGUAGE=en-US:Weekly sync",
		"DTSTART;TZID=Eastern Standard Time:20261029T090000",
		"DTEND;TZID=Eastern Standard Time:20261029T093000",
		"SEQUENCE:0",
		"TRANSP:TRANSPARENT",
		"STATUS:TENTATIVE",
		"END:VEVENT",
	), me)
	if ev.TZID != "America/New_York" || !ev.Start.Equal(utc("2026-10-29T13:00Z")) || !ev.End.Equal(utc("2026-10-29T13:30Z")) {
		t.Errorf("times = %s %v–%v", ev.TZID, ev.Start, ev.End)
	}
	if ev.PartStat != "tentative" || ev.Organizer == nil || ev.Organizer.Email != "peter@example.com" || len(ev.Attendees) != 1 {
		t.Errorf("people = %+v / %+v / %q", ev.Organizer, ev.Attendees, ev.PartStat)
	}
	if ev.Recurrence != "RRULE:FREQ=WEEKLY;BYDAY=TH;COUNT=6\nEXDATE;TZID=Eastern Standard Time:20261105T090000" {
		t.Errorf("recurrence = %q", ev.Recurrence)
	}
	if !ev.Transparent || ev.Status != "tentative" || ev.Summary != "Weekly sync" {
		t.Errorf("event = %+v", ev)
	}
}

func TestAllDay(t *testing.T) {
	ev := parseOne(t, ics("BEGIN:VEVENT", "UID:h", "SUMMARY:Holidays",
		"DTSTART;VALUE=DATE:20261224", "DTEND;VALUE=DATE:20261226", "END:VEVENT"), me)
	if !ev.AllDay || !ev.Start.Equal(utc("2026-12-24T00:00Z")) || !ev.End.Equal(utc("2026-12-26T00:00Z")) ||
		ev.TZID != "" || ev.Zone != time.UTC || ev.Floating {
		t.Errorf("all-day = %+v", ev)
	}
	one := parseOne(t, ics("BEGIN:VEVENT", "UID:b", "DTSTART;VALUE=DATE:20260412", "END:VEVENT"), me)
	if !one.End.Equal(utc("2026-04-13T00:00Z")) || one.Status != "confirmed" {
		t.Errorf("one day = %+v", one)
	}
	dur := parseOne(t, ics("BEGIN:VEVENT", "UID:d", "DTSTART;VALUE=DATE:20260412", "DURATION:P3D", "END:VEVENT"), me)
	if !dur.End.Equal(utc("2026-04-15T00:00Z")) {
		t.Errorf("three days = %+v", dur)
	}
}

func TestUTCAndFloating(t *testing.T) {
	ev := parseOne(t, ics("BEGIN:VEVENT", "UID:u", "DTSTART:20261009T140000Z", "DURATION:PT1H30M", "END:VEVENT"), me)
	if ev.TZID != "UTC" || !ev.Start.Equal(utc("2026-10-09T14:00Z")) || !ev.End.Equal(utc("2026-10-09T15:30Z")) || ev.Zone != time.UTC {
		t.Errorf("UTC = %+v", ev)
	}
	noEnd := parseOne(t, ics("BEGIN:VEVENT", "UID:n", "DTSTART:20261009T140000Z", "END:VEVENT"), me)
	if !noEnd.End.Equal(noEnd.Start) {
		t.Errorf("a timed event without an end = %v–%v", noEnd.Start, noEnd.End)
	}
	berlin, _ := time.LoadLocation("Europe/Berlin")
	fl := parseOne(t, ics("BEGIN:VEVENT", "UID:f", "DTSTART:20261009T090000", "DTEND:20261009T100000", "END:VEVENT"),
		Options{Local: berlin})
	if !fl.Floating || fl.TZID != "" || !fl.Start.Equal(utc("2026-10-09T07:00Z")) || fl.Zone != berlin {
		t.Errorf("floating = %+v", fl)
	}
}

func TestZones(t *testing.T) {
	paris := parseOne(t, ics(
		"BEGIN:VTIMEZONE", "TZID:/citadel.org/20190914_1/Europe/Paris", "X-LIC-LOCATION:Europe/Paris",
		"BEGIN:STANDARD", "DTSTART:19701025T030000", "TZOFFSETFROM:+0200", "TZOFFSETTO:+0100", "END:STANDARD",
		"END:VTIMEZONE",
		"BEGIN:VEVENT", "UID:p", "DTSTART;TZID=/citadel.org/20190914_1/Europe/Paris:20260115T100000", "END:VEVENT"), me)
	if paris.TZID != "Europe/Paris" || !paris.Start.Equal(utc("2026-01-15T09:00Z")) {
		t.Errorf("X-LIC-LOCATION = %s %v", paris.TZID, paris.Start)
	}
	india := parseOne(t, ics(
		"BEGIN:VTIMEZONE", "TZID:(UTC+05:30) Chennai\\, Kolkata",
		"BEGIN:STANDARD", "DTSTART:16010101T000000", "TZOFFSETFROM:+0530", "TZOFFSETTO:+0530", "END:STANDARD",
		"END:VTIMEZONE",
		"BEGIN:VEVENT", "UID:i", `DTSTART;TZID="(UTC+05:30) Chennai, Kolkata":20261009T100000`, "END:VEVENT"), me)
	if india.TZID != "UTC+05:30" || !india.Start.Equal(utc("2026-10-09T04:30Z")) {
		t.Errorf("VTIMEZONE only = %s %v", india.TZID, india.Start)
	}
	if _, off := india.Start.In(india.Zone).Zone(); off != 5*3600+30*60 {
		t.Errorf("zone offset = %d", off)
	}
	mars := parseOne(t, ics("BEGIN:VEVENT", "UID:m", "DTSTART;TZID=Mars/Olympus:20261009T100000", "END:VEVENT"), me)
	if mars.TZID != "UTC" || !mars.Start.Equal(utc("2026-10-09T10:00Z")) {
		t.Errorf("unknown zone = %s %v", mars.TZID, mars.Start)
	}
}

// A series' master and its override, as CalDAV keeps them in one object.
func TestOverrides(t *testing.T) {
	evs, err := Parse(ics(
		"BEGIN:VEVENT", "UID:s", "SUMMARY:Standup", "RRULE:FREQ=DAILY;COUNT=5",
		"DTSTART;TZID=America/New_York:20261026T090000", "DTEND;TZID=America/New_York:20261026T091500", "END:VEVENT",
		"BEGIN:VEVENT", "UID:s", "SUMMARY:Standup (moved)", "RECURRENCE-ID;TZID=America/New_York:20261028T090000",
		"DTSTART;TZID=America/New_York:20261028T100000", "DTEND;TZID=America/New_York:20261028T101500", "END:VEVENT",
		"BEGIN:VEVENT", "UID:a", "SUMMARY:Off", "RECURRENCE-ID;VALUE=DATE:20261225",
		"DTSTART;VALUE=DATE:20261226", "STATUS:CANCELLED", "END:VEVENT",
	), me)
	if err != nil || len(evs) != 3 {
		t.Fatalf("events = %+v, %v", evs, err)
	}
	if evs[0].RecurrenceID != "" || evs[0].Recurrence != "RRULE:FREQ=DAILY;COUNT=5" {
		t.Errorf("master = %+v", evs[0])
	}
	if evs[1].RecurrenceID != "2026-10-28T13:00:00.000Z" || !evs[1].Start.Equal(utc("2026-10-28T14:00Z")) {
		t.Errorf("override = %+v", evs[1])
	}
	if evs[2].RecurrenceID != "2026-12-25" || evs[2].Status != "cancelled" || !evs[2].AllDay {
		t.Errorf("all-day override = %+v", evs[2])
	}
}

func TestAlarms(t *testing.T) {
	ev := parseOne(t, ics("BEGIN:VEVENT", "UID:x", "DTSTART:20261009T140000Z",
		"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER:-PT15M", "END:VALARM",
		"BEGIN:VALARM", "ACTION:AUDIO", "TRIGGER;RELATED=END:PT0S", "END:VALARM",
		"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER;VALUE=DATE-TIME:20261009T120000Z", "END:VALARM",
		"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER:-P1DT2H30M", "END:VALARM",
		"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER:+P1W", "END:VALARM",
		"BEGIN:VALARM", "ACTION:X-WR-ALARM", "TRIGGER:-PT5M", "END:VALARM",
		"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER:soon", "END:VALARM",
		"END:VEVENT"), me)
	want := []Alarm{
		{Action: "display", Offset: -15 * time.Minute},
		{Action: "audio", RelatedEnd: true},
		{Action: "display", At: utc("2026-10-09T12:00Z")},
		{Action: "display", Offset: -(26*time.Hour + 30*time.Minute)},
		{Action: "display", Offset: 7 * 24 * time.Hour},
	}
	if !reflect.DeepEqual(ev.Alarms, want) {
		t.Errorf("alarms = %+v\nwant %+v", ev.Alarms, want)
	}
}

func TestPeople(t *testing.T) {
	ev := parseOne(t, ics("BEGIN:VEVENT", "UID:o", "DTSTART:20261009T140000Z",
		"ORGANIZER:mailto:ann@example.com",
		"ATTENDEE;ROLE=CHAIR;PARTSTAT=ACCEPTED:mailto:chair@example.com",
		"ATTENDEE;ROLE=NON-PARTICIPANT;PARTSTAT=X-UNKNOWN:mailto:cc@example.com",
		"ATTENDEE;PARTSTAT=DELEGATED:mailto:del@example.com",
		"ATTENDEE:invalid-not-mailto",
		"END:VEVENT"), me)
	if ev.PartStat != "" {
		t.Errorf("the organizer has no answer: %q", ev.PartStat)
	}
	want := []Attendee{
		{Email: "chair@example.com", Role: "chair", PartStat: "accepted"},
		{Email: "cc@example.com", Role: "none", PartStat: "needsaction"},
		{Email: "del@example.com", Role: "required", PartStat: "delegated"},
		{Email: "invalid-not-mailto", Role: "required", PartStat: "needsaction"},
	}
	if !reflect.DeepEqual(ev.Attendees, want) {
		t.Errorf("attendees = %+v\nwant %+v", ev.Attendees, want)
	}
}

func TestParseOddities(t *testing.T) {
	if _, err := Parse([]byte("BEGIN:VCARD\r\nEND:VCARD\r\n"), me); err == nil {
		t.Error("a vCard parsed as a calendar")
	}
	if _, err := Parse([]byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\n"), me); err == nil {
		t.Error("an unclosed calendar parsed")
	}
	todo, err := Parse(ics("BEGIN:VTODO", "UID:t", "END:VTODO"), me)
	if err != nil || len(todo) != 0 {
		t.Errorf("a task's events = %+v, %v", todo, err)
	}
	noStart, err := Parse(ics("BEGIN:VEVENT", "UID:n", "SUMMARY:No start", "END:VEVENT",
		"BEGIN:VEVENT", "UID:y", "DTSTART:20261009T140000Z", "END:VEVENT"), me)
	if err != nil || len(noStart) != 1 || noStart[0].UID != "y" {
		t.Errorf("an event without DTSTART = %+v, %v; want it left out", noStart, err)
	}
}

func TestRecurrenceKey(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	if got := RecurrenceKey(time.Date(2026, 10, 29, 9, 0, 0, 0, ny), false); got != "2026-10-29T13:00:00.000Z" {
		t.Errorf("timed = %q", got)
	}
	if got := RecurrenceKey(utc("2026-12-25T00:00Z"), true); got != "2026-12-25" {
		t.Errorf("all-day = %q", got)
	}
}

func check(t *testing.T, got, want Event) {
	t.Helper()
	gv, wv := reflect.ValueOf(got), reflect.ValueOf(want)
	for i := range gv.NumField() {
		f := gv.Type().Field(i).Name
		a, b := gv.Field(i).Interface(), wv.Field(i).Interface()
		if f == "Start" || f == "End" {
			if !a.(time.Time).Equal(b.(time.Time)) {
				t.Errorf("%s = %v, want %v", f, a, b)
			}
			continue
		}
		if f == "Zone" {
			if a.(*time.Location).String() != b.(*time.Location).String() {
				t.Errorf("Zone = %v, want %v", a, b)
			}
			continue
		}
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s = %#v\nwant %#v", f, a, b)
		}
	}
}
