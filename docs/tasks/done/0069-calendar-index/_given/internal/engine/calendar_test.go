package engine_test

// CONTRACT TEST for task card T-0069 (docs/tasks). Do not edit.

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func vcal(lines ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VCALENDAR\r\n")
}

// calendarEnv is a server whose clock reads 2026-10-08 12:00 UTC, with an
// account (user@dav.test) whose calendars Work and Home (read-only) are
// synced.
type calendarEnv struct {
	srv        *rpctest.Server
	c          *api.Client
	acct       int64
	work, home int64
}

func newCalendarEnv(t *testing.T) *calendarEnv {
	t.Helper()
	ctx := t.Context()
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	dav := davtest.New(t, davtest.Options{})
	srv := rpctest.StartWith(t, rpctest.Options{
		PIM: &pimsync.Config{HTTP: dav.Client, Interval: time.Hour, Local: time.UTC},
		Now: func() time.Time { return now },
	})
	e := &calendarEnv{srv: srv, c: srv.Dial(t)}
	e.acct = davAccount(t, e.c)

	work := dav.Calendar("work", "Work", "#3366cc")
	home := dav.Calendar("home", "Home", "")
	dav.SetReadOnly(home, true)
	dav.Put(work+"standup.ics", vcal(
		"BEGIN:VEVENT", "UID:standup", "DTSTART;TZID=Europe/Berlin:20261009T090000",
		"DTEND;TZID=Europe/Berlin:20261009T091500", "RRULE:FREQ=DAILY;COUNT=7", "SUMMARY:Standup", "LOCATION:Room 4",
		"DESCRIPTION:Daily\\, short", "ORGANIZER;CN=Maria:mailto:maria@example.com",
		"ATTENDEE;PARTSTAT=ACCEPTED:mailto:user@dav.test", "ATTENDEE;ROLE=OPT-PARTICIPANT:mailto:bob@example.com",
		"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER:-PT10M", "END:VALARM", "END:VEVENT",
		"BEGIN:VEVENT", "UID:standup", "RECURRENCE-ID;TZID=Europe/Berlin:20261010T090000",
		"DTSTART;TZID=Europe/Berlin:20261010T100000", "DTEND;TZID=Europe/Berlin:20261010T101500",
		"SUMMARY:Standup (late)", "ORGANIZER;CN=Maria:mailto:maria@example.com", "END:VEVENT"))
	dav.Put(work+"late.ics", vcal("BEGIN:VEVENT", "UID:late", "DTSTART:20261008T230000Z", "DTEND:20261008T233000Z",
		"SUMMARY:Late call", "END:VEVENT"))
	dav.Put(work+"holiday.ics", vcal("BEGIN:VEVENT", "UID:holiday", "DTSTART;VALUE=DATE:20261012",
		"DTEND;VALUE=DATE:20261013", "SUMMARY:Holiday", "TRANSP:TRANSPARENT", "END:VEVENT"))
	dav.Put(work+"review.ics", vcal("BEGIN:VEVENT", "UID:review", "DTSTART:20260105T150000Z", "DTEND:20260105T160000Z",
		"RRULE:FREQ=WEEKLY", "SUMMARY:Review", "ORGANIZER:mailto:carol@example.com",
		"BEGIN:VALARM", "ACTION:AUDIO", "TRIGGER;RELATED=END:-PT5M", "END:VALARM", "END:VEVENT"))
	dav.Put(home+"dentist.ics", vcal("BEGIN:VEVENT", "UID:dentist", "DTSTART:20261009T140000Z", "DTEND:20261009T150000Z",
		"SUMMARY:Dentist", "STATUS:TENTATIVE",
		"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER;VALUE=DATE-TIME:20261009T130000Z", "END:VALARM", "END:VEVENT"))

	if _, err := e.c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: e.acct, Service: api.ServiceKindCalendar,
		Enabled: true, URL: ptr(dav.URL)}); err != nil {
		t.Fatal(err)
	}
	if err := srv.PIM.Pass(ctx, e.acct); err != nil {
		t.Fatal(err)
	}
	cols, err := e.c.Account().Collections(ctx, &api.AccountCollectionsParams{AccountID: &e.acct})
	if err != nil {
		t.Fatal(err)
	}
	for _, col := range cols {
		switch col.Name {
		case "Work":
			e.work = col.ID
		case "Home":
			e.home = col.ID
		}
	}
	if e.work == 0 || e.home == 0 {
		t.Fatalf("collections = %+v", cols)
	}
	return e
}

