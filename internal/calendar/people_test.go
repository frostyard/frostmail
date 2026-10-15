package calendar

import (
	"reflect"
	"testing"
)

// iCloud writes the account's own addresses as its principal URL, with
// the address in EMAIL (seen on the user's account, 2026-10-14): the user
// still organizes the event.
func TestPrincipalPeopleUseTheirEmail(t *testing.T) {
	ev := parseOne(t, ics("BEGIN:VEVENT", "UID:p", "DTSTART:20261028T150000Z",
		"ORGANIZER;CN=Ann Smith;EMAIL=ann@example.com:/aNTM/principal/",
		"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE;EMAIL=ann@example.com:/aNTM/principal/",
		"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;SCHEDULE-STATUS=1.1:mailto:Bob@Example.com",
		"ATTENDEE;EMAIL=carl@example.com:mailto:carl@other.example",
		"END:VEVENT"), me)
	if ev.Organizer == nil || ev.Organizer.Email != "ann@example.com" || ev.Organizer.Name != "Ann Smith" {
		t.Fatalf("organizer = %+v", ev.Organizer)
	}
	if ev.PartStat != "" {
		t.Errorf("the user organizes the event, so has no answer: %q", ev.PartStat)
	}
	want := []Attendee{
		{Email: "bob@example.com", Role: "required", PartStat: "accepted"},
		{Email: "carl@other.example", Role: "required", PartStat: "needsaction"},
	}
	if !reflect.DeepEqual(ev.Attendees, want) {
		t.Errorf("attendees = %+v\nwant %+v", ev.Attendees, want)
	}
}
