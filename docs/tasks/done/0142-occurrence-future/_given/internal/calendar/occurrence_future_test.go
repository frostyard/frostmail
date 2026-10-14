package calendar

// CONTRACT TEST for task card T-0142 (docs/tasks). Do not edit.
// It uses the fixtures and helpers of eventpatch_test.go (T-0139),
// occurrence_test.go (T-0140) and occurrence_all_test.go (T-0141).

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const newUID = "standup-2@frostmail"

// googleOverride is googleSeries' override of October 29.
var googleOverride = ev(
	"BEGIN:VEVENT", "DTSTART;TZID=America/New_York:20261029T110000", "DTEND;TZID=America/New_York:20261029T111500",
	"DTSTAMP:20261010T120000Z", "UID:standup@google.com", "RECURRENCE-ID;TZID=America/New_York:20261029T090000",
	"CREATED:20261010T115900Z", "LAST-MODIFIED:20261010T120000Z", "SEQUENCE:0", "SUMMARY:Standup (moved)",
	"END:VEVENT",
)

func splitSeries(t *testing.T, src, id string, c EventChange) (past, future string) {
	t.Helper()
	patched, split, err := PatchOccurrence([]byte(src), id, ScopeFuture, c, newUID, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	if split == nil {
		t.Fatalf("All Future Events did not split the series:\n%s", patched)
	}
	for _, out := range [][]byte{patched, split} {
		if _, err := Parse(out, Options{Local: time.UTC}); err != nil {
			t.Fatalf("a half does not parse: %v\n%s", err, out)
		}
	}
	return string(patched), string(split)
}

func TestPatchFutureSplitsTheSeries(t *testing.T) {
	past, future := splitSeries(t, googleSeries, "2026-10-29T13:00:00.000Z", EventChange{Summary: sp("Standup v2")})
	same(t, past, strings.NewReplacer(
		"RRULE:FREQ=WEEKLY;BYDAY=TH;COUNT=10\r\nDTSTAMP:20261010T120000Z",
		"RRULE:FREQ=WEEKLY;BYDAY=TH;UNTIL=20261029T125959Z\r\nDTSTAMP:"+nowStamp,
		"CREATED:20261010T115900Z\r\nLAST-MODIFIED:20261010T120000Z\r\nSEQUENCE:0\r\nSUMMARY:Standup\r\n",
		"CREATED:20261010T115900Z\r\nLAST-MODIFIED:"+nowStamp+"\r\nSEQUENCE:1\r\nSUMMARY:Standup\r\n",
		googleOverride, "",
	).Replace(googleSeries))
	same(t, future, strings.NewReplacer(
		"DTSTART;TZID=America/New_York:20261015T090000\r\nDTEND;TZID=America/New_York:20261015T091500\r\n"+
			"RRULE:FREQ=WEEKLY;BYDAY=TH;COUNT=10\r\nDTSTAMP:20261010T120000Z\r\nUID:standup@google.com\r\n"+
			"CREATED:20261010T115900Z\r\nLAST-MODIFIED:20261010T120000Z\r\nSEQUENCE:0\r\nSUMMARY:Standup\r\n",
		"DTSTART;TZID=America/New_York:20261029T090000\r\nDTEND;TZID=America/New_York:20261029T091500\r\n"+
			"RRULE:FREQ=WEEKLY;BYDAY=TH;COUNT=8\r\nDTSTAMP:"+nowStamp+"\r\nUID:"+newUID+"\r\n"+
			"CREATED:"+nowStamp+"\r\nLAST-MODIFIED:"+nowStamp+"\r\nSEQUENCE:0\r\nSUMMARY:Standup v2\r\n",
		"UID:standup@google.com\r\nRECURRENCE-ID", "UID:"+newUID+"\r\nRECURRENCE-ID",
	).Replace(googleSeries))

	before, after := occurrences(t, past), occurrences(t, future)
	if len(before) != 2 || before["2026-10-22T13:00:00.000Z"].Summary != "Standup" {
		t.Errorf("before the split: %+v", before)
	}
	if len(after) != 8 || after["2026-10-29T13:00:00.000Z"].Summary != "Standup (moved)" ||
		after["2026-11-05T14:00:00.000Z"].Summary != "Standup v2" || after["2026-12-17T14:00:00.000Z"].UID != newUID {
		t.Errorf("after the split: %+v", after)
	}
}

func TestPatchFutureMovesTheRest(t *testing.T) {
	start := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC) // 10:00 EST, from 09:00
	past, future := splitSeries(t, editedSeries(t), "2026-11-05T14:00:00.000Z", EventChange{AllDay: sp(false),
		Zone: sp("America/New_York"), Start: &start, End: sp(start.Add(15 * time.Minute))})
	hasLines(t, past, "RRULE:FREQ=WEEKLY;BYDAY=TH;UNTIL=20261105T135959Z",
		"RECURRENCE-ID;TZID=America/New_York:20261022T090000", "RECURRENCE-ID;TZID=America/New_York:20261029T090000")
	if strings.Contains(past, "EXDATE") {
		t.Errorf("November 12's EXDATE belongs to the new series:\n%s", past)
	}
	hasLines(t, future, "DTSTART;TZID=America/New_York:20261105T100000", "DTEND;TZID=America/New_York:20261105T101500",
		"RRULE:FREQ=WEEKLY;BYDAY=TH;COUNT=7", "EXDATE;TZID=America/New_York:20261112T100000")
	if strings.Contains(future, "RECURRENCE-ID") || strings.Contains(future, "standup@google.com") {
		t.Errorf("the new series holds the old one's overrides or UID:\n%s", future)
	}
	if before := occurrences(t, past); len(before) != 3 || before["2026-10-22T13:00:00.000Z"].Location != "Room 2" {
		t.Errorf("before the split: %+v", before)
	}
	after := occurrences(t, future)
	if len(after) != 6 {
		t.Errorf("%d occurrences after the split, want 6: %+v", len(after), after)
	}
	for id, o := range after {
		if wall := o.Start.In(o.Zone); wall.Hour() != 10 || wall.Day() == 12 {
			t.Errorf("%s reads back as %+v", id, o)
		}
	}
}