func (e *calendarEnv) rng(t *testing.T, from, to, zone string, calendars ...int64) []api.Occurrence {
	t.Helper()
	p := &api.CalendarRangeParams{From: from, To: to, CalendarIDs: calendars}
	if zone != "" {
		p.TimeZone = &zone
	}
	occ, err := e.c.Calendar().Range(t.Context(), p)
	if err != nil {
		t.Fatalf("range %s–%s %s: %v", from, to, zone, err)
	}
	return occ
}

// lines writes occurrences as "start summary" in UTC.
func lines(occ []api.Occurrence) []string {
	var out []string
	for _, o := range occ {
		out = append(out, o.Start.UTC().Format("01-02T15:04")+" "+o.Summary)
	}
	return out
}

func TestCalendarRange(t *testing.T) {
	e := newCalendarEnv(t)
	berlin := e.rng(t, "2026-10-09", "2026-10-14", "Europe/Berlin")
	want := []string{
		"10-08T23:00 Late call", "10-09T07:00 Standup", "10-09T14:00 Dentist", "10-10T08:00 Standup (late)",
		"10-11T07:00 Standup", "10-11T22:00 Holiday", "10-12T07:00 Standup", "10-12T15:00 Review", "10-13T07:00 Standup",
	}
	if got := lines(berlin); !slices.Equal(got, want) {
		t.Fatalf("Berlin days:\n%q\nwant\n%q", got, want)
	}
	if got := lines(e.rng(t, "2026-10-09", "2026-10-14", "UTC")); !slices.Equal(got, []string{
		"10-09T07:00 Standup", "10-09T14:00 Dentist", "10-10T08:00 Standup (late)", "10-11T07:00 Standup",
		"10-12T00:00 Holiday", "10-12T07:00 Standup", "10-12T15:00 Review", "10-13T07:00 Standup",
	}) {
		t.Errorf("UTC days = %q", got)
	}

	standup, late, holiday, dentist := berlin[4], berlin[3], berlin[5], berlin[2]
	if standup.EventID == 0 || standup.RecurrenceID != "2026-10-11T07:00:00.000Z" || standup.CalendarID != e.work ||
		standup.AccountID != e.acct || standup.Location != "Room 4" || standup.AllDay || standup.StartDate != "" ||
		!standup.End.Equal(time.Date(2026, 10, 11, 7, 15, 0, 0, time.UTC)) || standup.Status != api.EventStatusConfirmed ||
		standup.Answer == nil || *standup.Answer != api.PartStatAccepted || standup.Transparent || !standup.Recurring {
		t.Errorf("a series occurrence = %+v", standup)
	}
	if late.EventID == standup.EventID || late.RecurrenceID != "2026-10-10T07:00:00.000Z" || !late.Recurring {
		t.Errorf("an override = %+v", late)
	}
	if berlin[1].EventID != standup.EventID {
		t.Errorf("occurrences of a series name its master: %d, %d", berlin[1].EventID, standup.EventID)
	}
	if !holiday.AllDay || holiday.StartDate != "2026-10-12" || holiday.EndDate != "2026-10-13" ||
		!holiday.End.Equal(time.Date(2026, 10, 12, 22, 0, 0, 0, time.UTC)) || !holiday.Transparent || holiday.Recurring ||
		holiday.Answer != nil {
		t.Errorf("an all-day event = %+v", holiday)
	}
	if dentist.CalendarID != e.home || dentist.Status != api.EventStatusTentative {
		t.Errorf("dentist = %+v", dentist)
	}

	if got := lines(e.rng(t, "2026-10-09", "2026-10-14", "UTC", e.home)); !slices.Equal(got, []string{"10-09T14:00 Dentist"}) {
		t.Errorf("Home only = %q", got)
	}
	if got := e.rng(t, "2026-10-09", "2026-10-10", ""); len(got) == 0 {
		t.Errorf("maild's local zone: nothing")
	}

	// Beyond the window (2025-10-08 to 2028-10-07), occurrences are
	// computed on demand; a range across its end has both.
	if got := lines(e.rng(t, "2029-01-01", "2029-01-29", "UTC")); !slices.Equal(got, []string{
		"01-01T15:00 Review", "01-08T15:00 Review", "01-15T15:00 Review", "01-22T15:00 Review",
	}) {
		t.Errorf("2029 = %q", got)
	}
	across := e.rng(t, "2028-09-25", "2028-10-23", "UTC")
	if got := lines(across); !slices.Equal(got, []string{
		"09-25T15:00 Review", "10-02T15:00 Review", "10-09T15:00 Review", "10-16T15:00 Review",
	}) {
		t.Errorf("across the window's end = %q", got)
	}
	if o := across[3]; o.RecurrenceID != "2028-10-16T15:00:00.000Z" || !o.Recurring || o.CalendarID != e.work || o.AccountID != e.acct {
		t.Errorf("an occurrence beyond the window = %+v", o)
	}

	// Hidden calendars are left out, inside the window and beyond it.
	off := false
	if _, err := e.c.Account().SetCollection(t.Context(), &api.AccountSetCollectionParams{ID: e.work, Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	if got := lines(e.rng(t, "2026-10-09", "2026-10-14", "UTC")); !slices.Equal(got, []string{"10-09T14:00 Dentist"}) {
		t.Errorf("with Work hidden = %q", got)
	}
	if got := e.rng(t, "2029-01-01", "2029-01-29", "UTC"); len(got) != 0 {
		t.Errorf("2029 with Work hidden = %q", lines(got))
	}
}

func TestCalendarRangeParams(t *testing.T) {
	e := newCalendarEnv(t)
	for _, p := range []api.CalendarRangeParams{
		{From: "2026-10-32", To: "2026-11-01"},
		{From: "2026-10-09", To: "10/10/2026"},
		{From: "2026-10-09", To: "2026-10-09"},
		{From: "2026-10-10", To: "2026-10-09"},
		{From: "2026-01-01", To: "2027-02-06"}, // 400 days is the most
		{From: "2026-10-09", To: "2026-10-10", TimeZone: ptr("Mars/Olympus")},
	} {
		if _, err := e.c.Calendar().Range(t.Context(), &p); code(err) != api.CodeInvalidParams {
			t.Errorf("range %+v: %v", p, err)
		}
	}
	if _, err := e.c.Calendar().Range(t.Context(), &api.CalendarRangeParams{From: "2026-01-01", To: "2027-02-05"}); err != nil {
		t.Errorf("400 days: %v", err)
	}
}

func TestCalendarEvent(t *testing.T) {
	e := newCalendarEnv(t)
	ctx := t.Context()
	occ := e.rng(t, "2026-10-09", "2026-10-14", "UTC")
	byName := map[string]api.Occurrence{}
	for _, o := range occ {
		byName[o.Summary] = o
	}
	master := byName["Standup"].EventID

	ev, err := e.c.Calendar().Event(ctx, &api.CalendarEventParams{ID: master})
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID != master || ev.RecurrenceID != "" || ev.CalendarID != e.work || ev.AccountID != e.acct || ev.UID != "standup" ||
		ev.Summary != "Standup" || ev.Location != "Room 4" || ev.Description != "Daily, short" || ev.AllDay ||
		!ev.Start.Equal(time.Date(2026, 10, 9, 7, 0, 0, 0, time.UTC)) || !ev.End.Equal(time.Date(2026, 10, 9, 7, 15, 0, 0, time.UTC)) ||
		ev.TimeZone != "Europe/Berlin" || !ev.Recurring || ev.Recurrence != "RRULE:FREQ=DAILY;COUNT=7" ||
		ev.Status != api.EventStatusConfirmed || ev.Transparent || ev.Answer == nil || *ev.Answer != api.PartStatAccepted ||
		!slices.Equal(ev.Alarms, []int64{10}) || ev.ReadOnly {
		t.Errorf("series = %+v", ev)
	}
	if o := ev.Organizer; o == nil || *o != (api.Attendee{Email: "maria@example.com", Name: "Maria", Role: api.AttendeeRoleChair, Answer: api.PartStatAccepted}) {
		t.Errorf("organizer = %+v", o)
	}
	if !slices.Equal(ev.Attendees, []api.Attendee{
		{Email: "user@dav.test", Role: api.AttendeeRoleRequired, Answer: api.PartStatAccepted, IsUser: true},
		{Email: "bob@example.com", Role: api.AttendeeRoleOptional, Answer: api.PartStatNeedsaction},
	}) {
		t.Errorf("attendees = %+v", ev.Attendees)
	}

	third, err := e.c.Calendar().Event(ctx, &api.CalendarEventParams{ID: master, RecurrenceID: ptr("2026-10-12T07:00:00.000Z")})
	if err != nil || third.ID != master || third.RecurrenceID != "2026-10-12T07:00:00.000Z" ||
		!third.Start.Equal(time.Date(2026, 10, 12, 7, 0, 0, 0, time.UTC)) || !third.End.Equal(time.Date(2026, 10, 12, 7, 15, 0, 0, time.UTC)) {
		t.Errorf("an occurrence = %+v, %v", third, err)
	}
	moved := byName["Standup (late)"]
	ov, err := e.c.Calendar().Event(ctx, &api.CalendarEventParams{ID: moved.EventID, RecurrenceID: ptr(moved.RecurrenceID)})
	if err != nil || ov.ID != moved.EventID || ov.RecurrenceID != "2026-10-10T07:00:00.000Z" || !ov.Recurring ||
		!ov.Start.Equal(time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)) || ov.Summary != "Standup (late)" {
		t.Errorf("an override = %+v, %v", ov, err)
	}

	review, err := e.c.Calendar().Event(ctx, &api.CalendarEventParams{ID: byName["Review"].EventID})
	if err != nil || !slices.Equal(review.Alarms, []int64{-55}) || review.TimeZone != "" || review.Answer != nil || len(review.Attendees) != 0 {
		t.Errorf("review = %+v, %v; an alarm 5 minutes before the end is 55 after the start", review, err)
	}
	dentist, err := e.c.Calendar().Event(ctx, &api.CalendarEventParams{ID: byName["Dentist"].EventID})
	if err != nil || !dentist.ReadOnly || !slices.Equal(dentist.Alarms, []int64{60}) || dentist.Organizer != nil {
		t.Errorf("dentist = %+v, %v", dentist, err)
	}
	holiday, err := e.c.Calendar().Event(ctx, &api.CalendarEventParams{ID: byName["Holiday"].EventID})
	if err != nil || !holiday.AllDay || holiday.StartDate != "2026-10-12" || holiday.EndDate != "2026-10-13" || holiday.Recurring {
		t.Errorf("holiday = %+v, %v", holiday, err)
	}

	for _, p := range []api.CalendarEventParams{
		{ID: 999999},
		{ID: byName["Dentist"].EventID, RecurrenceID: ptr("2026-10-09T14:00:00.000Z")}, // not a series
		{ID: master, RecurrenceID: ptr("yesterday")},
	} {
		if _, err := e.c.Calendar().Event(ctx, &p); code(err) != api.CodeNotFound {
			t.Errorf("event %+v: %v", p, err)
		}
	}
}

func TestContactCardUpcoming(t *testing.T) {
	e := newCalendarEnv(t)
	upcoming := func(email string) []string {
		t.Helper()
		card, err := e.c.People().Card(t.Context(), &api.PeopleCardParams{Email: email})
		if err != nil {
			t.Fatal(err)
		}
		if card.Upcoming == nil {
			t.Errorf("%s: upcoming is nil", email)
		}
		return lines(card.Upcoming)
	}
	if got := upcoming("maria@example.com"); !slices.Equal(got, []string{
		"10-09T07:00 Standup", "10-10T08:00 Standup (late)", "10-11T07:00 Standup", "10-12T07:00 Standup", "10-13T07:00 Standup",
	}) {
		t.Errorf("as organizer, at most 5 = %q", got)
	}
	if got := upcoming("BOB@example.com"); !slices.Equal(got, []string{
		"10-09T07:00 Standup", "10-11T07:00 Standup", "10-12T07:00 Standup", "10-13T07:00 Standup", "10-14T07:00 Standup",
	}) {
		t.Errorf("as attendee = %q; the override has no attendees", got)
	}
	if got := upcoming("carol@example.com"); !slices.Equal(got, []string{
		"10-12T15:00 Review", "10-19T15:00 Review", "10-26T15:00 Review", "11-02T15:00 Review",
	}) {
		t.Errorf("within 30 days = %q", got)
	}
	if got := upcoming("stranger@example.com"); len(got) != 0 {
		t.Errorf("stranger = %q", got)
	}
}
