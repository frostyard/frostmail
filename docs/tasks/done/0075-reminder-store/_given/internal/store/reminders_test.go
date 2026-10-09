package store

// CONTRACT TEST for task card T-0075 (docs/tasks). Do not edit.

import (
	"slices"
	"testing"
	"time"
)

// reminderFixture is newCalendarFixture with alarms: a daily standup with
// an override, a single event, an all-day birthday, and events in the
// hidden and switched-off calendars.
func reminderFixture(t *testing.T) *calendarFixture {
	t.Helper()
	f := newCalendarFixture(t)
	minutes := func(m int) *time.Duration { d := time.Duration(m) * time.Minute; return &d }
	instant := func(s string) *time.Time { v := at(s); return &v }
	f.add(t, f.a, "/a/s.ics", []EventRow{
		{UID: "s", Summary: "Standup", Start: at("2026-10-08T09:00Z"), End: at("2026-10-08T09:15Z"),
			Recurrence: "RRULE:FREQ=DAILY;COUNT=3", Status: "confirmed", Alarms: []AlarmRow{
				{Action: "display", Offset: minutes(-10)},
				{Action: "display", Offset: minutes(-5), RelatedEnd: true},
				{Action: "display", At: instant("2026-10-08T06:00Z")}, // a series' absolute alarm: ignored
			}},
		{UID: "s", RecurrenceID: "2026-10-10T09:00:00.000Z", Summary: "Standup (late)", Start: at("2026-10-10T11:00Z"),
			End: at("2026-10-10T11:15Z"), Status: "confirmed", Alarms: []AlarmRow{{Action: "audio", Offset: minutes(-5)}}},
	},
		InstanceRow{EventID: 0, RecurrenceID: "2026-10-08T09:00:00.000Z", Start: at("2026-10-08T09:00Z"), End: at("2026-10-08T09:15Z")},
		InstanceRow{EventID: 0, RecurrenceID: "2026-10-09T09:00:00.000Z", Start: at("2026-10-09T09:00Z"), End: at("2026-10-09T09:15Z")},
		InstanceRow{EventID: 1, RecurrenceID: "2026-10-10T09:00:00.000Z", Start: at("2026-10-10T11:00Z"), End: at("2026-10-10T11:15Z")},
	)
	f.add(t, f.b, "/b/one.ics", []EventRow{{UID: "one", Summary: "Single", Start: at("2026-10-09T14:00Z"), End: at("2026-10-09T15:00Z"),
		Status: "confirmed", Alarms: []AlarmRow{
			{Action: "display", Offset: minutes(-15)},
			{Action: "display", At: instant("2026-10-09T12:00Z")},
			{Action: "display", Offset: minutes(-40 * 24 * 60)}, // more than 30 days before: ignored
		}}})
	// 9:00 the day before, in New York: midnight there is 04:00 UTC.
	f.add(t, f.a, "/a/b.ics", []EventRow{{UID: "bday", Summary: "Birthday", AllDay: true, Start: day("2026-10-09"),
		End: day("2026-10-10"), Status: "confirmed", Alarms: []AlarmRow{{Action: "display", Offset: minutes(-15 * 60)}}}})
	for _, c := range []Collection{f.c, f.x} {
		f.add(t, c, "/h.ics", []EventRow{{UID: "hidden", Summary: "Hidden", Start: at("2026-10-09T10:00Z"), End: at("2026-10-09T11:00Z"),
			Status: "confirmed", Alarms: []AlarmRow{{Action: "display", Offset: minutes(-10)}}}})
	}
	return f
}

var newYork = func() *time.Location {
	l, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return l
}()

// same compares keys by value; their instants with Equal.
func same(a, b AlarmKey) bool {
	return a.CollectionID == b.CollectionID && a.UID == b.UID && a.RecurrenceID == b.RecurrenceID && a.TriggerAt.Equal(b.TriggerAt)
}

func keys(alarms []AlarmKey) []string {
	var out []string
	for _, a := range alarms {
		out = append(out, a.UID+" "+a.RecurrenceID+" "+a.TriggerAt.UTC().Format("01-02T15:04"))
	}
	return out
}

func TestAlarmsDue(t *testing.T) {
	f := reminderFixture(t)
	ctx := t.Context()
	due, err := f.d.AlarmsDue(ctx, at("2026-10-08T00:00Z"), at("2026-10-11T00:00Z"), newYork)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"s 2026-10-08T09:00:00.000Z 10-08T08:50",
		"s 2026-10-08T09:00:00.000Z 10-08T09:10",
		"bday  10-08T13:00",
		"s 2026-10-09T09:00:00.000Z 10-09T08:50",
		"s 2026-10-09T09:00:00.000Z 10-09T09:10",
		"one  10-09T12:00",
		"one  10-09T13:45",
		"s 2026-10-10T09:00:00.000Z 10-10T10:55",
	}
	if got := keys(due); !slices.Equal(got, want) {
		t.Fatalf("due:\n%q\nwant\n%q", got, want)
	}
	if due[0].CollectionID != f.a.ID || due[5].CollectionID != f.b.ID {
		t.Errorf("collections = %d, %d", due[0].CollectionID, due[5].CollectionID)
	}
	// The window is (since, until].
	edge, _ := f.d.AlarmsDue(ctx, at("2026-10-08T08:50Z"), at("2026-10-08T09:10Z"), newYork)
	if got := keys(edge); !slices.Equal(got, []string{"s 2026-10-08T09:00:00.000Z 10-08T09:10"}) {
		t.Errorf("edges = %q", got)
	}
	// Fired alarms are not due again.
	if err := f.d.Tx(ctx, func(tx *Tx) error { return tx.FireReminders(ctx, due[:2], at("2026-10-08T09:10Z")) }); err != nil {
		t.Fatal(err)
	}
	if err := f.d.Tx(ctx, func(tx *Tx) error { return tx.FireReminders(ctx, due[:1], at("2026-10-08T09:30Z")) }); err != nil {
		t.Fatalf("firing again: %v", err)
	}
	if again, _ := f.d.AlarmsDue(ctx, at("2026-10-08T00:00Z"), at("2026-10-08T12:00Z"), newYork); len(again) != 0 {
		t.Errorf("due after firing = %q", keys(again))
	}
}

