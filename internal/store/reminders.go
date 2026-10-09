package store

import (
	"context"
	"errors"
	"time"
)

// errRemindersNotYet marks what task card T-0075 has yet to write.
var errRemindersNotYet = errors.New("store: reminders not implemented yet")

// AlarmKey names one alarm of one occurrence: a reminders row's key. It
// survives resyncs, and an event moved to a new time gets a new trigger.
type AlarmKey struct {
	CollectionID int64
	UID          string
	// RecurrenceID is the occurrence's ("" for a single event).
	RecurrenceID string
	TriggerAt    time.Time
}

// ReminderRow is a fired reminder with its occurrence.
type ReminderRow struct {
	AlarmKey
	FiredAt      time.Time
	SnoozedUntil *time.Time
	// DueAt is when it is shown: TriggerAt, or SnoozedUntil once a snooze
	// ended.
	DueAt    time.Time
	Event    EventRow // without attendees and alarms
	Instance InstanceRow
}

// Task T-0075 writes the functions below.

// AlarmsDue returns the alarms of shown occurrences whose trigger is in
// (since, until] and that have not fired, by trigger.
func (d *DB) AlarmsDue(ctx context.Context, since, until time.Time, local *time.Location) ([]AlarmKey, error) {
	return nil, errRemindersNotYet
}

// FireReminders records alarms as fired.
func (t *Tx) FireReminders(ctx context.Context, keys []AlarmKey, firedAt time.Time) error {
	return errRemindersNotYet
}

// ActiveReminders returns the fired reminders to show at now.
func (d *DB) ActiveReminders(ctx context.Context, now time.Time) ([]ReminderRow, error) {
	return nil, errRemindersNotYet
}

// SnoozesEnded returns the reminders whose snooze ended in (since, until].
func (d *DB) SnoozesEnded(ctx context.Context, since, until time.Time) ([]AlarmKey, error) {
	return nil, errRemindersNotYet
}

// SnoozeReminders shows reminders again at until.
func (t *Tx) SnoozeReminders(ctx context.Context, keys []AlarmKey, until time.Time) (int, error) {
	return 0, errRemindersNotYet
}

// DismissReminders stops showing reminders.
func (t *Tx) DismissReminders(ctx context.Context, keys []AlarmKey, at time.Time) (int, error) {
	return 0, errRemindersNotYet
}
