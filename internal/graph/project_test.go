package graph

import (
	"encoding/json/v2"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"
)

var newYork = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return loc
}()

var userEmails = []string{"cy@hotmail.com"}

func project(t *testing.T, src []byte) EventProjection {
	t.Helper()
	p, err := ProjectEvent(src, userEmails, newYork)
	if err != nil {
		t.Fatalf("project: %v\n%s", err, src)
	}
	return p
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestProjectSingleEvent(t *testing.T) {
	src, err := EventSource(event(t, single), nil, "cy@hotmail.com")
	if err != nil {
		t.Fatal(err)
	}
	p := project(t, src)
	f := p.Master.Fields
	if f["subject"] != "FM probe single" || asJSON(t, f["start"]) != `{"dateTime":"2026-10-27T10:00:00","timeZone":"Eastern Standard Time"}` ||
		asJSON(t, f["location"]) != `{"displayName":"Room 4"}` || f["isReminderOn"] != true || f["reminderMinutesBeforeStart"] != 10 ||
		f["showAs"] != "busy" || !p.Organizer || asJSON(t, f["attendees"]) != `[]` {
		t.Errorf("fields = %s", asJSON(t, f))
	}
	if d := Diff(f, project(t, src).Master.Fields); len(d) != 0 {
		t.Errorf("a source differs from itself: %v", d)
	}
}

// An edit sends only what it changed: the HTML body, the categories and
// everything else Graph has stay as they are.
func TestEditDiffsOnlyWhatChanged(t *testing.T) {
	src, err := EventSource(event(t, single), nil, "cy@hotmail.com")
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(src), "SUMMARY:FM probe single", "SUMMARY:FM probe, renamed", 1)
	edited = strings.Replace(edited, "DTSTART;TZID=America/New_York:20261027T100000", "DTSTART;TZID=America/New_York:20261027T103000", 1)
	edited = strings.Replace(edited, "DTEND;TZID=America/New_York:20261027T110000", "DTEND;TZID=America/New_York:20261027T113000", 1)
	d := Diff(project(t, src).Master.Fields, project(t, []byte(edited)).Master.Fields)
	want := `{"end":{"dateTime":"2026-10-27T11:30:00","timeZone":"Eastern Standard Time"},"start":{"dateTime":"2026-10-27T10:30:00","timeZone":"Eastern Standard Time"},"subject":"FM probe, renamed"}`
	if got := asJSON(t, d); got != want {
		t.Errorf("diff = %s\nwant %s", got, want)
	}
}

