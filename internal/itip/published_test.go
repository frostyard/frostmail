package itip_test

import (
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/itip"
)

// A published series with an override: each event names an organizer and
// attendees, one on a folded line, and has an alarm.
const published = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nMETHOD:PUBLISH\r\n" +
	"BEGIN:VEVENT\r\nUID:course@school.example\r\nDTSTAMP:20261001T080000Z\r\nDTSTART:20261005T170000Z\r\n" +
	"DTEND:20261005T180000Z\r\nRRULE:FREQ=WEEKLY;COUNT=4\r\nSUMMARY:Evening course\r\n" +
	"ORGANIZER;CN=School:mailto:office@school.example\r\n" +
	"ATTENDEE;CN=A very long name that makes this line fold over the limit of sevent\r\n y-five octets;PARTSTAT=ACCEPTED:mailto:user@dav.test\r\n" +
	"ATTENDEE:mailto:other@school.example\r\n" +
	"BEGIN:VALARM\r\nACTION:EMAIL\r\nTRIGGER:-PT1H\r\nSUMMARY:Course\r\nDESCRIPTION:Course\r\nATTENDEE:mailto:list@school.example\r\nEND:VALARM\r\n" +
	"LOCATION:Room 2\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:course@school.example\r\nRECURRENCE-ID:20261012T170000Z\r\nDTSTAMP:20261001T080000Z\r\n" +
	"DTSTART:20261012T180000Z\r\nDTEND:20261012T190000Z\r\nSUMMARY:Evening course (late)\r\n" +
	"ORGANIZER;CN=School:mailto:office@school.example\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT10M\r\nDESCRIPTION:Course\r\nEND:VALARM\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

func TestPublishedIsTheUsersOwnEvent(t *testing.T) {
	out, err := itip.Published([]byte(published))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, gone := range []string{"METHOD", "ORGANIZER", "ATTENDEE", "VALARM", "TRIGGER", "ACTION", "y-five octets", "school.example\r\nATT"} {
		if strings.Contains(got, gone) {
			t.Errorf("the copy has %q:\n%s", gone, got)
		}
	}
	want := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:course@school.example\r\nDTSTAMP:20261001T080000Z\r\nDTSTART:20261005T170000Z\r\n" +
		"DTEND:20261005T180000Z\r\nRRULE:FREQ=WEEKLY;COUNT=4\r\nSUMMARY:Evening course\r\n" +
		"LOCATION:Room 2\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:course@school.example\r\nRECURRENCE-ID:20261012T170000Z\r\nDTSTAMP:20261001T080000Z\r\n" +
		"DTSTART:20261012T180000Z\r\nDTEND:20261012T190000Z\r\nSUMMARY:Evening course (late)\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	if got != want {
		t.Errorf("the copy =\n%q\nwant\n%q", got, want)
	}
	events, err := calendar.Parse(out, calendar.Options{Local: time.UTC, UserEmails: []string{"user@dav.test"}})
	if err != nil || len(events) != 2 {
		t.Fatalf("the copy parses as %d events, %v", len(events), err)
	}
	for _, e := range events {
		if e.Organizer != nil || len(e.Attendees) != 0 || len(e.Alarms) != 0 || e.PartStat != "" {
			t.Errorf("event %q keeps people or alarms: %+v", e.RecurrenceID, e)
		}
	}
}

// An event with nothing to remove comes back as it is.
func TestPublishedLeavesAPlainEventAlone(t *testing.T) {
	src := []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:a\r\nDTSTART:20261005T170000Z\r\nSUMMARY:Plain\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	out, err := itip.Published(src)
	if err != nil || string(out) != string(src) {
		t.Errorf("out = %q, %v", out, err)
	}
	if _, err := itip.Published([]byte("not a calendar")); err == nil {
		t.Error("text that is no calendar was accepted")
	}
}
