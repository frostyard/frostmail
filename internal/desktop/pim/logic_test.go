package pim

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"
	"reflect"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

func TestDates(t *testing.T) {
	if got := addDays("2026-02-28", 1); got != "2026-03-01" {
		t.Error(got)
	}
	if got := startOfWeek("2026-10-08", 1); got != "2026-10-05" {
		t.Error(got)
	}
	if got := startOfWeek("2026-10-08", 0); got != "2026-10-04" {
		t.Error(got)
	}
	g := monthGrid("2026-10-08", 0)
	if len(g) != 42 || g[0] != "2026-09-27" || g[41] != "2026-11-07" {
		t.Error(g)
	}
	if from, to := viewRange(viewWeek, "2026-10-08", 1); from != "2026-10-05" || to != "2026-10-12" {
		t.Error(from, to)
	}
	if got := stepDate(viewMonth, "2026-01-31", 1); got != "2026-02-28" {
		t.Error(got)
	}
	if got := stepDate(viewMonth, "2026-03-31", -1); got != "2026-02-28" {
		t.Error(got)
	}
	for _, c := range []struct {
		view viewKind
		date string
		want string
	}{
		{viewDay, "2026-10-08", "Thursday, October 8, 2026"},
		{viewMonth, "2026-10-08", "October 2026"},
		{viewWeek, "2026-10-08", "October 2026"},
		{viewWeek, "2026-09-30", "Sep – Oct 2026"},
		{viewWeek, "2026-12-31", "Dec 2026 – Jan 2027"},
	} {
		if got := viewTitle(c.view, c.date, 0); got != c.want {
			t.Errorf("%s %s: %q", c.view, c.date, got)
		}
	}
}

func TestDragArithmetic(t *testing.T) {
	loc := time.UTC
	r := span{time.Date(2026, 10, 8, 14, 0, 0, 0, loc), time.Date(2026, 10, 8, 15, 0, 0, 0, loc)}
	if got := stepsOf(24); got != 30 {
		t.Error(got)
	}
	if got := stepsOf(5); got != 0 {
		t.Error(got)
	}
	if got := minuteAt(1000); got != 1245 {
		t.Error(got)
	}
	if got := minuteAt(-3); got != 0 {
		t.Error(got)
	}
	m := moved(r, 1, 15, loc)
	if !m.start.Equal(time.Date(2026, 10, 9, 14, 15, 0, 0, loc)) || m.end.Sub(m.start) != time.Hour {
		t.Error(m)
	}
	if s := resized(r, -120, loc); !s.end.Equal(r.start.Add(15 * time.Minute)) {
		t.Error(s)
	}
	if d := drawn("2026-10-08", 600, 540, loc); !d.start.Equal(time.Date(2026, 10, 8, 9, 0, 0, 0, loc)) || !d.end.Equal(time.Date(2026, 10, 8, 10, 15, 0, 0, loc)) {
		t.Error(d)
	}
	answer := api.PartStatAccepted
	if canMove(&api.Occurrence{CalendarID: 1, Answer: &answer}, map[int64]bool{1: true}) {
		t.Error("an invitation moved")
	}
	if canMove(&api.Occurrence{CalendarID: 2}, map[int64]bool{1: true}) {
		t.Error("a read-only event moved")
	}
	f := shiftedDays(&api.Occurrence{AllDay: true, StartDate: "2026-10-09", EndDate: "2026-10-10"}, 2, loc, "UTC")
	if *f.StartDate != "2026-10-11" || *f.EndDate != "2026-10-12" {
		t.Error(f)
	}
}

