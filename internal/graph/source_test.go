package graph

import (
	"bytes"
	"encoding/json/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/vcardx"
)

// The fixtures have the shapes Graph sent the probe of the user's
// Outlook.com account (plan 0010, Phase 1): times in UTC as asked, zones
// as written (IANA or Windows), occurrence IDs ending in the day.

func event(t *testing.T, src string) Event {
	t.Helper()
	var e Event
	if err := json.Unmarshal([]byte(src), &e); err != nil {
		t.Fatal(err)
	}
	return e
}

func parse(t *testing.T, src []byte) []calendar.Event {
	t.Helper()
	events, err := calendar.Parse(src, calendar.Options{UserEmails: []string{"cy@hotmail.com"}})
	if err != nil {
		t.Fatalf("the derived source does not parse: %v\n%s", err, src)
	}
	return events
}

func starts(t *testing.T, events []calendar.Event, from, to string) []string {
	t.Helper()
	f, _ := time.Parse(time.RFC3339, from)
	u, _ := time.Parse(time.RFC3339, to)
	occ, err := calendar.Expand(events, f, u)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, o := range occ {
		out = append(out, o.Start.UTC().Format(time.RFC3339))
	}
	return out
}

const single = `{
	"id": "AAMk-single", "@odata.etag": "W/\"ck-1\"", "changeKey": "ck-1",
	"iCalUId": "040000008200E00074C5B7101A82E008-single", "type": "singleInstance",
	"subject": "FM probe single", "body": {"contentType": "html", "content": "<html><body><p>Bring the <b>notes</b>.</p></body></html>"},
	"start": {"dateTime": "2026-10-27T14:00:00.0000000", "timeZone": "UTC"},
	"end": {"dateTime": "2026-10-27T15:00:00.0000000", "timeZone": "UTC"},
	"originalStartTimeZone": "America/New_York", "originalEndTimeZone": "America/New_York",
	"isAllDay": false, "isCancelled": false, "isOrganizer": true, "isReminderOn": true, "reminderMinutesBeforeStart": 10,
	"showAs": "busy", "sensitivity": "normal", "categories": ["Work"], "location": {"displayName": "Room 4"},
	"organizer": {"emailAddress": {"name": "Cy", "address": "cy@hotmail.com"}}, "attendees": [],
	"responseStatus": {"response": "organizer"},
	"createdDateTime": "2026-10-16T00:16:20.1234567Z", "lastModifiedDateTime": "2026-10-16T00:16:30.7654321Z"
}`

func TestSingleEvent(t *testing.T) {
	src, err := EventSource(event(t, single), nil, "cy@hotmail.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"UID:040000008200E00074C5B7101A82E008-single\r\n",
		"DTSTART;TZID=America/New_York:20261027T100000\r\n", "DTEND;TZID=America/New_York:20261027T110000\r\n",
		"SUMMARY:FM probe single\r\n", "LOCATION:Room 4\r\n", "DESCRIPTION:Bring the notes.\r\n",
		"CATEGORIES:Work\r\n", "TRANSP:OPAQUE\r\n", "TRIGGER:-PT10M\r\n", "BEGIN:VTIMEZONE\r\nTZID:America/New_York\r\n",
		"LAST-MODIFIED:20261016T001630Z\r\n", "CREATED:20261016T001620Z\r\n",
	} {
		if !strings.Contains(string(src), line) {
			t.Errorf("no %q in\n%s", line, src)
		}
	}
	if strings.Contains(string(src), "ORGANIZER") {
		t.Errorf("an event without invitees has an ORGANIZER:\n%s", src)
	}
	events := parse(t, src)
	if len(events) != 1 || events[0].Summary != "FM probe single" || events[0].TZID != "America/New_York" ||
		!events[0].Start.Equal(time.Date(2026, 10, 27, 14, 0, 0, 0, time.UTC)) || len(events[0].Alarms) != 1 ||
		events[0].Alarms[0].Offset != -10*time.Minute {
		t.Errorf("parsed = %+v", events)
	}
	again, _ := EventSource(event(t, single), nil, "cy@hotmail.com")
	if !bytes.Equal(src, again) {
		t.Error("the same JSON derived two sources")
	}
}

