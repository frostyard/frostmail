package main

// CONTRACT TEST for task card T-0160 (docs/tasks). Do not edit.

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
)

// The fixture's account is a Gmail one, so its Work calendar takes
// invitees, and maild asks before telling them (ADR-0035).
func TestCalInvitees(t *testing.T) {
	srv, _ := pimFixture(t)
	args := func(more ...string) []string { return append([]string{"--socket", srv.Socket}, more...) }
	refused := func(want string, more ...string) {
		t.Helper()
		out, err := runMailctl(t, args(more...)...)
		if err == nil {
			t.Fatalf("mailctl %q succeeded: %s", more, out)
		}
		if msg := err.Error() + out; !strings.Contains(msg, want) || !strings.Contains(msg, "--notify") {
			t.Errorf("mailctl %q: %q, want %q and --notify", more, msg, want)
		}
	}
	add := []string{"cal", "add", "--summary", "Planning", "--start", "2026-10-20 15:00", "--zone", "UTC",
		"--invite", "Carl@Example.com", "--invite-optional", "dana@example.com"}

	refused("invitations to 2 invitees", add...)

	var w api.EventWrite
	if err := json.Unmarshal([]byte(mustRun(t, args(append([]string{"--json"}, append(add, "--notify")...)...)...)), &w); err != nil || w.Event == nil {
		t.Fatalf("cal add --notify = %+v, %v", w, err)
	}
	if w.Event.Organizer == nil || w.Event.Organizer.Email != "ann@gmail.example" {
		t.Errorf("organizer = %+v", w.Event.Organizer)
	}
	if len(w.Event.Attendees) != 2 || w.Event.Attendees[0].Email != "carl@example.com" ||
		w.Event.Attendees[0].Role != api.AttendeeRoleRequired || w.Event.Attendees[1].Role != api.AttendeeRoleOptional {
		t.Errorf("attendees = %+v", w.Event.Attendees)
	}
	id := itoa64(w.Event.ID)

	refused("cancellations to 2 invitees", "cal", "rm", id)
	if got := mustRun(t, args("cal", "rm", id, "--notify")...); got != "deleted event "+id+"\n" {
		t.Errorf("cal rm --notify = %q", got)
	}

	// Nobody invited: nothing to ask, and --notify does no harm.
	added := mustRun(t, args("cal", "add", "--summary", "Focus", "--start", "2026-10-21 09:00", "--zone", "UTC", "--notify")...)
	m := regexp.MustCompile(`^added Focus \(event (\d+)\)\n$`).FindStringSubmatch(added)
	if m == nil {
		t.Fatalf("cal add without invitees = %q", added)
	}
	mustRun(t, args("cal", "rm", m[1])...)

	if out, err := runMailctl(t, args("cal", "add", "--summary", "x", "--start", "2026-10-22 09:00", "--zone", "UTC",
		"--invite", "not an address", "--notify")...); err == nil {
		t.Errorf("a bad invitee was taken: %s", out)
	}
}