func TestLayoutColumnsAndLanes(t *testing.T) {
	loc := time.UTC
	at := func(h, m int) time.Time { return time.Date(2026, 10, 8, h, m, 0, 0, loc) }
	occs := []api.Occurrence{{EventID: 1, Start: at(9, 0), End: at(10, 0)}, {EventID: 2, Start: at(9, 30), End: at(10, 30)}, {EventID: 3, Start: at(11, 0), End: at(11, 30)}}
	b := timedLayout(occs, []string{"2026-10-08"}, loc)["2026-10-08"]
	if b[0].columns != 2 || b[1].column != 1 || b[2].columns != 1 {
		t.Errorf("%+v", b)
	}
	// An occurrence across midnight splits.
	cross := []api.Occurrence{{EventID: 1, Start: time.Date(2026, 10, 8, 23, 0, 0, 0, loc), End: time.Date(2026, 10, 9, 1, 0, 0, 0, loc)}}
	l := timedLayout(cross, []string{"2026-10-08", "2026-10-09"}, loc)
	if l["2026-10-08"][0].endMin != 1440 || l["2026-10-09"][0].startMin != 0 || l["2026-10-09"][0].endMin != 60 {
		t.Errorf("%+v", l)
	}
	declinedAnswer := api.PartStatDeclined
	if got := timedLayout([]api.Occurrence{{Start: at(9, 0), End: at(10, 0), Answer: &declinedAnswer}}, []string{"2026-10-08"}, loc)["2026-10-08"]; len(got) != 0 {
		t.Error("a declined occurrence shows")
	}
	days := []string{"2026-10-05", "2026-10-06", "2026-10-07"}
	bars := allDayLanes([]api.Occurrence{{AllDay: true, StartDate: "2026-10-05", EndDate: "2026-10-07"}, {AllDay: true, StartDate: "2026-10-06", EndDate: "2026-10-08"}}, days)
	if bars[0].first != 0 || bars[0].last != 1 || bars[0].lane != 0 || bars[1].lane != 1 || bars[1].last != 2 {
		t.Errorf("%+v", bars)
	}
	items := monthItems([]api.Occurrence{{EventID: 1, Start: at(9, 0), End: at(10, 0)}, {EventID: 2, AllDay: true, StartDate: "2026-10-08", EndDate: "2026-10-09"}}, "2026-10-08", loc)
	if items[0].EventID != 2 || items[1].EventID != 1 {
		t.Error("all-day ones do not come first")
	}
}

