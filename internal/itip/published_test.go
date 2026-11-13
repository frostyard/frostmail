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

// A file that publishes several events gives the one a card shows: the
// first without a RECURRENCE-ID, with the overrides of its UID. The other
// UIDs' events and what is no event go; the time zones stay
// (frostyard/frostmail#311: a calendar resource holds one UID).
func TestPublishedKeepsOneUID(t *testing.T) {
	zone := "BEGIN:VTIMEZONE\r\nTZID:Europe/Berlin\r\nBEGIN:STANDARD\r\nDTSTART:19701025T030000\r\nTZOFFSETFROM:+0200\r\nTZOFFSETTO:+0100\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\n"
	override := "BEGIN:VEVENT\r\nUID:first@x.example\r\nRECURRENCE-ID:20261012T170000Z\r\nDTSTART:20261012T180000Z\r\nSUMMARY:First (late)\r\nEND:VEVENT\r\n"
	first := "BEGIN:VEVENT\r\nUID:first@x.example\r\nDTSTART:20261005T170000Z\r\nRRULE:FREQ=WEEKLY;COUNT=4\r\nSUMMARY:First\r\nEND:VEVENT\r\n"
	second := "BEGIN:VEVENT\r\nUID:second@x.example\r\nDTSTART:20261006T170000Z\r\nSUMMARY:Second\r\nORGANIZER:mailto:o@x.example\r\nEND:VEVENT\r\n"
	todo := "BEGIN:VTODO\r\nUID:todo@x.example\r\nSUMMARY:A task\r\nEND:VTODO\r\n"
	head, tail := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nMETHOD:PUBLISH\r\n", "END:VCALENDAR\r\n"
	// The override comes before its master, with other components between
	// and after.
	src := head + zone + override + todo + first + second + tail
	out, err := itip.Published([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" + zone + override + first + tail
	if string(out) != want {
		t.Errorf("the copy =\n%q\nwant\n%q", out, want)
	}
	msg, err := itip.Parse([]byte(src), calendar.Options{Local: time.UTC})
	if err != nil || msg.Main().UID != "first@x.example" {
		t.Fatalf("the card's event = %q, %v", msg.Main().UID, err)
	}

	// With no master at all, the first event is the one, as on the card.
	stray := "BEGIN:VEVENT\r\nUID:stray@x.example\r\nRECURRENCE-ID:20261013T170000Z\r\nDTSTART:20261013T180000Z\r\nSUMMARY:Stray\r\nEND:VEVENT\r\n"
	out, err = itip.Published([]byte(head + override + stray + tail))
	if err != nil || string(out) != "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n"+override+tail {
		t.Errorf("without a master = %q, %v", out, err)
	}
	for name, bad := range map[string]string{
		"no event":             head + todo + tail,
		"an event with no UID": head + "BEGIN:VEVENT\r\nDTSTART:20261005T170000Z\r\nEND:VEVENT\r\n" + tail,
	} {
		if out, err := itip.Published([]byte(bad)); err == nil {
			t.Errorf("%s gave %q", name, out)
		}
	}
}

// A part that holds two calendar objects gives the first, sanitized, and
// nothing of the second or of what surrounds them, also when the first
// needs no change (frostyard/frostmail#311).
func TestPublishedIsOneCalendarObject(t *testing.T) {
	plain := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:a@x.example\r\nDTSTART:20261005T170000Z\r\nSUMMARY:Plain\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	second := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:PUBLISH\r\nBEGIN:VEVENT\r\nUID:b@x.example\r\nDTSTART:20261006T170000Z\r\nSUMMARY:Second\r\n" +
		"ORGANIZER:mailto:o@x.example\r\nATTENDEE:mailto:someone@else.example\r\n" +
		"BEGIN:VALARM\r\nACTION:EMAIL\r\nTRIGGER:-PT1H\r\nSUMMARY:S\r\nDESCRIPTION:D\r\nATTENDEE:mailto:list@else.example\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	for name, src := range map[string]string{
		"a second calendar after one that needs no change": plain + second,
		"three calendars": plain + second + second,
	} {
		out, err := itip.Published([]byte(src))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if string(out) != plain {
			t.Errorf("%s: the copy =\n%q\nwant\n%q", name, out, plain)
		}
	}
	// The first calendar is the one sanitized, when it is the one with work.
	out, err := itip.Published([]byte(second + plain))
	if err != nil {
		t.Fatal(err)
	}
	want := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:b@x.example\r\nDTSTART:20261006T170000Z\r\nSUMMARY:Second\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if string(out) != want {
		t.Errorf("the first of two =\n%q\nwant\n%q", out, want)
	}
	// Lines outside any calendar object are no calendar: nothing is made.
	if out, err := itip.Published([]byte("X-STRAY:before\r\n" + plain + second)); err == nil {
		t.Errorf("stray lines before the calendar gave %q", out)
	}
	// A lone calendar that needs no change still comes back as it is.
	if out, err := itip.Published([]byte(plain)); err != nil || string(out) != plain {
		t.Errorf("alone = %q, %v", out, err)
	}
}