func TestAllDayEvent(t *testing.T) {
	e := event(t, `{"id": "AAMk-day", "iCalUId": "day", "type": "singleInstance", "subject": "FM probe all day",
		"start": {"dateTime": "2026-10-28T00:00:00.0000000", "timeZone": "UTC"},
		"end": {"dateTime": "2026-10-29T00:00:00.0000000", "timeZone": "UTC"},
		"originalStartTimeZone": "America/New_York", "isAllDay": true, "showAs": "free"}`)
	src, err := EventSource(e, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "DTSTART;VALUE=DATE:20261028\r\nDTEND;VALUE=DATE:20261029\r\n") ||
		strings.Contains(string(src), "VTIMEZONE") || !strings.Contains(string(src), "TRANSP:TRANSPARENT") {
		t.Errorf("all-day source:\n%s", src)
	}
	if ev := parse(t, src); len(ev) != 1 || !ev[0].AllDay || !ev[0].Transparent {
		t.Errorf("parsed = %+v", ev)
	}
}

// The probe's daily series of five in Eastern Standard Time: the second
// occurrence moved half an hour, the third deleted.
const series = `{
	"id": "AAMk-master", "@odata.etag": "W/\"ck-m\"", "iCalUId": "series-uid", "type": "seriesMaster",
	"subject": "FM probe series, renamed",
	"start": {"dateTime": "2026-10-26T13:30:00.0000000", "timeZone": "UTC"},
	"end": {"dateTime": "2026-10-26T13:45:00.0000000", "timeZone": "UTC"},
	"originalStartTimeZone": "Eastern Standard Time", "originalEndTimeZone": "Eastern Standard Time",
	"recurrence": {"pattern": {"type": "daily", "interval": 1, "month": 0, "dayOfMonth": 0, "firstDayOfWeek": "sunday", "index": "first"},
		"range": {"type": "numbered", "startDate": "2026-10-26", "endDate": "0001-01-01", "recurrenceTimeZone": "Eastern Standard Time", "numberOfOccurrences": 5}},
	"cancelledOccurrences": ["OID.AAMk-master.2026-10-28"]
}`

const exception = `{
	"id": "AAMk-exception", "iCalUId": "series-uid", "type": "exception", "seriesMasterId": "AAMk-master",
	"occurrenceId": "OID.AAMk-master.2026-10-27", "subject": "FM probe series, renamed",
	"start": {"dateTime": "2026-10-27T14:00:00.0000000", "timeZone": "UTC"},
	"end": {"dateTime": "2026-10-27T14:15:00.0000000", "timeZone": "UTC"},
	"originalStartTimeZone": "Eastern Standard Time", "originalEndTimeZone": "Eastern Standard Time"
}`

func TestSeriesWithAnExceptionAndACancellation(t *testing.T) {
	src, err := EventSource(event(t, series), []Event{event(t, exception)}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"DTSTART;TZID=America/New_York:20261026T093000\r\n", "RRULE:FREQ=DAILY;COUNT=5\r\n",
		"EXDATE;TZID=America/New_York:20261028T093000\r\n", "RECURRENCE-ID;TZID=America/New_York:20261027T093000\r\n",
		"DTSTART;TZID=America/New_York:20261027T100000\r\n",
	} {
		if !strings.Contains(string(src), line) {
			t.Errorf("no %q in\n%s", line, src)
		}
	}
	got := starts(t, parse(t, src), "2026-10-25T00:00:00Z", "2026-11-02T00:00:00Z")
	want := []string{"2026-10-26T13:30:00Z", "2026-10-27T14:00:00Z", "2026-10-29T13:30:00Z", "2026-10-30T13:30:00Z"}
	if !slices.Equal(got, want) {
		t.Errorf("occurrences = %v, want %v", got, want)
	}
}

