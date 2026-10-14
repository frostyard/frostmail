package calendar

// CONTRACT TEST for task card T-0140 (docs/tasks). Do not edit.
// It uses eventpatch_test.go's fixtures and helpers (T-0139).

import (
	"strings"
	"testing"
	"time"
)

// googleSeries is a weekly standup on Thursdays in New York, ten times,
// with its third occurrence (October 29) moved to 11:00.
var googleSeries = ev(
	"BEGIN:VCALENDAR", "PRODID:-//Google Inc//Google Calendar 70.9054//EN", "VERSION:2.0",
	"BEGIN:VTIMEZONE", "TZID:America/New_York", "X-LIC-LOCATION:America/New_York",
	"BEGIN:DAYLIGHT", "TZOFFSETFROM:-0500", "TZOFFSETTO:-0400", "TZNAME:EDT", "DTSTART:19700308T020000",
	"RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=2SU", "END:DAYLIGHT",
	"BEGIN:STANDARD", "TZOFFSETFROM:-0400", "TZOFFSETTO:-0500", "TZNAME:EST", "DTSTART:19701101T020000",
	"RRULE:FREQ=YEARLY;BYMONTH=11;BYDAY=1SU", "END:STANDARD", "END:VTIMEZONE",
	"BEGIN:VEVENT", "DTSTART;TZID=America/New_York:20261015T090000", "DTEND;TZID=America/New_York:20261015T091500",
	"RRULE:FREQ=WEEKLY;BYDAY=TH;COUNT=10", "DTSTAMP:20261010T120000Z", "UID:standup@google.com",
	"CREATED:20261010T115900Z", "LAST-MODIFIED:20261010T120000Z", "SEQUENCE:0", "SUMMARY:Standup",
	"X-GOOGLE-CONFERENCE:https://meet.google.com/std-upup-now",
	"BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:This is an event reminder", "TRIGGER:-P0DT0H10M0S", "END:VALARM",
	"END:VEVENT",
	"BEGIN:VEVENT", "DTSTART;TZID=America/New_York:20261029T110000", "DTEND;TZID=America/New_York:20261029T111500",
	"DTSTAMP:20261010T120000Z", "UID:standup@google.com", "RECURRENCE-ID;TZID=America/New_York:20261029T090000",
	"CREATED:20261010T115900Z", "LAST-MODIFIED:20261010T120000Z", "SEQUENCE:0", "SUMMARY:Standup (moved)",
	"END:VEVENT", "END:VCALENDAR",
)

// The master's lines as an override of October 22 starts them.
var standupOverride = ev(
	"BEGIN:VEVENT", "DTSTART;TZID=America/New_York:20261022T090000",
	"RECURRENCE-ID;TZID=America/New_York:20261022T090000", "DTEND;TZID=America/New_York:20261022T091500",
	"DTSTAMP:"+nowStamp, "UID:standup@google.com", "CREATED:20261010T115900Z", "LAST-MODIFIED:"+nowStamp,
	"SEQUENCE:0", "SUMMARY:Standup in Room 2", "X-GOOGLE-CONFERENCE:https://meet.google.com/std-upup-now",
	"BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:This is an event reminder", "TRIGGER:-P0DT0H10M0S", "END:VALARM",
	"END:VEVENT",
)

const masterEnd = "END:VALARM\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nDTSTART;TZID=America/New_York:20261029T110000"

