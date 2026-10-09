package notify

import (
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/dbustest"
)

type reminderClick struct{ id, action string }

func TestReminderNotes(t *testing.T) {
	f, addr := startFake(t)
	d, err := NewDesktop(t.Context(), dbustest.Connect(t, addr), slog.New(slog.DiscardHandler), func(int64) {})
	if err != nil {
		t.Fatal(err)
	}
	clicks := make(chan reminderClick, 4)
	d.OnReminder(func(id, action string) { clicks <- reminderClick{id, action} })
	ctx := t.Context()
	if err := d.Remind(ctx, []Reminder{{ID: "r1", Summary: "Standup", Body: "9:30 – 9:45 AM · Zoom"}, {ID: "r2", Summary: "Dentist"}}); err != nil {
		t.Fatal(err)
	}
	if err := d.Remind(ctx, []Reminder{{ID: "r1", Summary: "Standup", Body: "again"}}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	calls := slices.Clone(f.calls)
	f.mu.Unlock()
	if len(calls) != 3 || calls[0].summary != "Standup" || calls[0].body != "9:30 – 9:45 AM · Zoom" || calls[0].category != "x-frostmail.reminder" ||
		!slices.Equal(calls[0].actions, []string{"default", "Open", "snooze", "Snooze", "dismiss", "Dismiss"}) || calls[0].replaces != 0 {
		t.Fatalf("calls = %+v", calls)
	}
	if calls[2].replaces != 1 {
		t.Errorf("showing r1 again replaces %d, want its note 1", calls[2].replaces)
	}

	click := func(want reminderClick) {
		t.Helper()
		select {
		case got := <-clicks:
			if got != want {
				t.Errorf("click = %+v, want %+v", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no click %+v", want)
		}
	}
	f.emit(t, "ActionInvoked", uint32(1), "snooze")
	click(reminderClick{"r1", ActionSnooze})
	f.emit(t, "ActionInvoked", uint32(2), "default")
	click(reminderClick{"r2", "default"})

	// Withdrawing closes the notes of reminders no longer active.
	if err := d.Withdraw(ctx, map[string]bool{"r2": true}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	closed := slices.Clone(f.closed)
	f.mu.Unlock()
	if !slices.Equal(closed, []uint32{1}) {
		t.Errorf("closed = %v", closed)
	}
	// A note the user closed is forgotten: its actions do nothing, and a
	// later dismiss does not close it again.
	f.emit(t, "NotificationClosed", uint32(2), uint32(2))
	f.emit(t, "ActionInvoked", uint32(2), "dismiss")
	f.emit(t, "ActionInvoked", uint32(1), "dismiss")
	if err := d.Remind(ctx, []Reminder{{ID: "r3", Summary: "Marker"}}); err != nil {
		t.Fatal(err)
	}
	f.emit(t, "ActionInvoked", uint32(3), "dismiss")
	click(reminderClick{"r3", ActionDismiss})
	if err := d.Withdraw(ctx, map[string]bool{"r3": true}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Equal(f.closed, []uint32{1}) {
		t.Errorf("closed after forgetting = %v", f.closed)
	}
}