func TestEventText(t *testing.T) {
	for in, want := range map[string]string{
		"":                                       "",
		"RRULE:FREQ=DAILY;COUNT=7":               "Every day, 7 times",
		"RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR": "Every weekday",
		"RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE": "Every 2 weeks on Monday and Wednesday",
		"RRULE:FREQ=MONTHLY;BYDAY=-1FR":            "Every month on the last Friday",
		"RRULE:FREQ=MONTHLY;BYMONTHDAY=15":         "Every month on day 15",
		"RRULE:FREQ=YEARLY;UNTIL=20270101":         "Every year, until January 1, 2027",
		"RRULE:FREQ=WEEKLY;BYSETPOS=1":             "Custom",
		"EXDATE:20261008":                          "Custom",
	} {
		if got := describeRecurrence(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	for in, want := range map[int64]string{0: "At time of event", 15: "15 minutes before", 60: "1 hour before", 1440: "1 day before", -540: "9 hours after"} {
		if got := alarmText(in); got != want {
			t.Errorf("%d: %q", in, got)
		}
	}
	k := clock{loc: time.UTC}
	at := func(h, m int) time.Time { return time.Date(2026, 10, 8, h, m, 0, 0, time.UTC) }
	if got := k.timeRange(at(9, 0), at(9, 15)); got != "9:00 – 9:15 AM" {
		t.Error(got)
	}
	if got := k.timeRange(at(9, 0), at(13, 0)); got != "9:00 AM – 1:00 PM" {
		t.Error(got)
	}
	if got := k.dateText(true, time.Time{}, "2026-10-12", "2026-10-15"); got != "October 12 – 14, 2026" {
		t.Error(got)
	}
	if got := k.dateText(true, time.Time{}, "2026-10-12", "2026-10-13"); got != "Monday, October 12, 2026" {
		t.Error(got)
	}
	if got := k.dateText(false, at(9, 0), "", ""); got != "Thursday, October 8, 2026" {
		t.Error(got)
	}
}

func TestReminderWhen(t *testing.T) {
	k := clock{loc: time.UTC}
	now := time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC)
	r := func(d time.Duration) api.Reminder { return api.Reminder{Start: now.Add(d)} }
	for _, c := range []struct {
		r    api.Reminder
		want string
	}{
		{r(10 * time.Minute), "In 10 minutes"},
		{r(30 * time.Second), "In 1 minute"},
		{r(-2 * time.Minute), "Now"},
		{r(-10 * time.Minute), "10 minutes ago"},
		{r(3 * time.Hour), "Today, 1:00 PM"},
		{r(26 * time.Hour), "Tomorrow, 12:00 PM"},
		{r(-5 * time.Hour), "Today, 5:00 AM"},
		{r(-24 * time.Hour), "Yesterday, 10:00 AM"},
		{api.Reminder{AllDay: true, StartDate: "2026-10-09"}, "Tomorrow"},
		{api.Reminder{AllDay: true, StartDate: "2026-10-16"}, "Fri, Oct 16"},
	} {
		if got := k.reminderWhen(c.r, now); got != c.want {
			t.Errorf("%+v: %q, want %q", c.r, got, c.want)
		}
	}
	ch := snoozeChoices(now, time.UTC)
	if len(ch) != 5 || ch[3].label != "1 hour" || ch[4].label != "Tomorrow" || !ch[4].until.Equal(time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("%+v", ch)
	}
}

func TestRepeatRules(t *testing.T) {
	start := "2026-10-08" // a Thursday, the second of the month
	for rule, want := range map[string]repeatPreset{"": presetNever, "FREQ=DAILY": presetDaily, "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR": presetWeekdays,
		"FREQ=WEEKLY": presetWeekly, "FREQ=WEEKLY;INTERVAL=2": presetBiweekly, "FREQ=MONTHLY": presetMonthly, "FREQ=YEARLY": presetYearly, "FREQ=DAILY;INTERVAL=3": presetCustom, "FREQ=WEEKLY;BYSETPOS=1": presetCustom} {
		if got := presetOf(rule, start); got != want {
			t.Errorf("%q: %q", rule, got)
		}
	}
	if got := ruleOfRepeat(repeat{frequency: "WEEKLY", interval: 2, weekdays: []string{"MO", "WE"}}, start); got != "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE" {
		t.Error(got)
	}
	if got := ruleOfRepeat(repeat{frequency: "WEEKLY", interval: 1, weekdays: []string{"TH"}}, start); got != "FREQ=WEEKLY" {
		t.Error(got)
	}
	if got := ruleOfRepeat(repeat{frequency: "MONTHLY", interval: 1, byWeekday: true}, start); got != "FREQ=MONTHLY;BYDAY=2TH" {
		t.Error(got)
	}
	if got := endOf("FREQ=DAILY;COUNT=7", time.UTC); got.kind != "count" || got.count != 7 {
		t.Error(got)
	}
	if got := endOf("FREQ=DAILY;UNTIL=20261031T235959Z", time.UTC); got.kind != "date" || got.date != "2026-10-31" {
		t.Error(got)
	}
	if got := withEnd("FREQ=DAILY;COUNT=7", repeatEnd{kind: "date", date: "2026-10-31"}, false, time.UTC); got != "FREQ=DAILY;UNTIL=20261031T235959Z" {
		t.Error(got)
	}
	if got := withEnd("FREQ=DAILY", repeatEnd{kind: "date", date: "2026-10-31"}, true, time.UTC); got != "FREQ=DAILY;UNTIL=20261031" {
		t.Error(got)
	}
	if got := withEnd("FREQ=DAILY;COUNT=7", repeatEnd{kind: "never"}, false, time.UTC); got != "FREQ=DAILY" {
		t.Error(got)
	}
	if got := followStart("FREQ=WEEKLY;BYDAY=TH", "2026-10-08", "2026-10-09"); got != "FREQ=WEEKLY;BYDAY=FR" {
		t.Error(got)
	}
	if got := followStart("FREQ=MONTHLY;BYDAY=2TH", "2026-10-08", "2026-10-15"); got != "FREQ=MONTHLY;BYDAY=3TH" {
		t.Error(got)
	}
	if got := monthlyByDayLabel(nthWeekday(start), start); got != "on the second Thursday" {
		t.Error(got)
	}
}

func TestAZeroMoveKeepsItsInstants(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	// The second 1:30 of the night the clocks go back, and a start with seconds.
	second := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC).In(ny)
	for _, r := range []span{{second, second.Add(10 * time.Minute)}, {time.Date(2026, 10, 8, 9, 0, 30, 0, ny), time.Date(2026, 10, 8, 10, 0, 45, 0, ny)}} {
		if got := moved(r, 0, 0, ny); !got.start.Equal(r.start) || !got.end.Equal(r.end) {
			t.Errorf("moved: %v, want %v", got, r)
		}
		if got := resized(r, 0, ny); !got.start.Equal(r.start) || !got.end.Equal(r.end) {
			t.Errorf("resized: %v, want %v", got, r)
		}
	}
}