func TestPatchFutureOfAnAllDaySeries(t *testing.T) {
	past, future := splitSeries(t, appleSeries, "2026-11-02", EventChange{Summary: sp("Recycling out")})
	hasLines(t, past, "RRULE:FREQ=WEEKLY;BYDAY=MO;UNTIL=20261101", "RECURRENCE-ID;VALUE=DATE:20261019")
	hasLines(t, future, "DTSTART;VALUE=DATE:20261102", "DTEND;VALUE=DATE:20261103", "RRULE:FREQ=WEEKLY;BYDAY=MO",
		"UID:"+newUID, "SUMMARY:Recycling out", "X-WR-ALARMUID:A1")
	if strings.Contains(future, "RECURRENCE-ID") || strings.Count(future, "BEGIN:VEVENT") != 1 {
		t.Errorf("the new series:\n%s", future)
	}
	if o := occurrences(t, future)["2026-11-09"]; o.Summary != "Recycling out" {
		t.Errorf("November 9 reads back as %+v", o)
	}
	if before := occurrences(t, past); len(before) != 3 || before["2026-10-19"].Summary != "Bins out (late)" {
		t.Errorf("before the split: %+v", before)
	}
}

func TestPatchFutureKeepsUntil(t *testing.T) {
	series := strings.Replace(googleSeries, "COUNT=10", "UNTIL=20261217T140000Z", 1)
	past, future := splitSeries(t, series, "2026-11-05T14:00:00.000Z", EventChange{Location: sp("Room 3")})
	hasLines(t, past, "RRULE:FREQ=WEEKLY;BYDAY=TH;UNTIL=20261105T135959Z")
	hasLines(t, future, "RRULE:FREQ=WEEKLY;BYDAY=TH;UNTIL=20261217T140000Z", "LOCATION:Room 3")
	if n, m := len(occurrences(t, past)), len(occurrences(t, future)); n != 3 || m != 7 {
		t.Errorf("%d occurrences before the split and %d after, want 3 and 7", n, m)
	}
}

func TestPatchFutureFromTheFirstIsAll(t *testing.T) {
	c := EventChange{Summary: sp("Team standup")}
	patched, split, err := PatchOccurrence([]byte(googleSeries), "2026-10-15T13:00:00.000Z", ScopeFuture, c, newUID, eventNow)
	if err != nil || split != nil {
		t.Fatalf("from the first occurrence: %v, split\n%s", err, split)
	}
	same(t, string(patched), patchedAll(t, googleSeries, "2026-10-15T13:00:00.000Z", c))
	if _, _, err := PatchOccurrence([]byte(googleSeries), "2026-10-22T13:00:00.000Z", ScopeFuture, c, "", eventNow); err == nil {
		t.Error("a split took no UID")
	}
	if _, _, err := PatchOccurrence([]byte(googleSeries), "2026-10-22T13:00:00.000Z", ScopeFuture,
		EventChange{RRule: sp("FREQ=SOMETIMES")}, newUID, eventNow); err == nil {
		t.Error("a split took a rule that does not parse")
	}
}

func TestDeleteFuture(t *testing.T) {
	got, err := DeleteOccurrence([]byte(googleSeries), "2026-10-29T13:00:00.000Z", ScopeFuture, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	past, _ := splitSeries(t, googleSeries, "2026-10-29T13:00:00.000Z", EventChange{Summary: sp("Standup v2")})
	same(t, string(got), past)

	got, err = DeleteOccurrence([]byte(appleSeries), "2026-10-19", ScopeFuture, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	if occs := occurrences(t, string(got)); len(occs) != 1 || strings.Contains(string(got), "Bins out (late)") {
		t.Errorf("after deleting from October 19: %+v\n%s", occs, got)
	}

	if _, err := DeleteOccurrence([]byte(googleSeries), "2026-10-15T13:00:00.000Z", ScopeFuture, eventNow); !errors.Is(err, ErrWholeSeries) {
		t.Errorf("deleting from the first occurrence = %v, want ErrWholeSeries", err)
	}
}