func TestRecurrencePatterns(t *testing.T) {
	for _, c := range []struct {
		name, recurrence, start, end string
		allDay                       bool
		rule                         string
		from, to                     string
		want                         []string
	}{{
		name:       "weekly on Monday across the end of daylight time",
		recurrence: `{"pattern": {"type": "weekly", "interval": 1, "daysOfWeek": ["monday"], "firstDayOfWeek": "sunday"}, "range": {"type": "numbered", "startDate": "2026-10-26", "recurrenceTimeZone": "Eastern Standard Time", "numberOfOccurrences": 3}}`,
		start:      "2026-10-26T13:00:00", end: "2026-10-26T13:30:00",
		rule: "FREQ=WEEKLY;BYDAY=MO;WKST=SU;COUNT=3", from: "2026-10-01T00:00:00Z", to: "2026-12-01T00:00:00Z",
		want: []string{"2026-10-26T13:00:00Z", "2026-11-02T14:00:00Z", "2026-11-09T14:00:00Z"},
	}, {
		name:       "every second Tuesday, every other month, until a date",
		recurrence: `{"pattern": {"type": "relativeMonthly", "interval": 2, "daysOfWeek": ["tuesday"], "index": "second"}, "range": {"type": "endDate", "startDate": "2026-11-10", "endDate": "2027-03-09", "recurrenceTimeZone": "Eastern Standard Time"}}`,
		start:      "2026-11-10T15:00:00", end: "2026-11-10T16:00:00",
		rule: "FREQ=MONTHLY;INTERVAL=2;BYDAY=2TU;UNTIL=20270309T150000Z", from: "2026-11-01T00:00:00Z", to: "2027-06-01T00:00:00Z",
		want: []string{"2026-11-10T15:00:00Z", "2027-01-12T15:00:00Z", "2027-03-09T15:00:00Z"},
	}, {
		name:       "a birthday every year, all day",
		recurrence: `{"pattern": {"type": "absoluteYearly", "interval": 1, "month": 5, "dayOfMonth": 17}, "range": {"type": "noEnd", "startDate": "2027-05-17", "recurrenceTimeZone": "UTC"}}`,
		start:      "2027-05-17T00:00:00", end: "2027-05-18T00:00:00", allDay: true,
		rule: "FREQ=YEARLY;BYMONTH=5;BYMONTHDAY=17", from: "2027-01-01T00:00:00Z", to: "2029-01-01T00:00:00Z",
		want: []string{"2027-05-17T00:00:00Z", "2028-05-17T00:00:00Z"},
	}} {
		t.Run(c.name, func(t *testing.T) {
			e := event(t, `{"id": "m", "iCalUId": "u", "type": "seriesMaster", "subject": "x", "isAllDay": `+map[bool]string{true: "true", false: "false"}[c.allDay]+`,
				"start": {"dateTime": "`+c.start+`.0000000", "timeZone": "UTC"}, "end": {"dateTime": "`+c.end+`.0000000", "timeZone": "UTC"},
				"originalStartTimeZone": "Eastern Standard Time", "recurrence": `+c.recurrence+`}`)
			src, err := EventSource(e, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(src), "RRULE:"+c.rule+"\r\n") {
				t.Errorf("no RRULE:%s in\n%s", c.rule, src)
			}
			if got := starts(t, parse(t, src), c.from, c.to); !slices.Equal(got, c.want) {
				t.Errorf("occurrences = %v, want %v", got, c.want)
			}
		})
	}
}

