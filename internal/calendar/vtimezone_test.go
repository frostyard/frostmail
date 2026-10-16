package calendar

import (
	"archive/zip"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/teambition/rrule-go"
)

var vtzFrom = time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

func lines(ls ...string) string { return strings.Join(ls, "\r\n") + "\r\n" }

func TestVTimezoneGolden(t *testing.T) {
	for _, tc := range []struct {
		zone string
		want string
	}{
		{"Europe/Berlin", lines("BEGIN:VTIMEZONE", "TZID:Europe/Berlin", "X-LIC-LOCATION:Europe/Berlin",
			"BEGIN:DAYLIGHT", "DTSTART:20250330T020000", "TZOFFSETFROM:+0100", "TZOFFSETTO:+0200", "TZNAME:CEST",
			"RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU", "END:DAYLIGHT",
			"BEGIN:STANDARD", "DTSTART:20251026T030000", "TZOFFSETFROM:+0200", "TZOFFSETTO:+0100", "TZNAME:CET",
			"RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU", "END:STANDARD", "END:VTIMEZONE")},
		{"America/New_York", lines("BEGIN:VTIMEZONE", "TZID:America/New_York", "X-LIC-LOCATION:America/New_York",
			"BEGIN:DAYLIGHT", "DTSTART:20250309T020000", "TZOFFSETFROM:-0500", "TZOFFSETTO:-0400", "TZNAME:EDT",
			"RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU", "END:DAYLIGHT",
			"BEGIN:STANDARD", "DTSTART:20251102T020000", "TZOFFSETFROM:-0400", "TZOFFSETTO:-0500", "TZNAME:EST",
			"RRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU", "END:STANDARD", "END:VTIMEZONE")},
		{"Asia/Kolkata", lines("BEGIN:VTIMEZONE", "TZID:Asia/Kolkata", "X-LIC-LOCATION:Asia/Kolkata",
			"BEGIN:STANDARD", "DTSTART:19700101T000000", "TZOFFSETFROM:+0530", "TZOFFSETTO:+0530", "TZNAME:IST",
			"END:STANDARD", "END:VTIMEZONE")},
	} {
		got, err := VTimezone(tc.zone, vtzFrom, "\r\n")
		if err != nil || got != tc.want {
			t.Errorf("VTimezone(%s) = %v\n%s\nwant\n%s", tc.zone, err, got, tc.want)
		}
	}
	for _, bad := range []string{"", "Local", "Mars/Olympus_Mons"} {
		if _, err := VTimezone(bad, vtzFrom, "\r\n"); err == nil {
			t.Errorf("VTimezone(%q) succeeded", bad)
		}
	}
}

// TestVTimezoneMatchesTzdata expands every generated observance through
// 2037 and checks each transition against Go's tzdata: the right instant
// and the right offsets on either side.
func TestVTimezoneMatchesTzdata(t *testing.T) {
	for _, zone := range []string{"Europe/Berlin", "America/New_York", "Australia/Sydney", "Australia/Lord_Howe",
		"Asia/Kolkata", "America/Sao_Paulo", "UTC", "Africa/Casablanca", "Pacific/Chatham"} {
		t.Run(zone, func(t *testing.T) {
			src, err := VTimezone(zone, vtzFrom, "\r\n")
			if err != nil {
				t.Fatal(err)
			}
			components, err := contentline.Parse([]byte(src))
			if err != nil {
				t.Fatal(err)
			}
			loc, _ := time.LoadLocation(zone)
			var got []time.Time
			for _, kind := range []string{"DAYLIGHT", "STANDARD"} {
				for _, o := range components[0].ChildrenNamed(kind) {
					got = append(got, observed(t, o, loc)...)
				}
			}
			slices.SortFunc(got, time.Time.Compare)
			want := transitions(loc, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), vtimezoneEnd)
			if len(got) != len(want) {
				t.Fatalf("%d transitions from the VTIMEZONE, %d in tzdata\n%s", len(got), len(want), src)
			}
			for i, w := range want {
				if !got[i].Equal(w.at) {
					t.Errorf("transition %d at %v, tzdata has %v", i, got[i], w.at)
				}
			}
			if zone == "Africa/Casablanca" && (!strings.Contains(src, "RDATE:") || strings.Contains(src, "RRULE:")) {
				t.Errorf("Casablanca's irregular changes are not RDATEs:\n%s", src)
			}
		})
	}
}

// observed are the instants an observance's onsets fall on, through 2037,
// each checked to move the zone between its offsets.
func observed(t *testing.T, o *contentline.Component, loc *time.Location) []time.Time {
	t.Helper()
	from, _ := zoneOffset(propertyValue(o, "TZOFFSETFROM"))
	to, _ := zoneOffset(propertyValue(o, "TZOFFSETTO"))
	if from == to {
		return nil
	}
	wall := time.FixedZone("", from)
	start, err := time.ParseInLocation("20060102T150405", propertyValue(o, "DTSTART"), wall)
	if err != nil {
		t.Fatal(err)
	}
	onsets := []time.Time{start}
	if r := propertyValue(o, "RRULE"); r != "" {
		options, err := rrule.StrToROptionInLocation(r, wall)
		if err != nil {
			t.Fatal(err)
		}
		options.Dtstart = start
		rule, err := rrule.NewRRule(*options)
		if err != nil {
			t.Fatal(err)
		}
		onsets = rule.Between(start, vtimezoneEnd, true)
	}
	for _, p := range o.Props {
		if p.Name == "RDATE" {
			at, err := time.ParseInLocation("20060102T150405", p.Value, wall)
			if err != nil {
				t.Fatal(err)
			}
			onsets = append(onsets, at)
		}
	}
	var out []time.Time
	for _, at := range onsets {
		if !at.Before(vtimezoneEnd) {
			continue
		}
		_, before := at.Add(-time.Second).In(loc).Zone()
		_, after := at.In(loc).Zone()
		if before != from || after != to {
			t.Errorf("at %v the offset goes %d to %d; the VTIMEZONE says %d to %d", at, before, after, from, to)
		}
		out = append(out, at.UTC())
	}
	return out
}

// slimZone loads a zone from Go's own zoneinfo.zip, the slim tzdata Go
// embeds and reads on Windows, whose later transitions come from a
// rule.
func slimZone(t *testing.T, name string) *time.Location {
	t.Helper()
	root, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Skipf("no go command: %v", err)
	}
	zr, err := zip.OpenReader(filepath.Join(strings.TrimSpace(string(root)), "lib", "time", "zoneinfo.zip"))
	if err != nil {
		t.Skipf("no zoneinfo.zip in GOROOT: %v", err)
	}
	defer func() { _ = zr.Close() }()
	f, err := zr.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocationFromTZData(name, data)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// Go's rule-based zone periods end the last day of a leap year before it
// begins (Go 1.27, found on Windows): the walk steps over it, and still
// finds New York's two changes a year.
func TestTransitionsWithSlimTzdata(t *testing.T) {
	loc := slimZone(t, "America/New_York")
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := transitions(loc, from, vtimezoneEnd)
	if want := 2 * (vtimezoneEnd.Year() - 2025); len(ts) != want {
		t.Fatalf("%d transitions, want %d", len(ts), want)
	}
	for _, tr := range ts {
		if m := tr.at.Month(); m != time.March && m != time.November {
			t.Errorf("a transition at %s", tr.at)
		}
	}
	if _, err := VTimezone("America/New_York", from, "\r\n"); err != nil {
		t.Fatal(err)
	}
}