func occurrences(t *testing.T, src string) map[string]Event {
	t.Helper()
	events, err := Parse([]byte(src), Options{Local: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	occs, err := Expand(events, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Event{}
	for _, o := range occs {
		e := events[o.Event]
		e.Start, e.End = o.Start, o.End
		out[o.RecurrenceID] = e
	}
	return out
}

func patchedOccurrence(t *testing.T, src, id string, c EventChange) string {
	t.Helper()
	out, split, err := PatchOccurrence([]byte(src), id, ScopeThis, c, "", eventNow)
	if err != nil {
		t.Fatal(err)
	}
	if split != nil {
		t.Errorf("This Event split the series:\n%s", split)
	}
	return string(out)
}

func TestPatchOccurrenceMakesAnOverride(t *testing.T) {
	const oct22 = "2026-10-22T13:00:00.000Z"
	got := patchedOccurrence(t, googleSeries, oct22, EventChange{Summary: sp("Standup in Room 2")})
	same(t, got, strings.Replace(googleSeries, masterEnd,
		"END:VALARM\r\nEND:VEVENT\r\n"+standupOverride+"BEGIN:VEVENT\r\nDTSTART;TZID=America/New_York:20261029T110000", 1))
	occs := occurrences(t, got)
	if len(occs) != 10 || occs[oct22].Summary != "Standup in Room 2" || occs["2026-10-15T13:00:00.000Z"].Summary != "Standup" {
		t.Errorf("occurrences = %+v", occs)
	}
}

func TestPatchOccurrencePatchesItsOverride(t *testing.T) {
	const oct29 = "2026-10-29T13:00:00.000Z"
	got := patchedOccurrence(t, googleSeries, oct29, EventChange{Location: sp("Room 9"), Summary: sp("Standup (late)")})
	same(t, got, strings.NewReplacer(
		"LAST-MODIFIED:20261010T120000Z\r\nSEQUENCE:0\r\nSUMMARY:Standup (moved)\r\n",
		"LAST-MODIFIED:"+nowStamp+"\r\nSEQUENCE:0\r\nSUMMARY:Standup (late)\r\nLOCATION:Room 9\r\n",
		"DTEND;TZID=America/New_York:20261029T111500\r\nDTSTAMP:20261010T120000Z",
		"DTEND;TZID=America/New_York:20261029T111500\r\nDTSTAMP:"+nowStamp,
	).Replace(googleSeries))
	if occs := occurrences(t, got); occs[oct29].Location != "Room 9" || occs[oct29].Start.Hour() != 15 {
		t.Errorf("the override reads back as %+v", occs[oct29])
	}
}

func TestPatchOccurrenceMovesOneOccurrence(t *testing.T) {
	const nov5 = "2026-11-05T14:00:00.000Z" // after New York's DST ends: 09:00 EST
	start := time.Date(2026, 11, 5, 19, 0, 0, 0, time.UTC)
	got := patchedOccurrence(t, googleSeries, nov5, EventChange{AllDay: sp(false), Zone: sp("America/New_York"),
		Start: &start, End: sp(start.Add(30 * time.Minute))})
	if !strings.Contains(got, "RECURRENCE-ID;TZID=America/New_York:20261105T090000\r\n") ||
		!strings.Contains(got, "DTSTART;TZID=America/New_York:20261105T140000\r\n") ||
		!strings.Contains(got, "DTEND;TZID=America/New_York:20261105T143000\r\n") {
		t.Errorf("the moved occurrence:\n%s", got)
	}
	occs := occurrences(t, got)
	if o := occs[nov5]; !o.Start.Equal(start) || o.End.Sub(o.Start) != 30*time.Minute {
		t.Errorf("November 5 reads back as %+v", o)
	}
}

func TestPatchOccurrenceOfAnAllDaySeries(t *testing.T) {
	got := patchedOccurrence(t, appleSeries, "2026-10-26", EventChange{Summary: sp("Bins out (bank holiday)")})
	for _, line := range []string{"RECURRENCE-ID;VALUE=DATE:20261026\r\n", "DTSTART;VALUE=DATE:20261026\r\n",
		"DTEND;VALUE=DATE:20261027\r\n", "SUMMARY:Bins out (bank holiday)\r\n"} {
		if !strings.Contains(got, line) {
			t.Errorf("the override lacks %q:\n%s", line, got)
		}
	}
	if strings.Count(got, "X-WR-ALARMUID:A1") != 2 || strings.Count(got, "RRULE:") != 1 {
		t.Errorf("the override should copy the alarm but not the rule:\n%s", got)
	}
	if occs := occurrences(t, got); occs["2026-10-26"].Summary != "Bins out (bank holiday)" || occs["2026-11-02"].Summary != "Bins out" {
		t.Errorf("occurrences = %+v", occs)
	}
}

func TestDeleteOccurrence(t *testing.T) {
	got, err := DeleteOccurrence([]byte(googleSeries), "2026-10-22T13:00:00.000Z", ScopeThis, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	same(t, string(got), strings.NewReplacer(
		"X-GOOGLE-CONFERENCE:https://meet.google.com/std-upup-now\r\nBEGIN:VALARM",
		"X-GOOGLE-CONFERENCE:https://meet.google.com/std-upup-now\r\nEXDATE;TZID=America/New_York:20261022T090000\r\nBEGIN:VALARM",
		"RRULE:FREQ=WEEKLY;BYDAY=TH;COUNT=10\r\nDTSTAMP:20261010T120000Z", "RRULE:FREQ=WEEKLY;BYDAY=TH;COUNT=10\r\nDTSTAMP:"+nowStamp,
		"CREATED:20261010T115900Z\r\nLAST-MODIFIED:20261010T120000Z\r\nSEQUENCE:0\r\nSUMMARY:Standup\r\n",
		"CREATED:20261010T115900Z\r\nLAST-MODIFIED:"+nowStamp+"\r\nSEQUENCE:1\r\nSUMMARY:Standup\r\n",
	).Replace(googleSeries))
	if occs := occurrences(t, string(got)); len(occs) != 9 {
		t.Errorf("%d occurrences after the delete", len(occs))
	}

	// Deleting a moved occurrence removes its override too.
	got, err = DeleteOccurrence([]byte(googleSeries), "2026-10-29T13:00:00.000Z", ScopeThis, eventNow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "Standup (moved)") || !strings.Contains(string(got), "EXDATE;TZID=America/New_York:20261029T090000\r\n") {
		t.Errorf("after deleting the moved occurrence:\n%s", got)
	}
	if occs := occurrences(t, string(got)); len(occs) != 9 {
		t.Errorf("%d occurrences after deleting the moved one", len(occs))
	}

	got, err = DeleteOccurrence([]byte(appleSeries), "2026-10-26", ScopeThis, eventNow)
	if err != nil || !strings.Contains(string(got), "EXDATE;VALUE=DATE:20261026\r\n") {
		t.Errorf("all-day delete = %v\n%s", err, got)
	}
}

func TestOccurrencesThatAreNot(t *testing.T) {
	for _, id := range []string{"2026-10-23T13:00:00.000Z", "2026-10-22T14:00:00.000Z", "2027-03-04T14:00:00.000Z", "yesterday"} {
		if _, _, err := PatchOccurrence([]byte(googleSeries), id, ScopeThis, EventChange{Summary: sp("x")}, "", eventNow); err == nil {
			t.Errorf("patched %s, which is no occurrence", id)
		}
		if _, err := DeleteOccurrence([]byte(googleSeries), id, ScopeThis, eventNow); err == nil {
			t.Errorf("deleted %s, which is no occurrence", id)
		}
	}
	if _, _, err := PatchOccurrence([]byte(googleEvent), "2026-10-15T13:00:00.000Z", ScopeThis, EventChange{Summary: sp("x")}, "", eventNow); err == nil {
		t.Error("an event that does not repeat took an occurrence change")
	}
	if _, _, err := PatchOccurrence([]byte(googleSeries), "2026-10-22T13:00:00.000Z", ScopeThis, EventChange{RRule: sp("FREQ=DAILY")}, "", eventNow); err == nil {
		t.Error("one occurrence took a rule of its own")
	}
}
