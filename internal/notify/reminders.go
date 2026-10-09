package notify

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
)

// Reminder is what a reminder's notification shows (docs/design/pim.md,
// Reminders).
type Reminder struct {
	ID      string // calendar.reminders' id
	Summary string // the event's title
	Body    string // its time and place
}

// Reminder actions, besides "default" (open the app).
const (
	ActionSnooze  = "snooze"
	ActionDismiss = "dismiss"
)

// OnReminder sets what clicking a reminder's notification or one of its
// actions does: action is "default", ActionSnooze or ActionDismiss.
func (d *Desktop) OnReminder(f func(id, action string)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onReminder = f
}

// Remind shows a notification per reminder, replacing one still shown for
// the same reminder.
func (d *Desktop) Remind(ctx context.Context, rs []Reminder) error {
	for _, r := range rs {
		d.mu.Lock()
		replaces := d.reminderNotes[r.ID]
		d.mu.Unlock()
		id, err := d.sendReminder(ctx, replaces, r)
		if err != nil {
			return err
		}
		d.mu.Lock()
		d.reminderNotes[r.ID] = id
		d.reminders[id] = r.ID
		d.mu.Unlock()
	}
	return nil
}

// Withdraw closes the reminder notifications whose reminders are not in
// active: snoozed or dismissed elsewhere.
func (d *Desktop) Withdraw(ctx context.Context, active map[string]bool) error {
	d.mu.Lock()
	var gone []uint32
	for id, note := range d.reminderNotes {
		if !active[id] {
			gone = append(gone, note)
			delete(d.reminderNotes, id)
			delete(d.reminders, note)
		}
	}
	d.mu.Unlock()
	for _, note := range gone {
		ctx, cancel := context.WithTimeout(ctx, callLimit)
		err := d.conn.Object(busName, busPath).CallWithContext(ctx, busIface+".CloseNotification", 0, note).Err
		cancel()
		if err != nil {
			return fmt.Errorf("close notification: %w", err)
		}
	}
	return nil
}

func (d *Desktop) sendReminder(ctx context.Context, replaces uint32, r Reminder) (uint32, error) {
	ctx, cancel := context.WithTimeout(ctx, callLimit)
	defer cancel()
	hints := map[string]dbus.Variant{
		"desktop-entry": dbus.MakeVariant(appID),
		"category":      dbus.MakeVariant("x-frostmail.reminder"),
		"urgency":       dbus.MakeVariant(byte(1)),
	}
	actions := []string{"default", "Open", ActionSnooze, "Snooze", ActionDismiss, "Dismiss"}
	var id uint32
	err := d.conn.Object(busName, busPath).CallWithContext(ctx, busIface+".Notify", 0,
		appName, replaces, appID, r.Summary, r.Body, actions, hints, int32(0)).Store(&id)
	if err != nil {
		return 0, fmt.Errorf("notify: %w", err)
	}
	return id, nil
}

// reminderAction runs the reminder handler for a notification's action and
// reports whether the notification was a reminder's.
func (d *Desktop) reminderAction(note uint32, action string, closed bool) bool {
	d.mu.Lock()
	id, ok := d.reminders[note]
	if ok && closed {
		delete(d.reminders, note)
		delete(d.reminderNotes, id)
	}
	f := d.onReminder
	d.mu.Unlock()
	if ok && !closed && f != nil {
		f(id, action)
	}
	return ok
}
