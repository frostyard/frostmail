package calendar

// CONTRACT TEST for task card T-0141 (docs/tasks). Do not edit.
// It uses the fixtures and helpers of eventpatch_test.go (T-0139) and
// occurrence_test.go (T-0140).

import (
	"strings"
	"testing"
	"time"
)

func patchedAll(t *testing.T, src, id string, c EventChange) string {
	t.Helper()
	out, split, err := PatchOccurrence([]byte(src), id, ScopeAll, c, "", eventNow)
	if err != nil {
		t.Fatal(err)
	}
	if split != nil {
		t.Errorf("All Events split the series:\n%s", split)
	}
	if _, err := Parse(out, Options{Local: time.UTC}); err != nil {
		t.Fatalf("the patched series does not parse: %v\n%s", err, out)
	}
	return string(out)
}

// editedSeries is googleSeries with November 12 deleted and October 22
// given a room of its own, at its own times.
func editedSeries(t *testing.T) string {
	t.Helper()
	deleted, err := DeleteOccurrence([]byte(googleSeries), "2026-11-12T14:00:00.000Z", ScopeThis, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	return patchedOccurrence(t, string(deleted), "2026-10-22T13:00:00.000Z", EventChange{Location: sp("Room 2")})
}

func hasLines(t *testing.T, got string, lines ...string) {
	t.Helper()
	for _, line := range lines {
		if !strings.Contains(got, line+"\r\n") {
			t.Errorf("no %q in:\n%s", line, got)
		}
	}
}

func TestPatchAllRenamesTheSeries(t *testing.T) {
	got := patchedAll(t, googleSeries, "2026-10-22T13:00:00.000Z", EventChange{Summary: sp("Team standup")})
	same(t, got, strings.NewReplacer(
		"COUNT=10\r\nDTSTAMP:20261010T120000Z", "COUNT=10\r\nDTSTAMP:"+nowStamp,
		"LAST-MODIFIED:20261010T120000Z\r\nSEQUENCE:0\r\nSUMMARY:Standup\r\n",
		"LAST-MODIFIED:"+nowStamp+"\r\nSEQUENCE:0\r\nSUMMARY:Team standup\r\n",
	).Replace(googleSeries))

	// A value an override shares with the master follows it; one of its own
	// stays.
	got = patchedAll(t, editedSeries(t), "2026-11-05T14:00:00.000Z",
		EventChange{Summary: sp("Team standup"), Location: sp("Room 7")})
	occs := occurrences(t, got)
	for id, want := range map[string][2]string{
		"2026-10-22T13:00:00.000Z": {"Team standup", "Room 2"},
		"2026-10-29T13:00:00.000Z": {"Standup (moved)", "Room 7"},
		"2026-11-19T14:00:00.000Z": {"Team standup", "Room 7"},
	} {
		if o := occs[id]; o.Summary != want[0] || o.Location != want[1] {
			t.Errorf("%s reads back as %q in %q, want %q", id, o.Summary, o.Location, want)
		}
	}
	if len(occs) != 9 {
		t.Errorf("%d occurrences, want 9", len(occs))
	}
}

func TestPatchAllMovesTheSeries(t *testing.T) {
	start := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC) // 10:00 EST, from 09:00
	got := patchedAll(t, editedSeries(t), "2026-11-05T14:00:00.000Z", EventChange{AllDay: sp(false),
		Zone: sp("America/New_York"), Start: &start, End: sp(start.Add(30 * time.Minute))})
	hasLines(t, got,
		"DTSTART;TZID=America/New_York:20261015T100000", "DTEND;TZID=America/New_York:20261015T103000",
		"EXDATE;TZID=America/New_York:20261112T100000",
		// October 22's override was at its occurrence's times, and moves.
		"RECURRENCE-ID;TZID=America/New_York:20261022T100000",
		"DTSTART;TZID=America/New_York:20261022T100000", "DTEND;TZID=America/New_York:20261022T103000",
		// October 29's was moved to 11:00, and stays.
		"RECURRENCE-ID;TZID=America/New_York:20261029T100000",
		"DTSTART;TZID=America/New_York:20261029T110000", "DTEND;TZID=America/New_York:20261029T111500")
	if m := master(t, got); m.Sequence != 2 {
		t.Errorf("the master's SEQUENCE is %d, want 2", m.Sequence)
	}
	occs := occurrences(t, got)
	if len(occs) != 9 {
		t.Errorf("%d occurrences, want 9", len(occs))
	}
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for id, o := range occs {
		wall, length := o.Start.In(newYork), o.End.Sub(o.Start)
		switch id {
		case "2026-10-29T14:00:00.000Z":
			if wall.Hour() != 11 || length != 15*time.Minute || o.Summary != "Standup (moved)" {
				t.Errorf("October 29 reads back as %+v", o)
			}
		default:
			if wall.Hour() != 10 || wall.Minute() != 0 || length != 30*time.Minute || wall.Day() == 12 {
				t.Errorf("%s reads back as %+v", id, o)
			}
		}
	}
	if occs["2026-10-22T14:00:00.000Z"].Location != "Room 2" || occs["2026-10-15T14:00:00.000Z"].Summary != "Standup" {
		t.Errorf("occurrences = %+v", occs)
	}
}