func TestRepeatKeepsWhatTheSheetDoesNotShow(t *testing.T) {
	// WKST decides which weeks an interval counts.
	start := "2026-10-08"
	r := repeatOf("FREQ=WEEKLY;INTERVAL=2;BYDAY=TH,SU;WKST=SU", start)
	if got := ruleOfRepeat(*r, start); got != "FREQ=WEEKLY;INTERVAL=2;BYDAY=TH,SU;WKST=SU" {
		t.Error(got)
	}
	both := "2026-02-27" // February 2026: the fourth Friday is the last
	if !isLastWeek(both) || nthWeekday(both) != "4FR" {
		t.Fatalf("%v %s", isLastWeek(both), nthWeekday(both))
	}
	r = repeatOf("FREQ=MONTHLY;BYDAY=-1FR", both)
	if r == nil || !r.byWeekday {
		t.Fatalf("%+v", r)
	}
	if got := ruleOfRepeat(*r, both); got != "FREQ=MONTHLY;BYDAY=-1FR" {
		t.Error(got)
	}
	if got := monthlyByDayLabel(r.monthlyByDay(both), both); got != "on the last Friday" {
		t.Error(got)
	}
	// A rule counting from the start keeps counting so.
	r = repeatOf("FREQ=MONTHLY;BYDAY=4FR", both)
	if got := ruleOfRepeat(*r, both); got != "FREQ=MONTHLY;BYDAY=4FR" {
		t.Error(got)
	}
	// A start moved out of the last week has no last to keep.
	r = repeatOf("FREQ=MONTHLY;BYDAY=-1FR", both)
	if got := ruleOfRepeat(*r, "2026-02-13"); got != "FREQ=MONTHLY;BYDAY=2FR" {
		t.Error(got)
	}
}

func TestUntilKeepsItsTime(t *testing.T) {
	rule := "FREQ=DAILY;UNTIL=20261031T150000Z"
	end := endOf(rule, time.UTC)
	if end.kind != "date" || end.date != "2026-10-31" {
		t.Fatalf("%+v", end)
	}
	// A change that keeps the end keeps the end's time.
	if got := withEnd("FREQ=WEEKLY", end, false, time.UTC); got != "FREQ=WEEKLY;UNTIL=20261031T150000Z" {
		t.Error(got)
	}
	// A new date is a new end, at that day's close.
	if got := withEnd(rule, repeatEnd{kind: "date", date: "2026-11-02"}, false, time.UTC); got != "FREQ=DAILY;UNTIL=20261102T235959Z" {
		t.Error(got)
	}
	// UNTIL's form follows the event's.
	if got := withEnd("FREQ=WEEKLY", end, true, time.UTC); got != "FREQ=WEEKLY;UNTIL=20261031" {
		t.Error(got)
	}
	dateOnly := endOf("FREQ=DAILY;UNTIL=20261031", time.UTC)
	if got := withEnd("FREQ=WEEKLY", dateOnly, false, time.UTC); got != "FREQ=WEEKLY;UNTIL=20261031T235959Z" {
		t.Error(got)
	}
}

func TestUntilClosesTheDayWhereMidnightIsSkipped(t *testing.T) {
	for _, c := range []struct{ zone, date, want string }{
		// time.Date reads the skipped midnight as the hour before here, and
		// as the hour after in Beirut.
		{"America/Havana", "2026-03-07", "20260308T045959Z"},
		{"Asia/Beirut", "2026-03-28", "20260328T215959Z"},
		{"America/New_York", "2026-03-07", "20260308T045959Z"},
	} {
		loc, err := time.LoadLocation(c.zone)
		if err != nil {
			t.Skip(err)
		}
		rule := withEnd("FREQ=DAILY", repeatEnd{kind: "date", date: c.date}, false, loc)
		if want := "FREQ=DAILY;UNTIL=" + c.want; rule != want {
			t.Errorf("%s: %s, want %s", c.zone, rule, want)
		}
		if got := endOf(rule, loc); got.date != c.date {
			t.Errorf("%s: reads back as %s", c.zone, got.date)
		}
	}
}