func TestProjectSeries(t *testing.T) {
	src, err := EventSource(event(t, series), []Event{event(t, exception)}, "")
	if err != nil {
		t.Fatal(err)
	}
	p := project(t, src)
	if got := asJSON(t, p.Master.Fields["recurrence"]); got != `{"pattern":{"interval":1,"type":"daily"},"range":{"numberOfOccurrences":5,"recurrenceTimeZone":"Eastern Standard Time","startDate":"2026-10-26","type":"numbered"}}` {
		t.Errorf("recurrence = %s", got)
	}
	if !slices.Equal(p.Cancelled, []string{"2026-10-28T13:30:00.000Z"}) {
		t.Errorf("cancelled = %v", p.Cancelled)
	}
	ex, ok := p.Overrides["2026-10-27T13:30:00.000Z"]
	if !ok || asJSON(t, ex.Fields["start"]) != `{"dateTime":"2026-10-27T10:00:00","timeZone":"Eastern Standard Time"}` {
		t.Errorf("overrides = %v", p.Overrides)
	}
	// An occurrence the series makes is the master at its start.
	occ, err := p.Occurrence("2026-10-29T13:30:00.000Z")
	if err != nil {
		t.Fatal(err)
	}
	if asJSON(t, occ["start"]) != `{"dateTime":"2026-10-29T09:30:00","timeZone":"Eastern Standard Time"}` ||
		asJSON(t, occ["end"]) != `{"dateTime":"2026-10-29T09:45:00","timeZone":"Eastern Standard Time"}` || occ["recurrence"] != nil {
		t.Errorf("occurrence = %s", asJSON(t, occ))
	}
	s, err := SeriesOf(event(t, strings.Replace(series, `"cancelledOccurrences"`, `"exceptionOccurrences": [`+exception+`], "cancelledOccurrences"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := s.Exception("2026-10-27T13:30:00.000Z"); !ok || id != "AAMk-exception" {
		t.Errorf("exception = %q, %v", id, ok)
	}
}

// Every recurrence Graph sends comes back as the same rule, through the
// derived source and its projection.
func TestRecurrenceRoundTrips(t *testing.T) {
	for _, c := range []struct {
		recurrence string
		start      string
		allDay     bool
	}{
		{`{"pattern": {"type": "daily", "interval": 2}, "range": {"type": "noEnd", "startDate": "2026-10-26", "recurrenceTimeZone": "Eastern Standard Time"}}`, "2026-10-26T13:00:00", false},
		{`{"pattern": {"type": "weekly", "interval": 1, "daysOfWeek": ["monday", "wednesday"], "firstDayOfWeek": "sunday"}, "range": {"type": "numbered", "startDate": "2026-10-26", "recurrenceTimeZone": "Eastern Standard Time", "numberOfOccurrences": 6}}`, "2026-10-26T13:00:00", false},
		{`{"pattern": {"type": "absoluteMonthly", "interval": 1, "dayOfMonth": 15}, "range": {"type": "endDate", "startDate": "2026-11-15", "endDate": "2027-02-15", "recurrenceTimeZone": "Eastern Standard Time"}}`, "2026-11-15T15:00:00", false},
		{`{"pattern": {"type": "relativeMonthly", "interval": 2, "daysOfWeek": ["tuesday"], "index": "second"}, "range": {"type": "endDate", "startDate": "2026-11-10", "endDate": "2027-03-09", "recurrenceTimeZone": "Eastern Standard Time"}}`, "2026-11-10T15:00:00", false},
		{`{"pattern": {"type": "relativeMonthly", "interval": 1, "daysOfWeek": ["saturday", "sunday"], "index": "last"}, "range": {"type": "noEnd", "startDate": "2026-10-31", "recurrenceTimeZone": "Eastern Standard Time"}}`, "2026-10-31T14:00:00", false},
		{`{"pattern": {"type": "absoluteYearly", "interval": 1, "month": 5, "dayOfMonth": 17}, "range": {"type": "noEnd", "startDate": "2027-05-17", "recurrenceTimeZone": "UTC"}}`, "2027-05-17T00:00:00", true},
		{`{"pattern": {"type": "relativeYearly", "interval": 1, "month": 11, "daysOfWeek": ["thursday"], "index": "fourth"}, "range": {"type": "endDate", "startDate": "2026-11-26", "endDate": "2030-11-28", "recurrenceTimeZone": "UTC"}}`, "2026-11-26T00:00:00", true},
	} {
		e := event(t, `{"id": "m", "iCalUId": "u", "type": "seriesMaster", "subject": "x", "isAllDay": `+map[bool]string{true: "true", false: "false"}[c.allDay]+`,
			"start": {"dateTime": "`+c.start+`.0000000", "timeZone": "UTC"}, "end": {"dateTime": "`+c.start+`.0000000", "timeZone": "UTC"},
			"originalStartTimeZone": "Eastern Standard Time", "recurrence": `+c.recurrence+`}`)
		start, _ := parseGraphTime(e.Start)
		loc := newYork
		if c.allDay {
			loc = time.UTC
		}
		want, err := RRule(*e.Recurrence, start, loc, c.allDay)
		if err != nil {
			t.Fatal(err)
		}
		src, err := EventSource(e, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		var back Recurrence
		if err := json.Unmarshal([]byte(asJSON(t, project(t, src).Master.Fields["recurrence"])), &back); err != nil {
			t.Fatal(err)
		}
		got, err := RRule(back, start, loc, c.allDay)
		if err != nil || got != want {
			t.Errorf("%s: came back as %q (%v), want %q", c.recurrence, got, err, want)
		}
		if back.Range.StartDate != e.Recurrence.Range.StartDate {
			t.Errorf("%s: startDate %q", c.recurrence, back.Range.StartDate)
		}
	}
}

func TestRulesOutlookCannotStore(t *testing.T) {
	start := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	for _, rule := range []string{
		"FREQ=HOURLY", "FREQ=MONTHLY;BYMONTHDAY=-1", "FREQ=MONTHLY;BYDAY=1MO,2TU", "FREQ=WEEKLY;BYMONTH=3",
		"FREQ=DAILY;INTERVAL=2;BYDAY=MO,FR", "FREQ=YEARLY;BYWEEKNO=20", "FREQ=MONTHLY;BYDAY=5FR", "FREQ=DAILY;COUNT=2;UNTIL=20261030T000000Z",
	} {
		if _, err := RecurrenceFromRule(rule, start, newYork, "Eastern Standard Time", false); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v", rule, err)
		}
	}
	src := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:r\r\nDTSTART:20261026T130000Z\r\nDTEND:20261026T140000Z\r\n" +
		"RRULE:FREQ=DAILY;COUNT=3\r\nRDATE:20261101T130000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if _, err := ProjectEvent([]byte(src), nil, newYork); !errors.Is(err, ErrUnsupported) {
		t.Errorf("an RDATE: err = %v", err)
	}
	// Weekdays are Graph's weekly pattern.
	rec, err := RecurrenceFromRule("FREQ=DAILY;BYDAY=MO,TU,WE,TH,FR;UNTIL=20261106T140000Z", start, newYork, "Eastern Standard Time", false)
	if err != nil || asJSON(t, rec) != `{"pattern":{"daysOfWeek":["monday","tuesday","wednesday","thursday","friday"],"firstDayOfWeek":"monday","interval":1,"type":"weekly"},"range":{"endDate":"2026-11-06","recurrenceTimeZone":"Eastern Standard Time","startDate":"2026-10-26","type":"endDate"}}` {
		t.Errorf("weekdays = %s, %v", asJSON(t, rec), err)
	}
}

// An all-day event is written at midnight in the user's zone, and read
// back on its day wherever its zone is.
// UNTIL is an instant and Graph's end a day: the last day with an
// occurrence by then.
func TestUntilIsTheLastOccurrencesDay(t *testing.T) {
	start := time.Date(2026, 10, 26, 13, 30, 0, 0, time.UTC) // 09:30 in New York
	for until, want := range map[string]string{
		"20261029T133000Z": "2026-10-29", "20261029T132959Z": "2026-10-28", "20261105T143000Z": "2026-11-05",
		"20261105T142959Z": "2026-11-04", "20261105": "2026-11-05",
	} {
		rec, err := RecurrenceFromRule("FREQ=DAILY;UNTIL="+until, start, newYork, "Eastern Standard Time", false)
		if err != nil {
			t.Fatal(err)
		}
		if got := rec["range"].(map[string]any)["endDate"]; got != want {
			t.Errorf("UNTIL=%s: endDate %v, want %s", until, got, want)
		}
	}
}

func TestAllDayInTheUsersZone(t *testing.T) {
	src := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:d\r\nDTSTART;VALUE=DATE:20261028\r\nDTEND;VALUE=DATE:20261029\r\nSUMMARY:FM day\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	p := project(t, []byte(src))
	if asJSON(t, p.Master.Fields["start"]) != `{"dateTime":"2026-10-28T00:00:00","timeZone":"Eastern Standard Time"}` || p.Master.Fields["isAllDay"] != true {
		t.Errorf("fields = %s", asJSON(t, p.Master.Fields))
	}
	for _, c := range []struct{ zone, start, end string }{
		{"Eastern Standard Time", "2026-10-28T04:00:00", "2026-10-29T04:00:00"},
		{"W. Europe Standard Time", "2026-10-27T23:00:00", "2026-10-28T23:00:00"},
		{"W. Europe Standard Time", "2026-10-28T00:00:00", "2026-10-29T00:00:00"},
	} {
		e := event(t, `{"id": "d", "iCalUId": "d", "type": "singleInstance", "subject": "FM day", "isAllDay": true,
			"start": {"dateTime": "`+c.start+`.0000000", "timeZone": "UTC"}, "end": {"dateTime": "`+c.end+`.0000000", "timeZone": "UTC"},
			"originalStartTimeZone": "`+c.zone+`", "originalEndTimeZone": "`+c.zone+`"}`)
		src, err := EventSource(e, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), "DTSTART;VALUE=DATE:20261028\r\nDTEND;VALUE=DATE:20261029\r\n") {
			t.Errorf("%s from %s:\n%s", c.zone, c.start, src)
		}
	}
}

// An invitee's copy has no attendees to write, and an answer.
func TestProjectInvitation(t *testing.T) {
	e := event(t, `{"id": "inv", "iCalUId": "inv-uid", "type": "singleInstance", "subject": "Planning",
		"start": {"dateTime": "2026-11-03T15:00:00.0000000", "timeZone": "UTC"}, "end": {"dateTime": "2026-11-03T16:00:00.0000000", "timeZone": "UTC"},
		"originalStartTimeZone": "Pacific Standard Time", "isOrganizer": false,
		"organizer": {"emailAddress": {"name": "Bob", "address": "bob@example.com"}},
		"attendees": [{"type": "required", "status": {"response": "none"}, "emailAddress": {"name": "Cy", "address": "cy@hotmail.com"}}],
		"responseStatus": {"response": "notResponded"}}`)
	src, err := EventSource(e, nil, "cy@hotmail.com")
	if err != nil {
		t.Fatal(err)
	}
	p := project(t, src)
	if p.Organizer || p.Master.Answer != "needsaction" || p.Master.Fields["attendees"] != nil || p.UID != "inv-uid" {
		t.Errorf("projection = %+v", p)
	}
	if asJSON(t, p.Master.Fields["start"]) != `{"dateTime":"2026-11-03T07:00:00","timeZone":"Pacific Standard Time"}` {
		t.Errorf("start = %s", asJSON(t, p.Master.Fields["start"]))
	}
}

func TestOrganizersInvitees(t *testing.T) {
	src := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:o\r\nDTSTART:20261103T150000Z\r\nDTEND:20261103T160000Z\r\nSUMMARY:FM planning\r\n" +
		"ORGANIZER;CN=Cy:mailto:cy@hotmail.com\r\nATTENDEE;CN=Cy;PARTSTAT=ACCEPTED:mailto:cy@hotmail.com\r\n" +
		"ATTENDEE;CN=Bee;ROLE=OPT-PARTICIPANT:mailto:bee@example.com\r\nATTENDEE:mailto:dee@example.com\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	p := project(t, []byte(src))
	want := `[{"emailAddress":{"address":"bee@example.com","name":"Bee"},"type":"optional"},{"emailAddress":{"address":"dee@example.com","name":""},"type":"required"}]`
	if !p.Organizer || asJSON(t, p.Master.Fields["attendees"]) != want {
		t.Errorf("attendees = %s", asJSON(t, p.Master.Fields["attendees"]))
	}
	if asJSON(t, p.Master.Fields["start"]) != `{"dateTime":"2026-11-03T15:00:00","timeZone":"UTC"}` {
		t.Errorf("start = %s", asJSON(t, p.Master.Fields["start"]))
	}
}

func TestGraphZone(t *testing.T) {
	for name, want := range map[string]string{
		"America/New_York": "Eastern Standard Time", "Europe/Berlin": "W. Europe Standard Time", "America/Detroit": "America/Detroit",
		"UTC": "UTC", "Etc/UTC": "UTC", "Local": "UTC", "": "UTC", "Nowhere/Special": "UTC",
	} {
		if got := GraphZone(name); got != want {
			t.Errorf("GraphZone(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestProjectContact(t *testing.T) {
	var c Contact
	if err := json.Unmarshal([]byte(`{"id": "AAMk-contact", "displayName": "FM Probe", "givenName": "FM", "surname": "Probe",
		"companyName": "Frostyard", "jobTitle": "Tester", "emailAddresses": [{"name": "FM Probe", "address": "fm.probe@example.com"}],
		"businessPhones": ["+1 555 0101"], "homePhones": ["+1 555 0102"], "mobilePhone": "+1 555 0100",
		"homeAddress": {"street": "1 Main St", "city": "Springfield", "state": "OR", "postalCode": "97477", "countryOrRegion": "United States"},
		"businessAddress": {}, "birthday": "1980-05-17T11:59:00Z", "personalNotes": "Likes tea.", "imAddresses": ["fm@im.example"]}`), &c); err != nil {
		t.Fatal(err)
	}
	src := ContactSource(c, []byte("\xff\xd8\xff\xe0jpeg"), "image/jpeg")
	p, err := ProjectContact(src)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"displayName": `"FM Probe"`, "givenName": `"FM"`, "surname": `"Probe"`, "companyName": `"Frostyard"`, "jobTitle": `"Tester"`,
		"emailAddresses": `[{"address":"fm.probe@example.com"}]`, "mobilePhone": `"+1 555 0100"`,
		"businessPhones": `["+1 555 0101"]`, "homePhones": `["+1 555 0102"]`, "birthday": `"1980-05-17T11:59:00Z"`,
		"homeAddress":     `{"city":"Springfield","countryOrRegion":"United States","postalCode":"97477","state":"OR","street":"1 Main St"}`,
		"businessAddress": `{}`, "personalNotes": `"Likes tea."`,
	}
	for _, k := range slices.Sorted(maps.Keys(want)) {
		if got := asJSON(t, p.Fields[k]); got != want[k] {
			t.Errorf("%s = %s, want %s", k, got, want[k])
		}
	}
	if string(p.Photo) != "\xff\xd8\xff\xe0jpeg" || p.PhotoType != "image/jpeg" {
		t.Errorf("photo = %q %q", p.Photo, p.PhotoType)
	}
	edited := strings.Replace(string(src), "NOTE:Likes tea.", "NOTE:Likes coffee.", 1)
	q, err := ProjectContact([]byte(edited))
	if err != nil {
		t.Fatal(err)
	}
	if d := Diff(p.Fields, q.Fields); asJSON(t, d) != `{"personalNotes":"Likes coffee."}` {
		t.Errorf("diff = %s", asJSON(t, d))
	}
	if _, err := ProjectContact([]byte(strings.Replace(edited, "BDAY:19800517", "BDAY:--0517", 1))); !errors.Is(err, ErrUnsupported) {
		t.Errorf("a birthday without its year: %v", err)
	}
}
