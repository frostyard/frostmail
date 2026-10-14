package main

// CONTRACT TEST for task card T-0145 (docs/tasks). Do not edit.

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestCalWriteCommands(t *testing.T) {
	srv, ids := pimFixture(t)
	run := func(args ...string) string { return mustRun(t, append([]string{"--socket", srv.Socket}, args...)...) }
	fails := func(args ...string) {
		t.Helper()
		if out, err := runMailctl(t, append([]string{"--socket", srv.Socket}, args...)...); err == nil {
			t.Errorf("mailctl %q succeeded: %s", args, out)
		}
	}
	agenda := func(from string) string {
		return run("cal", "agenda", "--from", from, "--days", "1", "--zone", "UTC", "--ids")
	}

	added := run("cal", "add", "--summary", "Planning", "--start", "2026-10-20 15:00", "--end", "2026-10-20 16:30",
		"--zone", "UTC", "--location", "Room 4", "--alert", "15")
	m := regexp.MustCompile(`^added Planning \(event (\d+)\)\n$`).FindStringSubmatch(added)
	if m == nil {
		t.Fatalf("cal add = %q", added)
	}
	if got := agenda("2026-10-20"); got != "2026-10-20 Tue\n  15:00–16:30  Planning (Room 4)  ["+m[1]+"]\n" {
		t.Errorf("agenda after cal add =\n%s", got)
	}

	// An hour long by default, in the zone given.
	run("cal", "add", "--summary", "Call", "--start", "2026-10-21T09:00", "--zone", "Europe/Berlin")
	if got := agenda("2026-10-21"); !strings.Contains(got, "  07:00–08:00  Call  [") {
		t.Errorf("agenda with the Berlin call =\n%s", got)
	}

	// All day, the last day included.
	trip := run("cal", "add", "--summary", "Trip", "--date", "2026-10-22", "--end-date", "2026-10-23")
	if !strings.HasPrefix(trip, "added Trip (event ") {
		t.Errorf("cal add --date = %q", trip)
	}
	for _, day := range []string{"2026-10-22", "2026-10-23"} {
		if got := agenda(day); !strings.Contains(got, "  all day      Trip  [") {
			t.Errorf("agenda on %s =\n%s", day, got)
		}
	}
	if got := agenda("2026-10-24"); strings.Contains(got, "Trip") {
		t.Errorf("the trip lasts into the 24th:\n%s", got)
	}

	// A repeating event, and one occurrence of it deleted.
	run("cal", "add", "--summary", "Sync", "--start", "2026-10-26 10:00", "--zone", "UTC", "--repeat", "FREQ=DAILY;COUNT=3")
	line := regexp.MustCompile(`Sync  \[(\d+) (\S+)\]`).FindStringSubmatch(agenda("2026-10-27"))
	if line == nil || line[2] != "2026-10-27T10:00:00.000Z" {
		t.Fatalf("agenda of the series = %q", line)
	}
	if got := run("cal", "rm", line[1], "--occurrence", line[2], "--scope", "this"); got != "deleted event "+line[1]+"\n" {
		t.Errorf("cal rm --occurrence = %q", got)
	}
	if got := agenda("2026-10-27"); strings.Contains(got, "Sync") {
		t.Errorf("the deleted occurrence is still there:\n%s", got)
	}
	if got := agenda("2026-10-28"); !strings.Contains(got, "Sync") {
		t.Errorf("the next occurrence is gone too:\n%s", got)
	}

	if got := run("cal", "rm", m[1]); got != "deleted event "+m[1]+"\n" {
		t.Errorf("cal rm = %q", got)
	}
	if got := run("cal", "agenda", "--from", "2026-10-20", "--days", "1", "--zone", "UTC"); got != "no events\n" {
		t.Errorf("after cal rm, agenda = %q", got)
	}

	var w api.EventWrite
	if err := json.Unmarshal([]byte(run("--json", "cal", "add", "--summary", "Lunch", "--calendar", itoa64(ids.work),
		"--date", "2026-10-30")), &w); err != nil || w.RestoreID == "" || w.Event == nil || w.Event.CalendarID != ids.work {
		t.Errorf("--json cal add = %+v, %v", w, err)
	}

	fails("cal", "add", "--start", "2026-10-20 15:00", "--zone", "UTC")                               // no summary
	fails("cal", "add", "--summary", "x")                                                             // no times
	fails("cal", "add", "--summary", "x", "--start", "2026-10-20 15:00", "--date", "2026-10-20")      // both
	fails("cal", "add", "--summary", "x", "--start", "tomorrow", "--zone", "UTC")                     // a bad time
	fails("cal", "add", "--summary", "x", "--start", "2026-10-20 15:00", "--end", "2026-10-20 14:00") // end first
	fails("cal", "add", "--summary", "x", "--start", "2026-10-20 15:00", "--zone", "Mars/Olympus")    // a bad zone
	fails("cal", "add", "--summary", "x", "--date", "2026-10-20", "--calendar", itoa64(ids.home))     // read-only
	fails("cal", "add", "--summary", "x", "--date", "2026-10-20", "--repeat", "FREQ=SOMETIMES")       // a bad rule
	fails("cal", "rm", "99999")
	fails("cal", "rm", "x")
	fails("cal", "rm", line[1], "--scope", "this")
}