func TestDraftTimesAcrossClockChanges(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	const zone = "America/New_York"
	// 2:30 does not happen on March 8, 2026.
	d := newDraft(newTimes{start: time.Date(2026, 3, 8, 7, 0, 0, 0, ny), end: time.Date(2026, 3, 8, 8, 0, 0, 0, ny)}, 7, zone, ny)
	skipped := d
	skipped.startDate, skipped.startTime = "2026-03-08", "02:30"
	if e := skipped.validate(); e.start != skippedTime {
		t.Errorf("start: %+v", e)
	}
	skipped = d
	skipped.endDate, skipped.endTime = "2026-03-08", "02:30"
	skipped.startTime = "01:00"
	if e := skipped.validate(); e.end != skippedTime {
		t.Errorf("end: %+v", e)
	}

	// The clocks go back at 2:00 on November 1, 2026: 1:50 EDT to 1:10 EST
	// is twenty minutes, whose end reads earlier than its start.
	start := time.Date(2026, 11, 1, 5, 50, 0, 0, time.UTC)
	end := time.Date(2026, 11, 1, 6, 10, 0, 0, time.UTC)
	event := &api.CalendarEvent{CalendarID: 3, TimeZone: zone, Start: start, End: end}
	dd := draftOf(event, "UTC", time.UTC)
	if dd.startTime != "01:50" || dd.endTime != "01:10" {
		t.Fatalf("%+v", dd)
	}
	if e := dd.validate(); e.any() {
		t.Errorf("an event in the repeated hour cannot be saved: %+v", e)
	}
	if s, e := dd.instants(); !s.Equal(start) || !e.Equal(end) {
		t.Errorf("instants %v %v", s, e)
	}
	// An edit of another field writes no times.
	renamed := dd
	renamed.summary = "Night shift"
	if f := changedFields(&dd, renamed); f.Start != nil || f.End != nil {
		t.Errorf("%+v", f)
	}
	// An event in the hour's second pass keeps it when only its end changes.
	second := time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)
	later := draftOf(&api.CalendarEvent{CalendarID: 3, TimeZone: zone, Start: second, End: second.Add(10 * time.Minute)}, "UTC", time.UTC)
	edited := later
	edited.endTime = "01:50"
	if e := edited.validate(); e.any() {
		t.Errorf("%+v", e)
	}
	f := changedFields(&later, edited)
	if f.Start == nil || !f.Start.Equal(second) || !f.End.Equal(second.Add(20*time.Minute)) {
		t.Errorf("start %v, end %v", f.Start, f.End)
	}
	// Moving the start keeps the length, across the repeated hour too.
	day := draftOf(&api.CalendarEvent{CalendarID: 3, TimeZone: zone, Start: time.Date(2026, 10, 31, 5, 50, 0, 0, time.UTC), End: time.Date(2026, 10, 31, 6, 10, 0, 0, time.UTC)}, "UTC", time.UTC)
	into := moveStart(day, "2026-11-01", "01:50")
	if into.endTime != "01:10" || into.validate().any() {
		t.Errorf("%+v: %+v", into, into.validate())
	}
	if s, e := into.instants(); e.Sub(s) != 20*time.Minute {
		t.Errorf("length %v", e.Sub(s))
	}
	// A change of zone reads the wall time in the new zone.
	rezoned := dd
	rezoned.timeZone = "UTC"
	rezoned.endTime = "02:10"
	if s, _ := rezoned.instants(); !s.Equal(time.Date(2026, 11, 1, 1, 50, 0, 0, time.UTC)) {
		t.Errorf("start %v", s)
	}
}