func TestInvitation(t *testing.T) {
	e := event(t, `{"id": "inv", "iCalUId": "inv-uid", "type": "singleInstance", "subject": "Planning",
		"start": {"dateTime": "2026-11-03T15:00:00.0000000", "timeZone": "UTC"}, "end": {"dateTime": "2026-11-03T16:00:00.0000000", "timeZone": "UTC"},
		"originalStartTimeZone": "Pacific Standard Time", "isOrganizer": false,
		"organizer": {"emailAddress": {"name": "Bob", "address": "bob@example.com"}},
		"attendees": [
			{"type": "required", "status": {"response": "none"}, "emailAddress": {"name": "Cy", "address": "Cy@Hotmail.com"}},
			{"type": "optional", "status": {"response": "accepted"}, "emailAddress": {"name": "Dee", "address": "dee@example.com"}},
			{"type": "resource", "status": {"response": "none"}, "emailAddress": {"name": "Room 4", "address": "room4@example.com"}}],
		"responseStatus": {"response": "tentativelyAccepted"}}`)
	src, err := EventSource(e, nil, "cy@hotmail.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "DTSTART;TZID=America/Los_Angeles:20261103T070000") {
		t.Errorf("the Windows zone was not mapped:\n%s", src)
	}
	ev := parse(t, src)
	if len(ev) != 1 || ev[0].Organizer == nil || ev[0].Organizer.Email != "bob@example.com" || ev[0].PartStat != "tentative" ||
		len(ev[0].Attendees) != 3 || ev[0].Attendees[1].Role != "optional" || ev[0].Attendees[1].PartStat != "accepted" {
		t.Errorf("parsed = %+v", ev)
	}
}

func TestContact(t *testing.T) {
	var c Contact
	if err := json.Unmarshal([]byte(`{
		"id": "AAMk-contact", "@odata.etag": "W/\"ck-c\"", "displayName": "FM Probe", "givenName": "FM", "surname": "Probe",
		"companyName": "Frostyard", "department": "Mail", "jobTitle": "Tester", "nickName": "Probie",
		"emailAddresses": [{"name": "FM Probe", "address": "fm.probe@example.com"}, {"address": "probe@example.org"}],
		"businessPhones": ["+1 555 0101"], "homePhones": [], "mobilePhone": "+1 555 0100",
		"homeAddress": {"street": "1 Main St", "city": "Springfield", "state": "OR", "postalCode": "97477", "countryOrRegion": "United States"},
		"businessAddress": {}, "birthday": "1980-05-17T11:59:00Z", "personalNotes": "Met at the conference.\r\nLikes tea.",
		"categories": ["Friends"], "lastModifiedDateTime": "2026-10-16T00:16:52Z"}`), &c); err != nil {
		t.Fatal(err)
	}
	src := ContactSource(c, []byte("\xff\xd8\xff\xe0jpeg"), "image/jpeg")
	card, err := vcardx.Parse(src)
	if err != nil {
		t.Fatalf("the derived vCard does not parse: %v\n%s", err, src)
	}
	if card.DisplayName() != "FM Probe" || card.Organization != "Frostyard" || card.Title != "Tester" ||
		len(card.Emails) != 2 || card.Emails[0].Value != "fm.probe@example.com" || len(card.Photo.Data) == 0 {
		t.Errorf("card = %+v\n%s", card, src)
	}
	var labels []string
	for _, p := range card.Phones {
		labels = append(labels, p.Label+" "+p.Value)
	}
	if !slices.Equal(labels, []string{"mobile +1 555 0100", "work +1 555 0101"}) {
		t.Errorf("phones = %v", labels)
	}
	for _, line := range []string{"BDAY:19800517\r\n", "NOTE:Met at the conference.\\nLikes tea.\r\n", "ADR;TYPE=home:;;1 Main St;Springfield;OR;97477;United States\r\n"} {
		if !strings.Contains(string(src), line) {
			t.Errorf("no %q in\n%s", line, src)
		}
	}
	if strings.Contains(string(src), "TYPE=work:;;;") {
		t.Errorf("an empty business address was written:\n%s", src)
	}
}
