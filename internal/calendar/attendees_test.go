package calendar

import (
	"strings"
	"testing"
	"time"
)

var bea = &Attendee{Email: "bea@example.com", Name: "Bea Jones"}

func invite(list ...Attendee) EventChange {
	return EventChange{Attendees: &list, Organizer: bea}
}

// vevent is the lines of src's VEVENTs, alarms left out, so a test can
// read what changed.
func vevent(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	inAlarm := false
	for _, line := range strings.Split(strings.ReplaceAll(src, "\r\n ", ""), "\r\n") {
		switch {
		case line == "BEGIN:VALARM":
			inAlarm = true
		case line == "END:VALARM":
			inAlarm = false
		case !inAlarm && (strings.HasPrefix(line, "ORGANIZER") || strings.HasPrefix(line, "ATTENDEE") ||
			strings.HasPrefix(line, "SEQUENCE") || strings.HasPrefix(line, "RECURRENCE-ID") || line == "BEGIN:VEVENT"):
			out = append(out, line)
		}
	}
	return out
}

func sameLines(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// An event gets its first invitees, and with them its organizer, before
// its alarms; the server's own EMAIL alarm keeps its ATTENDEE, and
// SEQUENCE stays, since the invitees are not a significant change.
func TestFirstInviteesBringTheOrganizer(t *testing.T) {
	out := patchedEvent(t, googleEvent, invite(Attendee{Email: "Carl@Example.com", Name: "Carl Diaz"},
		Attendee{Email: "dana@example.com", Role: "optional"}))
	want := strings.Replace(googleEvent, "X-GOOGLE-CONFERENCE:https://meet.google.com/abc-defg-hij\r\n",
		"X-GOOGLE-CONFERENCE:https://meet.google.com/abc-defg-hij\r\n"+
			"ORGANIZER;CN=Bea Jones:mailto:bea@example.com\r\n"+
			"ATTENDEE;CN=Carl Diaz;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:\r\n mailto:carl@example.com\r\n"+
			"ATTENDEE;ROLE=OPT-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:dana@e\r\n xample.com\r\n", 1)
	want = strings.Replace(want, "DTSTAMP:20261010T120000Z", "DTSTAMP:"+nowStamp, 1)
	want = strings.Replace(want, "LAST-MODIFIED:20261010T120000Z", "LAST-MODIFIED:"+nowStamp, 1)
	if out != want {
		t.Errorf("patched:\n%s\nwant:\n%s", out, want)
	}
	evs, err := Parse([]byte(out), Options{Local: time.UTC, UserEmails: []string{"bea@example.com"}})
	if err != nil || len(evs) != 1 || evs[0].Organizer == nil || evs[0].Organizer.Email != "bea@example.com" ||
		len(evs[0].Attendees) != 2 || evs[0].Attendees[1].Role != "optional" {
		t.Errorf("read back: %+v, %v", evs, err)
	}
}

// googleMeeting is an organizer copy as Google keeps it after its own
// scheduling: the organizer, and invitees with Google's parameters, one
// of whom accepted.
var googleMeeting = ev(
	"BEGIN:VCALENDAR", "PRODID:-//Google Inc//Google Calendar 70.9054//EN", "VERSION:2.0",
	"BEGIN:VEVENT", "DTSTART:20261027T150000Z", "DTEND:20261027T160000Z", "DTSTAMP:20261014T003019Z",
	"ORGANIZER;CN=bea@example.com:mailto:bea@example.com", "UID:fm-1",
	"ATTENDEE;CUTYPE=INDIVIDUAL;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;CN=carl@example.com;X-NUM-GUESTS=0:mailto:carl@example.com",
	"ATTENDEE;CUTYPE=INDIVIDUAL;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;CN=dana@example.com;X-NUM-GUESTS=0:mailto:dana@example.com",
	"CREATED:20261014T003019Z", "LAST-MODIFIED:20261014T003019Z", "SEQUENCE:0", "STATUS:CONFIRMED",
	"SUMMARY:Planning", "END:VEVENT", "END:VCALENDAR",
)

// An invitee who stays keeps their line and answer, only ROLE following
// the change; one removed loses their line; one added comes after the
// last invitee.
func TestInviteesKeepTheirLines(t *testing.T) {
	out := patchedEvent(t, googleMeeting, invite(Attendee{Email: "carl@example.com", Role: "optional"},
		Attendee{Email: "erin@example.com", Name: "Erin"}))
	sameLines(t, vevent(t, out), []string{
		"BEGIN:VEVENT",
		"ORGANIZER;CN=bea@example.com:mailto:bea@example.com",
		"ATTENDEE;CUTYPE=INDIVIDUAL;ROLE=OPT-PARTICIPANT;PARTSTAT=ACCEPTED;CN=carl@example.com;X-NUM-GUESTS=0:mailto:carl@example.com",
		"ATTENDEE;CN=Erin;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:erin@example.com",
		"SEQUENCE:0",
	})
	if !strings.Contains(out, "SUMMARY:Planning\r\n") || !strings.Contains(out, "DTSTAMP:"+nowStamp) {
		t.Errorf("the rest of the event changed:\n%s", out)
	}
}

// The same invitees in the same roles change nothing.
func TestSameInviteesChangeNothing(t *testing.T) {
	c := invite(Attendee{Email: "dana@example.com"}, Attendee{Email: "carl@example.com", Name: "Carl"})
	out, err := PatchEvent([]byte(googleMeeting), c, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != googleMeeting {
		t.Errorf("patched:\n%s", out)
	}
}

// Removing every invitee removes their lines and keeps the organizer.
func TestNoInviteesLeft(t *testing.T) {
	out := patchedEvent(t, googleMeeting, invite())
	sameLines(t, vevent(t, out), []string{
		"BEGIN:VEVENT", "ORGANIZER;CN=bea@example.com:mailto:bea@example.com", "SEQUENCE:0",
	})
}

// icloudMeeting is an organizer copy as iCloud keeps it: the organizer
// and its own ATTENDEE line as the account's principal, with EMAIL.
var icloudMeeting = ev(
	"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Frostyard//Frostmail//EN",
	"BEGIN:VEVENT", "UID:fm-2", "CREATED:20261015T003019Z", "LAST-MODIFIED:20261015T003019Z", "SEQUENCE:0",
	"DTSTART:20261028T150000Z", "DTEND:20261028T160000Z", "SUMMARY:Review",
	"ORGANIZER;CN=Bea Jones;EMAIL=bea@example.com:/aNTM/principal/",
	"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE;EMAIL=bea@example.com:/aNTM/principal/",
	"DTSTAMP:20261015T003021Z",
	"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;SCHEDULE-STATUS=1.1:mailto:carl@example.com",
	"END:VEVENT", "END:VCALENDAR",
)

// The organizer's own line is not an invitee: it stays when the
// invitees change.
func TestTheOrganizersOwnLineStays(t *testing.T) {
	out := patchedEvent(t, icloudMeeting, invite(Attendee{Email: "dana@example.com"}))
	sameLines(t, vevent(t, out), []string{
		"BEGIN:VEVENT", "SEQUENCE:0",
		"ORGANIZER;CN=Bea Jones;EMAIL=bea@example.com:/aNTM/principal/",
		"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE;EMAIL=bea@example.com:/aNTM/principal/",
		"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:dana@example.com",
	})
}

func TestNewEventWithInvitees(t *testing.T) {
	start, end, zone := time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC), time.Date(2026, 10, 27, 16, 0, 0, 0, time.UTC), "UTC"
	c := invite(Attendee{Email: "carl@example.com", Name: "Carl Diaz"}, Attendee{Email: "carl@example.com"})
	c.AllDay, c.Start, c.End, c.Zone, c.Summary = sp(false), &start, &end, &zone, sp("Planning")
	out, err := NewEvent("fm-3", c, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	sameLines(t, vevent(t, string(out)), []string{
		"BEGIN:VEVENT", "SEQUENCE:0",
		"ORGANIZER;CN=Bea Jones:mailto:bea@example.com",
		"ATTENDEE;CN=Carl Diaz;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:carl@example.com",
	})
	c.Organizer = nil
	if _, err := NewEvent("fm-3", c, eventNow); err == nil {
		t.Error("invitees without an organizer were written")
	}
	empty := []Attendee{}
	c.Attendees = &empty
	if out, err := NewEvent("fm-3", c, eventNow); err != nil || strings.Contains(string(out), "ORGANIZER") {
		t.Errorf("no invitees: %v\n%s", err, out)
	}
}

func TestInviteesAreAddresses(t *testing.T) {
	for _, list := range [][]Attendee{
		{{Email: ""}}, {{Email: "carl"}}, {{Email: "carl@example.com, dana@example.com"}},
		{{Email: "carl@example.com", Role: "boss"}},
	} {
		if _, err := PatchEvent([]byte(googleMeeting), invite(list...), eventNow); err == nil {
			t.Errorf("%+v was written", list)
		}
	}
	if _, err := PatchEvent([]byte(googleEvent), EventChange{Attendees: &[]Attendee{{Email: "carl@example.com"}}}, eventNow); err == nil {
		t.Error("invitees without an organizer were written")
	}
}

// meetingSeries is a daily series with invitees and two overrides: one
// moved, which keeps the master's invitees, and one with an invitee of
// its own.
var meetingSeries = ev(
	"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//test//EN",
	"BEGIN:VEVENT", "UID:fm-s", "DTSTAMP:20261014T000000Z", "SEQUENCE:0",
	"DTSTART:20261019T150000Z", "DTEND:20261019T153000Z", "RRULE:FREQ=DAILY;COUNT=5", "SUMMARY:Standup",
	"ORGANIZER:mailto:bea@example.com",
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com",
	"END:VEVENT",
	"BEGIN:VEVENT", "UID:fm-s", "DTSTAMP:20261014T000000Z", "SEQUENCE:0", "RECURRENCE-ID:20261020T150000Z",
	"DTSTART:20261020T160000Z", "DTEND:20261020T163000Z", "SUMMARY:Standup",
	"ORGANIZER:mailto:bea@example.com",
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com",
	"END:VEVENT",
	"BEGIN:VEVENT", "UID:fm-s", "DTSTAMP:20261014T000000Z", "SEQUENCE:0", "RECURRENCE-ID:20261021T150000Z",
	"DTSTART:20261021T150000Z", "DTEND:20261021T153000Z", "SUMMARY:Standup",
	"ORGANIZER:mailto:bea@example.com",
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com",
	"ATTENDEE:mailto:guest@example.com",
	"END:VEVENT", "END:VCALENDAR",
)

func TestInviteesInEachScope(t *testing.T) {
	c := invite(Attendee{Email: "carl@example.com"}, Attendee{Email: "dana@example.com"})
	dana := "ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:dana@example.com"

	this, _, err := PatchOccurrence([]byte(meetingSeries), "2026-10-22T15:00:00.000Z", ScopeThis, c, "", eventNow)
	if err != nil {
		t.Fatal(err)
	}
	// The new override follows the master, as This Event writes it.
	sameLines(t, vevent(t, string(this)), []string{
		"BEGIN:VEVENT", "SEQUENCE:0", "ORGANIZER:mailto:bea@example.com", "ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com",
		"BEGIN:VEVENT", "SEQUENCE:0", "RECURRENCE-ID:20261022T150000Z", "ORGANIZER:mailto:bea@example.com",
		"ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com", dana,
		"BEGIN:VEVENT", "SEQUENCE:0", "RECURRENCE-ID:20261020T150000Z", "ORGANIZER:mailto:bea@example.com",
		"ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com",
		"BEGIN:VEVENT", "SEQUENCE:0", "RECURRENCE-ID:20261021T150000Z", "ORGANIZER:mailto:bea@example.com",
		"ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com", "ATTENDEE:mailto:guest@example.com",
	})

	all, _, err := PatchOccurrence([]byte(meetingSeries), "2026-10-22T15:00:00.000Z", ScopeAll, c, "", eventNow)
	if err != nil {
		t.Fatal(err)
	}
	sameLines(t, vevent(t, string(all)), []string{
		"BEGIN:VEVENT", "SEQUENCE:0", "ORGANIZER:mailto:bea@example.com", "ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com", dana,
		"BEGIN:VEVENT", "SEQUENCE:0", "RECURRENCE-ID:20261020T150000Z", "ORGANIZER:mailto:bea@example.com",
		"ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com", dana,
		"BEGIN:VEVENT", "SEQUENCE:0", "RECURRENCE-ID:20261021T150000Z", "ORGANIZER:mailto:bea@example.com",
		"ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com", "ATTENDEE:mailto:guest@example.com",
	})

	old, split, err := PatchOccurrence([]byte(meetingSeries), "2026-10-22T15:00:00.000Z", ScopeFuture, c, "fm-s2", eventNow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(old), "dana@") {
		t.Errorf("the old series got the new invitee:\n%s", old)
	}
	sameLines(t, vevent(t, string(split)), []string{
		"BEGIN:VEVENT", "SEQUENCE:0", "ORGANIZER:mailto:bea@example.com", "ATTENDEE;PARTSTAT=ACCEPTED:mailto:carl@example.com", dana,
	})
}