func TestPatchAllMovesUntil(t *testing.T) {
	series := strings.Replace(googleSeries, "COUNT=10", "UNTIL=20261217T140000Z", 1)
	start := time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC)
	got := patchedAll(t, series, "2026-10-15T13:00:00.000Z", EventChange{AllDay: sp(false),
		Zone: sp("America/New_York"), Start: &start, End: sp(start.Add(15 * time.Minute))})
	hasLines(t, got, "RRULE:FREQ=WEEKLY;BYDAY=TH;UNTIL=20261217T150000Z")
	if occs := occurrences(t, got); len(occs) != 10 {
		t.Errorf("%d occurrences, want 10", len(occs))
	}
}

func TestPatchAllChangesTheRule(t *testing.T) {
	// Every other Thursday: October 22's override names no occurrence any
	// more; October 29's and November 12's EXDATE still do.
	got := patchedAll(t, editedSeries(t), "2026-10-15T13:00:00.000Z",
		EventChange{RRule: sp("FREQ=WEEKLY;INTERVAL=2;BYDAY=TH;COUNT=5")})
	hasLines(t, got, "RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=TH;COUNT=5", "EXDATE;TZID=America/New_York:20261112T090000",
		"RECURRENCE-ID;TZID=America/New_York:20261029T090000")
	if strings.Contains(got, "Room 2") || strings.Count(got, "BEGIN:VEVENT") != 2 {
		t.Errorf("October 22's override should be gone:\n%s", got)
	}
	if occs := occurrences(t, got); len(occs) != 4 || occs["2026-10-29T13:00:00.000Z"].Summary != "Standup (moved)" {
		t.Errorf("occurrences = %+v", occs)
	}

	// No rule: the event stops repeating, and its overrides and EXDATEs go.
	got = patchedAll(t, editedSeries(t), "2026-10-22T13:00:00.000Z", EventChange{RRule: sp("")})
	if strings.Count(got, "BEGIN:VEVENT") != 1 || strings.Contains(got, "EXDATE") || strings.Contains(got, "RRULE:FREQ=WEEKLY") {
		t.Errorf("a series without a rule:\n%s", got)
	}
}

func TestPatchAllOfAnAllDaySeries(t *testing.T) {
	got := patchedAll(t, appleSeries, "2026-10-26", EventChange{AllDay: sp(true), StartDate: sp("2026-10-27"),
		EndDate: sp("2026-10-28"), RRule: sp("FREQ=WEEKLY;BYDAY=TU")})
	hasLines(t, got, "DTSTART;VALUE=DATE:20261013", "DTEND;VALUE=DATE:20261014", "RRULE:FREQ=WEEKLY;BYDAY=TU",
		"RECURRENCE-ID;VALUE=DATE:20261020",
		// The override had moved to the Tuesday, and stays there.
		"DTEND;VALUE=DATE:20261021\r\nDTSTAMP:20261001T080000Z\r\nDTSTART;VALUE=DATE:20261020")
	occs := occurrences(t, got)
	if occs["2026-10-20"].Summary != "Bins out (late)" || occs["2026-10-27"].Summary != "Bins out" || occs["2026-10-13"].Summary != "Bins out" {
		t.Errorf("occurrences = %+v", occs)
	}
	if _, ok := occs["2026-10-26"]; ok {
		t.Errorf("a Monday is left: %+v", occs)
	}
}

func TestPatchAllToAllDay(t *testing.T) {
	got := patchedAll(t, googleSeries, "2026-10-22T13:00:00.000Z",
		EventChange{AllDay: sp(true), StartDate: sp("2026-10-22"), EndDate: sp("2026-10-23")})
	hasLines(t, got, "DTSTART;VALUE=DATE:20261015", "DTEND;VALUE=DATE:20261016", "RECURRENCE-ID;VALUE=DATE:20261029")
	occs := occurrences(t, got)
	if len(occs) != 10 || occs["2026-10-29"].Summary != "Standup (moved)" || !occs["2026-11-05"].AllDay {
		t.Errorf("occurrences = %+v", occs)
	}
}

func TestPatchAllOfNothingAndOfWhatCannotBe(t *testing.T) {
	const oct22 = "2026-10-22T13:00:00.000Z"
	same(t, patchedAll(t, googleSeries, oct22, EventChange{}), googleSeries)
	start := time.Date(2026, 10, 22, 15, 0, 0, 0, time.UTC)
	for _, c := range []EventChange{{Start: &start}, {RRule: sp("FREQ=SOMETIMES")},
		{AllDay: sp(false), Zone: sp("Mars/Olympus"), Start: &start, End: sp(start.Add(time.Hour))}} {
		if _, _, err := PatchOccurrence([]byte(googleSeries), oct22, ScopeAll, c, "", eventNow); err == nil {
			t.Errorf("All Events took %+v", c)
		}
	}
	if _, err := DeleteOccurrence([]byte(googleSeries), oct22, ScopeAll, eventNow); err == nil {
		t.Error("All Events deleted by patch: deleting a series deletes its object")
	}
}
