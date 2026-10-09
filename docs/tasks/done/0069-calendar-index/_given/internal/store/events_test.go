package store

// CONTRACT TEST for task card T-0069 (docs/tasks). Do not edit.

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

func at(s string) time.Time {
	t, err := time.Parse("2006-01-02T15:04Z", s)
	if err != nil {
		panic(err)
	}
	return t
}

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// calendarFixture is two accounts' calendars: a and b enabled in account
// one (calendar on), c disabled, and d in account two (calendar off).
type calendarFixture struct {
	d          *DB
	one, two   Account
	a, b, c, x Collection
}

func newCalendarFixture(t *testing.T) *calendarFixture {
	t.Helper()
	d, _ := openTest(t)
	ctx := t.Context()
	f := &calendarFixture{d: d}
	f.one = insertAccount(t, d, sampleAccount("one@mailtest.test"))
	f.two = insertAccount(t, d, sampleAccount("two@mailtest.test"))
	if err := d.Tx(ctx, func(tx *Tx) error {
		if err := tx.SetService(ctx, f.one.ID, api.ServiceKindCalendar, true, "https://dav.test/"); err != nil {
			return err
		}
		return tx.SetService(ctx, f.two.ID, api.ServiceKindCalendar, false, "https://dav.test/")
	}); err != nil {
		t.Fatal(err)
	}
	cols := replaceCols(t, d, f.one.ID, api.CollectionKindCalendar,
		RemoteCollection{Href: "/a/", Name: "A"}, RemoteCollection{Href: "/b/", Name: "B", ReadOnly: true},
		RemoteCollection{Href: "/c/", Name: "C"})
	f.a, f.b, f.c = cols[0], cols[1], cols[2]
	f.x = replaceCols(t, d, f.two.ID, api.CollectionKindCalendar, RemoteCollection{Href: "/x/", Name: "X"})[0]
	off := false
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.UpdateCollection(ctx, f.c.ID, &off, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

// add stores an object with events and their instances (one each, at the
// event's own times, unless instances are given).
func (f *calendarFixture) add(t *testing.T, col Collection, href string, events []EventRow, instances ...InstanceRow) []int64 {
	t.Helper()
	ctx := t.Context()
	var ids []int64
	err := f.d.Tx(ctx, func(tx *Tx) error {
		obj, err := tx.PutObject(ctx, Object{CollectionID: col.ID, Href: href, Kind: ObjectVEvent, Raw: []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")})
		if err != nil {
			return err
		}
		if ids, err = tx.IndexEvents(ctx, obj, events); err != nil {
			return err
		}
		rows := instances
		if len(rows) == 0 {
			for i, ev := range events {
				rows = append(rows, InstanceRow{EventID: ids[i], RecurrenceID: ev.RecurrenceID, AllDay: ev.AllDay, Start: ev.Start, End: ev.End})
			}
		} else {
			for i := range rows {
				rows[i].EventID = ids[rows[i].EventID]
			}
		}
		return tx.ReplaceInstances(ctx, obj, rows)
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestIndexEvents(t *testing.T) {
	f := newCalendarFixture(t)
	ctx := t.Context()
	fifteen := -15 * time.Minute
	trigger := at("2026-10-09T13:00Z")
	master := EventRow{
		UID: "s", Summary: "Standup", Location: "Room 4", Description: "Daily", Start: at("2026-10-09T14:00Z"),
		End: at("2026-10-09T14:15Z"), TZID: "America/New_York", Recurrence: "RRULE:FREQ=DAILY", Status: "confirmed",
		Organizer: "maria@example.com", OrganizerName: "Maria", PartStat: "needsaction", Sequence: 3,
		Attendees: []AttendeeRow{
			{Email: "one@mailtest.test", Name: "One", Role: "required", PartStat: "needsaction"},
			{Email: "bob@example.com", Role: "optional", PartStat: "declined"},
		},
		Alarms: []AlarmRow{{Action: "display", Offset: &fifteen}, {Action: "audio", At: &trigger}},
	}
	override := EventRow{UID: "s", RecurrenceID: "2026-10-10T14:00:00.000Z", Summary: "Standup (moved)",
		Start: at("2026-10-10T15:00Z"), End: at("2026-10-10T15:15Z"), Status: "cancelled", Transparent: true}
	ids := f.add(t, f.b, "/b/s.ics", []EventRow{master, override})
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatalf("ids = %v", ids)
	}
	got, err := f.d.Event(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != ids[0] || got.Summary != "Standup" || got.Location != "Room 4" || got.Description != "Daily" ||
		!got.Start.Equal(master.Start) || !got.End.Equal(master.End) || got.TZID != "America/New_York" ||
		got.Recurrence != "RRULE:FREQ=DAILY" || got.Organizer != "maria@example.com" || got.OrganizerName != "Maria" ||
		got.PartStat != "needsaction" || got.Sequence != 3 || got.UID != "s" || got.Status != "confirmed" {
		t.Errorf("event = %+v", got)
	}
	if !slices.Equal(got.Attendees, master.Attendees) {
		t.Errorf("attendees = %+v", got.Attendees)
	}
	if len(got.Alarms) != 2 || got.Alarms[0].Offset == nil || *got.Alarms[0].Offset != fifteen || got.Alarms[0].At != nil ||
		got.Alarms[1].At == nil || !got.Alarms[1].At.Equal(trigger) || got.Alarms[1].Action != "audio" {
		t.Errorf("alarms = %+v", got.Alarms)
	}
	if got.CalendarID != f.b.ID || got.AccountID != f.one.ID || !got.ReadOnly {
		t.Errorf("calendar = %d/%d read-only %v", got.CalendarID, got.AccountID, got.ReadOnly)
	}
	ov, _ := f.d.Event(ctx, ids[1])
	if ov.RecurrenceID != "2026-10-10T14:00:00.000Z" || ov.Status != "cancelled" || !ov.Transparent {
		t.Errorf("override = %+v", ov)
	}

	// All-day dates round-trip as UTC midnights.
	holiday := f.add(t, f.a, "/a/h.ics", []EventRow{{UID: "h", AllDay: true, Start: day("2026-12-24"), End: day("2026-12-26"), Status: "confirmed"}})
	h, _ := f.d.Event(ctx, holiday[0])
	if !h.AllDay || !h.Start.Equal(day("2026-12-24")) || !h.End.Equal(day("2026-12-26")) || h.ReadOnly {
		t.Errorf("all-day = %+v", h)
	}

	// Indexing an object again replaces its events, attendees, alarms and
	// instances.
	obj, _ := f.d.ObjectByHref(ctx, f.b.ID, "/b/s.ics")
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.IndexEvents(ctx, obj.ID, []EventRow{{UID: "s", Summary: "Renamed", Start: master.Start, End: master.End, Status: "confirmed"}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.d.Event(ctx, ids[0]); !errors.Is(err, ErrNotFound) {
		t.Errorf("an old event: %v", err)
	}
	occ, _ := f.d.Occurrences(ctx, OccurrenceFilter{From: at("2026-10-01T00:00Z"), To: at("2026-11-01T00:00Z"),
		FromDate: "2026-10-01", ToDate: "2026-11-01"})
	if len(occ) != 0 {
		t.Errorf("instances of replaced events = %+v", occ)
	}
}

func TestOccurrences(t *testing.T) {
	f := newCalendarFixture(t)
	ctx := t.Context()
	timed := f.add(t, f.a, "/a/t.ics", []EventRow{{UID: "t", Summary: "Timed", Start: at("2026-10-09T14:00Z"), End: at("2026-10-09T15:00Z"),
		Status: "confirmed", Organizer: "maria@example.com"}})
	allDay := f.add(t, f.a, "/a/d.ics", []EventRow{{UID: "d", Summary: "Day", AllDay: true, Start: day("2026-10-09"), End: day("2026-10-10"),
		Status: "confirmed", Attendees: []AttendeeRow{{Email: "bob@example.com", Role: "required", PartStat: "accepted"}}}})
	zero := f.add(t, f.b, "/b/z.ics", []EventRow{{UID: "z", Summary: "Zero", Start: at("2026-10-09T16:00Z"), End: at("2026-10-09T16:00Z"), Status: "confirmed"}})
	f.add(t, f.c, "/c/h.ics", []EventRow{{UID: "h", Summary: "Hidden", Start: at("2026-10-09T10:00Z"), End: at("2026-10-09T11:00Z"), Status: "confirmed"}})
	f.add(t, f.x, "/x/o.ics", []EventRow{{UID: "o", Summary: "Off", Start: at("2026-10-09T10:00Z"), End: at("2026-10-09T11:00Z"), Status: "confirmed"}})
	// A daily series stored as three instances of one master.
	series := f.add(t, f.a, "/a/s.ics",
		[]EventRow{{UID: "s", Summary: "Daily", Start: at("2026-10-08T09:00Z"), End: at("2026-10-08T09:30Z"), Recurrence: "RRULE:FREQ=DAILY;COUNT=3", Status: "confirmed"}},
		InstanceRow{EventID: 0, RecurrenceID: "2026-10-08T09:00:00.000Z", Start: at("2026-10-08T09:00Z"), End: at("2026-10-08T09:30Z")},
		InstanceRow{EventID: 0, RecurrenceID: "2026-10-09T09:00:00.000Z", Start: at("2026-10-09T09:00Z"), End: at("2026-10-09T09:30Z")},
		InstanceRow{EventID: 0, RecurrenceID: "2026-10-10T09:00:00.000Z", Start: at("2026-10-10T09:00Z"), End: at("2026-10-10T09:30Z")},
	)
	oct9 := OccurrenceFilter{From: at("2026-10-09T00:00Z"), To: at("2026-10-10T00:00Z"), FromDate: "2026-10-09", ToDate: "2026-10-10"}
	run := func(filter OccurrenceFilter) []OccurrenceRow {
		t.Helper()
		occ, err := f.d.Occurrences(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		return occ
	}
	summaries := func(occ []OccurrenceRow) []string {
		var out []string
		for _, o := range occ {
			out = append(out, o.Event.Summary)
		}
		return out
	}
	got := run(oct9)
	if !slices.Equal(summaries(got), []string{"Day", "Daily", "Timed", "Zero"}) {
		t.Fatalf("Oct 9 = %q", summaries(got))
	}
	day0, daily, tm := got[0], got[1], got[2]
	if !day0.Instance.AllDay || !day0.Instance.Start.Equal(day("2026-10-09")) || day0.Event.ID != allDay[0] ||
		day0.Event.CalendarID != f.a.ID || day0.Event.AccountID != f.one.ID {
		t.Errorf("all-day occurrence = %+v", day0)
	}
	if daily.Instance.RecurrenceID != "2026-10-09T09:00:00.000Z" || daily.Event.ID != series[0] ||
		daily.Event.Recurrence != "RRULE:FREQ=DAILY;COUNT=3" {
		t.Errorf("series occurrence = %+v", daily)
	}
	if tm.Event.ID != timed[0] || !tm.Instance.End.Equal(at("2026-10-09T15:00Z")) {
		t.Errorf("timed occurrence = %+v", tm)
	}
	if got[3].Event.ID != zero[0] || !got[3].Event.ReadOnly {
		t.Errorf("zero-length occurrence = %+v", got[3])
	}

	// Edges: a timed event ending as the range starts is out, as is one
	// starting at To; an all-day event ending on FromDate or starting on
	// ToDate is out; a zero-length one at From is in.
	if edge := run(OccurrenceFilter{From: at("2026-10-09T15:00Z"), To: at("2026-10-09T16:00Z"),
		FromDate: "2026-10-10", ToDate: "2026-10-11"}); len(edge) != 0 {
		t.Errorf("edges = %q", summaries(edge))
	}
	if edge := run(OccurrenceFilter{From: at("2026-10-09T16:00Z"), To: at("2026-10-09T16:30Z"),
		FromDate: "2026-10-08", ToDate: "2026-10-09"}); !slices.Equal(summaries(edge), []string{"Zero"}) {
		t.Errorf("edges = %q", summaries(edge))
	}
	if b := run(OccurrenceFilter{From: oct9.From, To: oct9.To, FromDate: oct9.FromDate, ToDate: oct9.ToDate, CalendarIDs: []int64{f.b.ID}}); !slices.Equal(summaries(b), []string{"Zero"}) {
		t.Errorf("calendar B = %q", summaries(b))
	}
	with := OccurrenceFilter{From: at("2026-10-01T00:00Z"), To: at("2026-11-01T00:00Z"), FromDate: "2026-10-01", ToDate: "2026-11-01"}
	with.WithEmail = "maria@example.com"
	if w := run(with); !slices.Equal(summaries(w), []string{"Timed"}) {
		t.Errorf("with the organizer = %q", summaries(w))
	}
	with.WithEmail = "bob@example.com"
	if w := run(with); !slices.Equal(summaries(w), []string{"Day"}) {
		t.Errorf("with an attendee = %q", summaries(w))
	}
	limited := oct9
	limited.Limit = 2
	if l := run(limited); !slices.Equal(summaries(l), []string{"Day", "Daily"}) {
		t.Errorf("limit 2 = %q", summaries(l))
	}
}

func TestInstanceWindowAndEvents(t *testing.T) {
	f := newCalendarFixture(t)
	ctx := t.Context()
	if _, _, ok, err := f.d.InstanceWindow(ctx); ok || err != nil {
		t.Errorf("an unset window: ok %v, %v", ok, err)
	}
	if err := f.d.Tx(ctx, func(tx *Tx) error { return tx.SetInstanceWindow(ctx, day("2025-10-08"), day("2028-10-08")) }); err != nil {
		t.Fatal(err)
	}
	if err := f.d.Tx(ctx, func(tx *Tx) error { return tx.SetInstanceWindow(ctx, day("2025-10-09"), day("2028-10-09")) }); err != nil {
		t.Fatal(err)
	}
	from, to, ok, err := f.d.InstanceWindow(ctx)
	if !ok || err != nil || !from.Equal(day("2025-10-09")) || !to.Equal(day("2028-10-09")) {
		t.Errorf("window = %v–%v %v %v", from, to, ok, err)
	}
	f.add(t, f.a, "/a/1.ics", []EventRow{
		{UID: "1", Summary: "One", Start: at("2026-10-09T10:00Z"), End: at("2026-10-09T11:00Z"), Recurrence: "RRULE:FREQ=DAILY", TZID: "Europe/Berlin", Status: "confirmed",
			Attendees: []AttendeeRow{{Email: "bob@example.com", Role: "required", PartStat: "accepted"}}},
		{UID: "1", RecurrenceID: "2026-10-10T10:00:00.000Z", Summary: "Moved", Start: at("2026-10-10T12:00Z"), End: at("2026-10-10T13:00Z"), Status: "confirmed"},
	})
	f.add(t, f.b, "/b/2.ics", []EventRow{{UID: "2", Summary: "Two", AllDay: true, Start: day("2026-10-09"), End: day("2026-10-10"), Status: "confirmed"}})
	f.add(t, f.c, "/c/3.ics", []EventRow{{UID: "3", Summary: "Hidden", Start: at("2026-10-09T10:00Z"), End: at("2026-10-09T11:00Z"), Status: "confirmed"}})
	f.add(t, f.x, "/x/4.ics", []EventRow{{UID: "4", Summary: "Off", Start: at("2026-10-09T10:00Z"), End: at("2026-10-09T11:00Z"), Status: "confirmed"}})
	summaries := func(filter EventsFilter) []string {
		t.Helper()
		events, err := f.d.Events(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range events {
			out = append(out, e.Summary)
		}
		return out
	}
	if got := summaries(EventsFilter{}); !slices.Equal(got, []string{"One", "Moved", "Two", "Hidden", "Off"}) {
		t.Errorf("all events = %q", got)
	}
	if got := summaries(EventsFilter{Visible: true}); !slices.Equal(got, []string{"One", "Moved", "Two"}) {
		t.Errorf("visible events = %q", got)
	}
	if got := summaries(EventsFilter{Visible: true, CalendarIDs: []int64{f.b.ID, f.c.ID}}); !slices.Equal(got, []string{"Two"}) {
		t.Errorf("visible events in B and C = %q", got)
	}
	events, _ := f.d.Events(ctx, EventsFilter{CalendarIDs: []int64{f.a.ID}})
	one, _ := f.d.ObjectByHref(ctx, f.a.ID, "/a/1.ics")
	if len(events) != 2 {
		t.Fatalf("events in A = %+v", events)
	}
	e := events[0]
	if e.ID == 0 || e.ObjectID != one.ID || e.UID != "1" || e.Recurrence != "RRULE:FREQ=DAILY" || e.TZID != "Europe/Berlin" ||
		!e.Start.Equal(at("2026-10-09T10:00Z")) || e.CalendarID != f.a.ID || e.AccountID != f.one.ID || len(e.Attendees) != 0 {
		t.Errorf("event = %+v", e)
	}
	if events[1].RecurrenceID != "2026-10-10T10:00:00.000Z" || events[1].ObjectID != one.ID {
		t.Errorf("override = %+v", events[1])
	}
}