func TestEventDraft(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 10, 8, 9, 0, 0, 0, loc)
	d := newDraft(newTimes{start: start, end: start.Add(time.Hour)}, 7, "UTC", loc)
	if d.startDate != "2026-10-08" || d.startTime != "09:00" || d.endTime != "10:00" {
		t.Errorf("%+v", d)
	}
	moved := moveStart(d, "2026-10-09", "10:30")
	if moved.endDate != "2026-10-09" || moved.endTime != "11:30" {
		t.Errorf("the end did not follow: %+v", moved)
	}
	bad := d
	bad.endTime = "08:00"
	if e := bad.validate(); e.end != "The end is before the start" {
		t.Errorf("%+v", e)
	}
	bad = d
	bad.invitees = []draftInvitee{{email: "nope"}}
	if e := bad.validate(); e.invitees != "Check the invitees' addresses" {
		t.Errorf("%+v", e)
	}
	// A new event sends every field but empty text; a change only what
	// changed, and all of the times.
	f := changedFields(nil, d)
	if f.Summary != nil || f.Start == nil || f.TimeZone == nil || *f.AllDay {
		t.Errorf("%+v", f)
	}
	changed := d
	changed.summary = "X"
	f = changedFields(&d, changed)
	if f.Summary == nil || f.Start != nil || f.Attendees != nil {
		t.Errorf("%+v", f)
	}
	changed = d
	changed.endTime = "10:15"
	f = changedFields(&d, changed)
	if f.Start == nil || f.End == nil || f.TimeZone == nil {
		t.Errorf("%+v", f)
	}
	withPeople := d
	withPeople.invitees = []draftInvitee{{email: "a@b.test", role: api.AttendeeRoleOptional}}
	f = changedFields(&d, withPeople)
	if len(f.Attendees) != 1 || *f.Attendees[0].Role != api.AttendeeRoleOptional {
		t.Errorf("%+v", f)
	}
	removed := changedFields(&withPeople, d)
	if removed.Attendees == nil || len(removed.Attendees) != 0 {
		t.Error("removing every invitee does not send an empty list")
	}
	if !sendsInvitees(&d, withPeople) || sendsInvitees(&withPeople, d) {
		t.Error("sendsInvitees")
	}
	allDay := newDraft(newTimes{allDay: true, startDate: "2026-10-08", endDate: "2026-10-11"}, 7, "UTC", loc)
	if allDay.endDate != "2026-10-10" {
		t.Error(allDay.endDate)
	}
	if f := changedFields(nil, allDay); *f.StartDate != "2026-10-08" || *f.EndDate != "2026-10-11" {
		t.Errorf("%+v", f)
	}
	event := &api.CalendarEvent{CalendarID: 3, Start: start, End: start.Add(time.Hour), Recurrence: "RRULE:FREQ=DAILY\nEXDATE:20261008T090000Z",
		Attendees: []api.Attendee{{Email: "me@a.test", IsUser: true}, {Email: "ann@a.test", Name: "Ann", Role: api.AttendeeRoleOptional, Answer: api.PartStatAccepted}}, Alarms: []int64{15}}
	dd := draftOf(event, "UTC", loc)
	if dd.rrule != "FREQ=DAILY" || len(dd.invitees) != 1 || dd.invitees[0].role != api.AttendeeRoleOptional || dd.alarms[0] != 15 {
		t.Errorf("%+v", dd)
	}
}

func TestEventWindowKeys(t *testing.T) {
	if got := eventWindowKey(5, ""); got != "event-5" {
		t.Error(got)
	}
	if got := eventWindowKey(5, "2026-10-08T11:00:00.000Z"); got != "event-5-2026-10-08T11-00-00-000Z" {
		t.Error(got)
	}
}

func TestContactDraft(t *testing.T) {
	c := &api.Contact{GivenName: "Ada", Birthday: "1815-12-10", Emails: []api.LabeledValue{{Label: "home", Value: "ada@a.test"}}}
	d := contactDraftOf(c)
	if d.birthday != (birthdayParts{month: "12", day: "10", year: "1815"}) {
		t.Errorf("%+v", d.birthday)
	}
	if got := birthdayValue(birthdayParts{month: "12", day: "09"}); got != "--12-09" {
		t.Error(got)
	}
	if !validBirthday(birthdayParts{month: "02", day: "29"}) || validBirthday(birthdayParts{month: "02", day: "29", year: "2025"}) || !validBirthday(birthdayParts{month: "02", day: "29", year: "2024"}) || validBirthday(birthdayParts{month: "04", day: "31"}) {
		t.Error("birthdays")
	}
	if errs := validateContact(contactDraft{}, true); errs["name"] == "" {
		t.Error("a new contact needs an identity")
	}
	if errs := validateContact(contactDraft{emails: []api.LabeledValue{{Value: "nope"}}}, false); errs["emails.0"] == "" {
		t.Error("a bad address passed")
	}
	if got := nextLabel(kindPhone, []string{"mobile", "home"}); got != "work" {
		t.Error(got)
	}
	if got := nextLabel(kindEmail, []string{"home", "work", "other"}); got != "other" {
		t.Error(got)
	}
	d.givenName = " Ada "
	if f := contactChanges(c, d); !emptyContactFields(f) {
		t.Errorf("an unchanged contact sends %+v", f)
	}
	d.emails = append(d.emails, api.LabeledValue{Label: "work", Value: " w@a.test "})
	f := contactChanges(c, d)
	if len(f.Emails) != 2 || f.Emails[1].Value != "w@a.test" {
		t.Errorf("%+v", f.Emails)
	}
	if got := birthdayText("1815-12-10"); got != "December 10, 1815" {
		t.Error(got)
	}
	if got := birthdayText("--12-09"); got != "December 9" {
		t.Error(got)
	}
	if got := addressLines(api.PostalAddress{Street: "1 Main\nFloor 2", Locality: "Town", Postcode: "12345", Country: "Land"}); !reflect.DeepEqual(got, []string{"1 Main", "Floor 2", "Town 12345", "Land"}) {
		t.Error(got)
	}
}

