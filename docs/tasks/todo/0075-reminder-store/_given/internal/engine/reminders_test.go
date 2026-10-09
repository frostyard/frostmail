package engine_test

// CONTRACT TEST for task card T-0075 (docs/tasks). Do not edit.

import (
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

func TestReminderAPI(t *testing.T) {
	e := newCalendarEnv(t)
	ctx := t.Context()
	none, err := e.c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{})
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("reminders before any fired = %+v, %v", none, err)
	}
	// What maild's scheduler does when the alarms come due: the standup's
	// (10 minutes before 9:00 Berlin) and the dentist's (13:00 UTC).
	due, err := e.srv.DB.AlarmsDue(ctx, time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC), time.UTC)
	if err != nil || len(due) != 2 {
		t.Fatalf("due = %+v, %v", due, err)
	}
	if err := e.srv.DB.Tx(ctx, func(tx *store.Tx) error { return tx.FireReminders(ctx, due, time.Date(2026, 10, 9, 13, 0, 0, 0, time.UTC)) }); err != nil {
		t.Fatal(err)
	}

	list, err := e.c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{})
	if err != nil || len(list) != 2 {
		t.Fatalf("reminders = %+v, %v", list, err)
	}
	standup, dentist := list[0], list[1]
	if standup.ID == "" || standup.ID == dentist.ID || standup.Summary != "Standup" || standup.Location != "Room 4" ||
		standup.CalendarID != e.work || standup.RecurrenceID != "2026-10-09T07:00:00.000Z" || standup.AllDay ||
		!standup.Start.Equal(time.Date(2026, 10, 9, 7, 0, 0, 0, time.UTC)) ||
		!standup.DueAt.Equal(time.Date(2026, 10, 9, 6, 50, 0, 0, time.UTC)) || standup.StartDate != "" {
		t.Errorf("standup = %+v", standup)
	}
	ev, err := e.c.Calendar().Event(ctx, &api.CalendarEventParams{ID: standup.EventID, RecurrenceID: &standup.RecurrenceID})
	if err != nil || ev.Summary != "Standup" {
		t.Errorf("the standup's event = %+v, %v", ev, err)
	}
	if dentist.Summary != "Dentist" || dentist.CalendarID != e.home || dentist.RecurrenceID != "" ||
		!dentist.DueAt.Equal(time.Date(2026, 10, 9, 13, 0, 0, 0, time.UTC)) {
		t.Errorf("dentist = %+v", dentist)
	}

	// Snoozed until after now (the clock reads 2026-10-08 12:00): hidden.
	until := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	if err := e.c.Calendar().Snooze(ctx, &api.CalendarSnoozeParams{IDs: []string{standup.ID}, Until: until}); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{}); len(list) != 1 || list[0].ID != dentist.ID {
		t.Errorf("after snoozing = %+v", list)
	}
	if err := e.c.Calendar().Dismiss(ctx, &api.CalendarDismissParams{IDs: []string{dentist.ID}}); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.c.Calendar().Reminders(ctx, &api.CalendarRemindersParams{}); len(list) != 0 {
		t.Errorf("after dismissing = %+v", list)
	}

	for _, ids := range [][]string{nil, {"garbage"}, {standup.ID, "!!"}} {
		if err := e.c.Calendar().Dismiss(ctx, &api.CalendarDismissParams{IDs: ids}); code(err) != api.CodeInvalidParams {
			t.Errorf("dismiss %q: %v", ids, err)
		}
		if err := e.c.Calendar().Snooze(ctx, &api.CalendarSnoozeParams{IDs: ids, Until: until}); code(err) != api.CodeInvalidParams {
			t.Errorf("snooze %q: %v", ids, err)
		}
	}
}
