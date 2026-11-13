package main

import (
	"encoding/json"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestCalSearch(t *testing.T) {
	srv, _ := pimFixture(t)
	got := mustRun(t, "--socket", srv.Socket, "cal", "search", "--zone", "UTC", "off", "SITE")
	if want := "2026-10-16 Fri\n  10:00–11:00  Offsite [cancelled]\n"; got != want {
		t.Errorf("cal search =\n%s\nwant\n%s", got, want)
	}
	if got := mustRun(t, "--socket", srv.Socket, "cal", "search", "--zone", "UTC", "holiday"); got != "2026-10-16 Fri\n  all day      Holiday\n" {
		t.Errorf("an all-day result =\n%s", got)
	}
	if got := mustRun(t, "--socket", srv.Socket, "cal", "search", "zeppelin"); got != "no events\n" {
		t.Errorf("no result = %q", got)
	}
	var rows []api.Occurrence
	js := mustRun(t, "--socket", srv.Socket, "--json", "cal", "search", "--limit", "1", "room")
	if err := json.Unmarshal([]byte(js), &rows); err != nil || len(rows) != 1 || rows[0].Summary != "Standup" {
		t.Errorf("--json cal search --limit 1 = %s (%v)", js, err)
	}
	for _, bad := range [][]string{{}, {"--zone", "Mars/Olympus", "x"}, {"--limit", "0", "x"}, {"--limit", "501", "x"}} {
		if _, err := runMailctl(t, append([]string{"--socket", srv.Socket, "cal", "search"}, bad...)...); err == nil {
			t.Errorf("cal search %q succeeded", bad)
		}
	}
}