func TestTaskText(t *testing.T) {
	today := "2026-10-08"
	for due, want := range map[string]string{"": "", today: "Today", "2026-10-09": "Tomorrow", "2026-10-07": "Yesterday", "2026-10-16": "Fri, Oct 16", "2027-10-09": "Oct 9, 2027"} {
		if got := dueText(due, today); got != want {
			t.Errorf("%q: %q", due, got)
		}
	}
	parent := int64(1)
	all := []api.Task{{ID: 1, ListID: 1, Due: "2026-10-08"}, {ID: 2, ListID: 1, ParentID: &parent}, {ID: 3, ListID: 2, Due: "2026-10-01", Completed: true}, {ID: 4, ListID: 2, Due: "2026-10-20"}}
	if got := sourceTasks(all, sourceToday, today, true, nil); len(got) != 1 || got[0].ID != 1 {
		t.Errorf("Today: %+v", got)
	}
	if got := sourceTasks(all, sourceAll, today, false, nil); len(got) != 3 {
		t.Errorf("All: %+v", got)
	}
	if got := sourceTasks(all, sourceAll, today, true, nil); len(got) != 4 {
		t.Errorf("All with completed: %+v", got)
	}
	if got := sourceTasks(all, 2, today, false, map[int64]bool{3: true}); len(got) != 2 {
		t.Errorf("a ticked task left: %+v", got)
	}
	lines := taskLines(all[:2], map[int64]string{1: "Work"})
	if lines[0].header != "Work" || lines[2].level != 1 {
		t.Errorf("%+v", lines)
	}
	soon := dueSoon(all, today, 10)
	if len(soon) != 1 || soon[0].ID != 1 {
		t.Errorf("%+v", soon)
	}
}

func TestScalePhotoCropsToAJPEGAtMost512(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1000, 700))
	var in bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatal(err)
	}
	p, err := scalePhoto(in.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if p.ContentType != "image/jpeg" {
		t.Error(p.ContentType)
	}
	data, _ := base64.StdEncoding.DecodeString(p.Data)
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 512 || b.Dy() != 512 {
		t.Errorf("size %v", b)
	}
	// A small image is not scaled up.
	in.Reset()
	if err := png.Encode(&in, image.NewRGBA(image.Rect(0, 0, 100, 60))); err != nil {
		t.Fatal(err)
	}
	p, err = scalePhoto(in.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	data, _ = base64.StdEncoding.DecodeString(p.Data)
	img, _ = jpeg.Decode(bytes.NewReader(data))
	if b := img.Bounds(); b.Dx() != 60 || b.Dy() != 60 {
		t.Errorf("size %v", b)
	}
	if _, err := scalePhoto([]byte("not an image")); err == nil {
		t.Error("garbage scaled")
	}
}

func TestNoticeText(t *testing.T) {
	for k, want := range map[api.NoticeKind]string{api.NoticeKindInvitations: "Send Invitations?", api.NoticeKindUpdates: "Send Updates?", api.NoticeKindCancellations: "Send Cancellations?"} {
		if got := noticeTitle(k); got != want {
			t.Error(got)
		}
	}
	if noticeText(1) != "The invitee will get it by email." || noticeText(3) != "3 invitees will get it by email." {
		t.Error("noticeText")
	}
	var list []string
	for i := range 25 {
		list = remember(list, string(rune('a'+i)))
	}
	if len(list) != undoLimit || list[0] != "f" {
		t.Error(list)
	}
	if got := remember(list, ""); len(got) != undoLimit {
		t.Error("an empty restore was kept")
	}
}
