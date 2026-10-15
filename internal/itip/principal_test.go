package itip_test

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/itip"
)

// iCloud writes the account's own address as its principal URL, with the
// address in EMAIL: answering still finds the user's line.
func TestSetPartStatOnAPrincipal(t *testing.T) {
	src := []byte(strings.Join([]string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//test//EN", "BEGIN:VEVENT",
		"UID:p", "DTSTART:20261028T150000Z", "ORGANIZER:mailto:ann@example.com",
		"ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE;EMAIL=me@example.com:/aNTM/principal/",
		"END:VEVENT", "END:VCALENDAR", ""}, "\r\n"))
	out, err := itip.SetPartStat(src, "", "me@example.com", "accepted")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "ATTENDEE;PARTSTAT=ACCEPTED;EMAIL=me@example.com:/aNTM/principal/\r\n") {
		t.Errorf("answered:\n%s", out)
	}
}
