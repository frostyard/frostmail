package pimsync_test

// CONTRACT TEST for task card T-0069 (docs/tasks). Do not edit.

import (
	"cmp"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

func ics(lines ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VCALENDAR\r\n")
}

func utcAt(s string) time.Time {
	t, err := time.Parse("2006-01-02T15:04Z", s)
	if err != nil {
		panic(err)
	}
	return t
}

// newCalendarEnv is newEnv with the calendar service on, a clock, floating
// times read in New York, and a second identity, me@alias.example.
func newCalendarEnv(t *testing.T, now *time.Time) *env {
	t.Helper()
	ctx := t.Context()
	e := newEnv(t, davtest.Options{}, api.ServiceKindCalendar)
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO identities (account_id, name, email) VALUES (?, 'Me', 'me@alias.example')`, e.acct.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sec := secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json"))
	if err := sec.Set(ctx, secrets.AccountPassword(e.acct.ID), davtest.Password); err != nil {
		t.Fatal(err)
	}
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	e.m = pimsync.New(e.db, sec, slog.New(slog.NewTextHandler(io.Discard, nil)), pimsync.Config{
		HTTP: e.dav.Client, Batch: 2, Now: func() time.Time { return *now }, Local: newYork,
	})
	return e
}

// occurrences lists the stored occurrences overlapping [from, to) as
// "start summary" lines: 01-02T15:04 (UTC) for timed ones, 01-02 for
// all-day ones.
func (e *env) occurrences(from, to string) []string {
	e.t.Helper()
	f := store.OccurrenceFilter{From: utcAt(from + "T00:00Z"), To: utcAt(to + "T00:00Z"), FromDate: from, ToDate: to}
	occ, err := e.db.Occurrences(e.t.Context(), f)
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for _, o := range occ {
		layout := "01-02T15:04"
		if o.Instance.AllDay {
			layout = "01-02"
		}
		out = append(out, o.Instance.Start.Format(layout)+" "+o.Event.Summary)
	}
	return out
}

func standup(count int) []byte {
	return ics(
		"BEGIN:VEVENT", "UID:standup", "DTSTAMP:20261001T000000Z", "SEQUENCE:1",
		"DTSTART;TZID=Europe/Berlin:20261009T090000", "DTEND;TZID=Europe/Berlin:20261009T091500",
		"RRULE:FREQ=DAILY;COUNT="+string(rune('0'+count)), "SUMMARY:Standup", "LOCATION:Room 4",
		"ORGANIZER;CN=Maria:mailto:maria@example.com",
		"ATTENDEE;PARTSTAT=ACCEPTED:mailto:user@example.com",
		"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER:-PT10M", "END:VALARM",
		"END:VEVENT",
		"BEGIN:VEVENT", "UID:standup", "DTSTAMP:20261001T000000Z",
		"RECURRENCE-ID;TZID=Europe/Berlin:20261010T090000",
		"DTSTART;TZID=Europe/Berlin:20261010T100000", "DTEND;TZID=Europe/Berlin:20261010T101500",
		"SUMMARY:Standup (late)", "ORGANIZER:mailto:maria@example.com", "END:VEVENT")
}

// TestPassIndexesEvents: a pass parses the calendars' events, stores them
// with the user's answers and expands them into the window.
func TestPassIndexesEvents(t *testing.T) {
	now := utcAt("2026-10-08T12:00Z")
	e := newCalendarEnv(t, &now)
	work := e.dav.Calendar("work", "Work", "#3366cc", "VEVENT", "VTODO")
	e.dav.Put(work+"standup.ics", standup(5))
	e.dav.Put(work+"holiday.ics", ics("BEGIN:VEVENT", "UID:holiday", "DTSTART;VALUE=DATE:20261012",
		"DTEND;VALUE=DATE:20261013", "SUMMARY:Holiday", "TRANSP:TRANSPARENT", "END:VEVENT"))
	e.dav.Put(work+"lunch.ics", ics("BEGIN:VEVENT", "UID:lunch", "DTSTART:20261009T110000Z", "DTEND:20261009T120000Z",
		"SUMMARY:Lunch", "ORGANIZER:mailto:bob@example.com", "ATTENDEE;PARTSTAT=TENTATIVE:mailto:Me@Alias.example", "END:VEVENT"))
	e.dav.Put(work+"gym.ics", ics("BEGIN:VEVENT", "UID:gym", "DTSTART:20261009T180000", "DTEND:20261009T190000",
		"SUMMARY:Gym", "END:VEVENT"))
	e.dav.Put(work+"task.ics", ics("BEGIN:VTODO", "UID:t1", "SUMMARY:Do", "DUE:20261010T090000Z", "END:VTODO"))
	e.dav.Put(work+"broken.ics", []byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:broken\r\nno colon here\r\n"))
	e.pass()

	ctx := t.Context()
	from, to, ok, err := e.db.InstanceWindow(ctx)
	if !ok || err != nil || from.Format(time.DateOnly) != "2025-10-08" || to.Format(time.DateOnly) != "2028-10-07" {
		t.Errorf("window = %v–%v %v %v", from, to, ok, err)
	}
	want := []string{
		"10-09T07:00 Standup", "10-09T11:00 Lunch", "10-09T22:00 Gym", "10-10T08:00 Standup (late)",
		"10-11T07:00 Standup", "10-12 Holiday", "10-12T07:00 Standup", "10-13T07:00 Standup",
	}
	if got := e.occurrences("2026-10-09", "2026-10-14"); !slices.Equal(got, want) {
		t.Errorf("occurrences:\n%q\nwant\n%q", got, want)
	}
	if !slices.Contains(e.eventNames(), "calendar.changed") {
		t.Errorf("events = %q", e.eventNames())
	}

	occ, _ := e.db.Occurrences(ctx, store.OccurrenceFilter{From: utcAt("2026-10-09T00:00Z"), To: utcAt("2026-10-10T00:00Z")})
	if len(occ) != 3 {
		t.Fatalf("Oct 9 = %+v", occ)
	}
	series, err := e.db.Event(ctx, occ[0].Event.ID)
	if err != nil || series.PartStat != "accepted" || series.Organizer != "maria@example.com" || series.OrganizerName != "Maria" ||
		series.TZID != "Europe/Berlin" || series.Recurrence != "RRULE:FREQ=DAILY;COUNT=5" || series.Location != "Room 4" ||
		len(series.Alarms) != 1 || series.Alarms[0].Offset == nil || *series.Alarms[0].Offset != -10*time.Minute ||
		occ[0].Instance.RecurrenceID != "2026-10-09T07:00:00.000Z" {
		t.Errorf("series = %+v, %v (occurrence %+v)", series, err, occ[0].Instance)
	}
	if lunch := occ[1].Event; lunch.PartStat != "tentative" {
		t.Errorf("an identity's answer = %q", lunch.PartStat)
	}
	if gym := occ[2].Event; gym.PartStat != "" || gym.TZID != "" {
		t.Errorf("gym = %+v", gym)
	}

	// Changes and deletions on the server replace the index.
	e.dav.Put(work+"standup.ics", standup(2))
	e.dav.Delete(work + "holiday.ics")
	e.pass()
	want = []string{"10-09T07:00 Standup", "10-09T11:00 Lunch", "10-09T22:00 Gym", "10-10T08:00 Standup (late)"}
	if got := e.occurrences("2026-10-09", "2026-10-14"); !slices.Equal(got, want) {
		t.Errorf("after changes:\n%q\nwant\n%q", got, want)
	}

	// A calendar removed on the server takes its events along.
	e.dav.RemoveCollection(work)
	e.pass()
	if got := e.occurrences("2026-10-09", "2026-10-14"); len(got) != 0 {
		t.Errorf("after the calendar went = %q", got)
	}
}

// TestWindowMoves: when the day changes, the next pass moves the window
// and expands every stored event into it, without fetching them again.
func TestWindowMoves(t *testing.T) {
	now := utcAt("2026-10-08T12:00Z")
	e := newCalendarEnv(t, &now)
	cal := e.dav.Calendar("home", "Home", "")
	e.dav.Put(cal+"daily.ics", ics("BEGIN:VEVENT", "UID:daily", "DTSTART:20240101T120000Z", "DTEND:20240101T130000Z",
		"RRULE:FREQ=DAILY", "SUMMARY:Daily", "END:VEVENT"))
	e.pass()
	days := func(days ...string) []int {
		var n []int
		for _, d := range days {
			next, _ := time.Parse(time.DateOnly, d)
			n = append(n, len(e.occurrences(d, next.AddDate(0, 0, 1).Format(time.DateOnly))))
		}
		return n
	}
	if got := days("2025-10-07", "2025-10-08", "2028-10-06", "2028-10-07"); !slices.Equal(got, []int{0, 1, 1, 0}) {
		t.Errorf("occurrences on the window's edges = %v", got)
	}

	now = utcAt("2026-10-10T08:00Z")
	e.pass()
	if got := days("2025-10-09", "2025-10-10", "2028-10-08", "2028-10-09"); !slices.Equal(got, []int{0, 1, 1, 0}) {
		t.Errorf("occurrences on the moved window's edges = %v", got)
	}
	if n := e.requests()["REPORT "+cal+" calendar-multiget"]; n != 0 {
		t.Errorf("the pass fetched the calendar again: %v", e.requests())
	}
	from, _, _, _ := e.db.InstanceWindow(t.Context())
	if from.Format(time.DateOnly) != "2025-10-10" {
		t.Errorf("window from = %v", from)
	}
}

// TestInstances: stored events expand in their own zones, floating ones
// in the local zone, with overrides in place of their occurrences.
func TestInstances(t *testing.T) {
	newYork, _ := time.LoadLocation("America/New_York")
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	events := []store.EventRow{
		{ID: 10, UID: "s", Start: utcAt("2026-10-24T07:00Z"), End: utcAt("2026-10-24T07:30Z"), TZID: "Europe/Berlin",
			Recurrence: "RRULE:FREQ=DAILY;COUNT=3"},
		{ID: 11, UID: "s", RecurrenceID: "2026-10-25T08:00:00.000Z", Start: utcAt("2026-10-25T10:00Z"), End: utcAt("2026-10-25T10:30Z"),
			TZID: "Europe/Berlin"},
		{ID: 12, UID: "f", Start: utcAt("2026-10-20T13:00Z"), End: utcAt("2026-10-20T14:00Z"), Recurrence: "RRULE:FREQ=WEEKLY;COUNT=3"},
		{ID: 13, UID: "d", AllDay: true, Start: day("2026-10-24"), End: day("2026-10-25"), Recurrence: "RRULE:FREQ=YEARLY"},
		{ID: 14, UID: "x", Start: utcAt("2026-12-01T09:00Z"), End: utcAt("2026-12-01T10:00Z"), TZID: "UTC+05:30"},
	}
	got, err := pimsync.Instances(events, newYork, utcAt("2026-10-01T00:00Z"), utcAt("2026-11-15T00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(got, func(a, b store.InstanceRow) int {
		return cmp.Or(a.Start.Compare(b.Start), cmp.Compare(a.EventID, b.EventID))
	})
	want := []store.InstanceRow{
		{EventID: 12, RecurrenceID: "2026-10-20T13:00:00.000Z", Start: utcAt("2026-10-20T13:00Z"), End: utcAt("2026-10-20T14:00Z")},
		{EventID: 13, RecurrenceID: "2026-10-24", AllDay: true, Start: day("2026-10-24"), End: day("2026-10-25")},
		{EventID: 10, RecurrenceID: "2026-10-24T07:00:00.000Z", Start: utcAt("2026-10-24T07:00Z"), End: utcAt("2026-10-24T07:30Z")},
		{EventID: 11, RecurrenceID: "2026-10-25T08:00:00.000Z", Start: utcAt("2026-10-25T10:00Z"), End: utcAt("2026-10-25T10:30Z")},
		{EventID: 10, RecurrenceID: "2026-10-26T08:00:00.000Z", Start: utcAt("2026-10-26T08:00Z"), End: utcAt("2026-10-26T08:30Z")},
		{EventID: 12, RecurrenceID: "2026-10-27T13:00:00.000Z", Start: utcAt("2026-10-27T13:00Z"), End: utcAt("2026-10-27T14:00Z")},
		{EventID: 12, RecurrenceID: "2026-11-03T14:00:00.000Z", Start: utcAt("2026-11-03T14:00Z"), End: utcAt("2026-11-03T15:00Z")},
	}
	if len(got) != len(want) {
		t.Fatalf("instances = %+v", got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.EventID != w.EventID || g.RecurrenceID != w.RecurrenceID || g.AllDay != w.AllDay || !g.Start.Equal(w.Start) || !g.End.Equal(w.End) {
			t.Errorf("instance %d = %+v, want %+v", i, g, w)
		}
	}
	if none, err := pimsync.Instances(nil, newYork, utcAt("2026-10-01T00:00Z"), utcAt("2026-11-15T00:00Z")); err != nil || len(none) != 0 {
		t.Errorf("no events: %+v, %v", none, err)
	}
	from, to := pimsync.Window(time.Date(2026, 10, 8, 23, 30, 0, 0, time.FixedZone("x", -5*3600)))
	if from.Format(time.RFC3339) != "2025-10-09T00:00:00Z" || to.Format(time.RFC3339) != "2028-10-08T00:00:00Z" {
		t.Errorf("Window = %v–%v; want the UTC day's", from, to)
	}
}