func TestReminderLifecycle(t *testing.T) {
	f := reminderFixture(t)
	ctx := t.Context()
	due, _ := f.d.AlarmsDue(ctx, at("2026-10-08T00:00Z"), at("2026-10-11T00:00Z"), newYork)
	first, second, single := due[0], due[1], due[6]
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		return tx.FireReminders(ctx, []AlarmKey{first, second, single}, at("2026-10-09T13:45Z"))
	}); err != nil {
		t.Fatal(err)
	}
	active := func(now string) []ReminderRow {
		t.Helper()
		rows, err := f.d.ActiveReminders(ctx, at(now))
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	order := func(rows []ReminderRow) []string {
		var out []string
		for _, r := range rows {
			out = append(out, r.UID+" "+r.DueAt.UTC().Format("01-02T15:04"))
		}
		return out
	}
	rows := active("2026-10-09T13:45Z")
	if got := order(rows); !slices.Equal(got, []string{"s 10-08T08:50", "s 10-08T09:10", "one 10-09T13:45"}) {
		t.Fatalf("active = %q", got)
	}
	r := rows[0]
	if !same(r.AlarmKey, first) || !r.FiredAt.Equal(at("2026-10-09T13:45Z")) || r.SnoozedUntil != nil ||
		r.Event.Summary != "Standup" || r.Event.CalendarID != f.a.ID || r.Event.AccountID != f.one.ID ||
		!r.Instance.Start.Equal(at("2026-10-08T09:00Z")) || r.Instance.RecurrenceID != "2026-10-08T09:00:00.000Z" {
		t.Errorf("first = %+v", r)
	}
	if rows[2].Event.Summary != "Single" || !rows[2].Instance.Start.Equal(at("2026-10-09T14:00Z")) {
		t.Errorf("single = %+v", rows[2])
	}

	// A snoozed reminder hides until its snooze ends, then shows by it.
	var n int
	if err := f.d.Tx(ctx, func(tx *Tx) (err error) {
		n, err = tx.SnoozeReminders(ctx, []AlarmKey{first}, at("2026-10-09T14:30Z"))
		return err
	}); err != nil || n != 1 {
		t.Fatalf("snooze = %d, %v", n, err)
	}
	if got := order(active("2026-10-09T14:00Z")); !slices.Equal(got, []string{"s 10-08T09:10", "one 10-09T13:45"}) {
		t.Errorf("while snoozed = %q", got)
	}
	after := active("2026-10-09T14:30Z")
	if got := order(after); !slices.Equal(got, []string{"s 10-08T09:10", "one 10-09T13:45", "s 10-09T14:30"}) {
		t.Errorf("after the snooze = %q", got)
	}
	if s := after[2]; !same(s.AlarmKey, first) || s.SnoozedUntil == nil || !s.SnoozedUntil.Equal(at("2026-10-09T14:30Z")) {
		t.Errorf("the snoozed one = %+v", s)
	}
	ended, _ := f.d.SnoozesEnded(ctx, at("2026-10-09T14:00Z"), at("2026-10-09T14:30Z"))
	if len(ended) != 1 || !same(ended[0], first) {
		t.Errorf("snoozes ended = %+v", ended)
	}
	if ended, _ := f.d.SnoozesEnded(ctx, at("2026-10-09T14:30Z"), at("2026-10-09T15:00Z")); len(ended) != 0 {
		t.Errorf("snoozes ended later = %+v", ended)
	}

	// Dismissed reminders are gone; unknown keys change nothing.
	if err := f.d.Tx(ctx, func(tx *Tx) (err error) {
		n, err = tx.DismissReminders(ctx, []AlarmKey{second, {CollectionID: f.a.ID, UID: "nope", TriggerAt: at("2026-10-08T09:00Z")}},
			at("2026-10-09T14:31Z"))
		return err
	}); err != nil || n != 1 {
		t.Fatalf("dismiss = %d, %v", n, err)
	}
	if got := order(active("2026-10-09T14:31Z")); !slices.Equal(got, []string{"one 10-09T13:45", "s 10-09T14:30"}) {
		t.Errorf("after dismissing = %q", got)
	}

	// A reminder whose occurrence went, or whose calendar is hidden, is not shown.
	obj, _ := f.d.ObjectByHref(ctx, f.b.ID, "/b/one.ics")
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.IndexEvents(ctx, obj.ID, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := order(active("2026-10-09T14:31Z")); !slices.Equal(got, []string{"s 10-09T14:30"}) {
		t.Errorf("after the event went = %q", got)
	}
	off := false
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.UpdateCollection(ctx, f.a.ID, &off, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := active("2026-10-09T14:31Z"); len(got) != 0 {
		t.Errorf("with the calendar hidden = %+v", got)
	}
}
