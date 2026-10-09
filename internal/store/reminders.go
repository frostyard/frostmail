package store

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"
)

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

// RemindersChecked returns when the reminder scheduler last checked; ok is
// false when it never has.
func (d *DB) RemindersChecked(ctx context.Context) (at time.Time, ok bool, err error) {
	var s string
	err = d.db.QueryRowContext(ctx, "SELECT checked_at FROM reminders_checked WHERE id = 1").Scan(&s)
	if err == sql.ErrNoRows {
		return at, false, nil
	}
	if err != nil {
		return at, false, fmt.Errorf("reminders checked: %w", err)
	}
	at, err = ParseTime(s)
	return at, err == nil, err
}

// SetRemindersChecked records the reminder scheduler's check at at.
func (t *Tx) SetRemindersChecked(ctx context.Context, at time.Time) error {
	_, err := t.ExecContext(ctx, `INSERT INTO reminders_checked (id, checked_at) VALUES (1, ?)
 ON CONFLICT(id) DO UPDATE SET checked_at = excluded.checked_at`, FormatTime(at))
	if err != nil {
		return fmt.Errorf("set reminders checked: %w", err)
	}
	return nil
}

// AlarmsDue returns the alarms of shown occurrences whose trigger is in
// (since, until] and that have not fired, by trigger.
func (d *DB) AlarmsDue(ctx context.Context, since, until time.Time, local *time.Location) ([]AlarmKey, error) {
	candidates, err := d.alarmCandidates(ctx, since, until, local)
	if err != nil {
		return nil, err
	}
	fired, err := d.reminderKeys(ctx, `WHERE trigger_at > ? AND trigger_at <= ? ORDER BY trigger_at, collection_id, uid, recurrence_id`, since, until)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(candidates, compareAlarmKeys)
	candidates = slices.CompactFunc(candidates, func(a, b AlarmKey) bool { return compareAlarmKeys(a, b) == 0 })
	out := make([]AlarmKey, 0, len(candidates))
	for _, key := range candidates {
		if _, found := slices.BinarySearchFunc(fired, key, compareAlarmKeys); !found {
			out = append(out, key)
		}
	}
	return out, nil
}

func compareAlarmKeys(a, b AlarmKey) int {
	if n := a.TriggerAt.Compare(b.TriggerAt); n != 0 {
		return n
	}
	if n := cmp.Compare(a.CollectionID, b.CollectionID); n != 0 {
		return n
	}
	if n := cmp.Compare(a.UID, b.UID); n != 0 {
		return n
	}
	return cmp.Compare(a.RecurrenceID, b.RecurrenceID)
}

func (d *DB) alarmCandidates(ctx context.Context, since, until time.Time, local *time.Location) ([]AlarmKey, error) {
	upper, lower := until.Add(30*24*time.Hour), since.Add(-24*time.Hour)
	query := `SELECT col.id, e.uid, i.recurrence_id, e.recurrence_id, i.all_day,
 i.start_at, i.end_at, al.offset_seconds, al.related_end, al.trigger_at` + eventJoins + `
 JOIN instances i ON i.event_id = e.id JOIN alarms al ON al.event_id = e.id
 WHERE col.kind = 'calendar'` + visibleEvents + ` AND
 ((i.all_day = 0 AND i.start_at <= ? AND i.end_at >= ?)
 OR (i.all_day = 1 AND i.start_at <= ? AND i.end_at >= ?))`
	rows, err := d.db.QueryContext(ctx, query, FormatTime(upper), FormatTime(lower),
		upper.In(local).Format(time.DateOnly), lower.In(local).Format(time.DateOnly))
	if err != nil {
		return nil, fmt.Errorf("alarm candidates: %w", err)
	}
	defer rows.Close()
	out := []AlarmKey{}
	for rows.Next() {
		key, ok, err := scanAlarmCandidate(rows, local)
		if err != nil {
			return nil, fmt.Errorf("read alarm candidate: %w", err)
		}
		if ok && key.TriggerAt.After(since) && !key.TriggerAt.After(until) {
			out = append(out, key)
		}
	}
	return out, rows.Err()
}

func scanAlarmCandidate(rows *sql.Rows, local *time.Location) (AlarmKey, bool, error) {
	var key AlarmKey
	var eventRID, start, end string
	var allDay, relatedEnd bool
	var offset sql.NullInt64
	var absolute sql.NullString
	err := rows.Scan(&key.CollectionID, &key.UID, &key.RecurrenceID, &eventRID,
		&allDay, &start, &end, &offset, &relatedEnd, &absolute)
	if err != nil {
		return key, false, err
	}
	if absolute.Valid {
		if eventRID != key.RecurrenceID {
			return key, false, nil
		}
		key.TriggerAt, err = ParseTime(absolute.String)
		return key, err == nil, err
	}
	if !offset.Valid || offset.Int64 < -30*24*60*60 || offset.Int64 > 24*60*60 {
		return key, false, nil
	}
	a, b, err := parseEventTimes(start, end, allDay)
	if err != nil {
		return key, false, err
	}
	if relatedEnd {
		a = b
	}
	if allDay {
		a = time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, local)
	}
	key.TriggerAt = a.Add(time.Duration(offset.Int64) * time.Second).UTC()
	return key, true, nil
}

// FireReminders records alarms as fired.
func (t *Tx) FireReminders(ctx context.Context, keys []AlarmKey, firedAt time.Time) error {
	for _, key := range keys {
		_, err := t.ExecContext(ctx, `INSERT INTO reminders (collection_id, uid, recurrence_id, trigger_at, fired_at)
 VALUES (?, ?, ?, ?, ?) ON CONFLICT(collection_id, uid, recurrence_id, trigger_at) DO NOTHING`,
			key.CollectionID, key.UID, key.RecurrenceID, FormatTime(key.TriggerAt), FormatTime(firedAt))
		if err != nil {
			return fmt.Errorf("fire reminder: %w", err)
		}
	}
	return nil
}

// ActiveReminders returns the fired reminders to show at now.
func (d *DB) ActiveReminders(ctx context.Context, now time.Time) ([]ReminderRow, error) {
	query := `SELECT r.collection_id, r.uid, r.recurrence_id, r.trigger_at, r.fired_at, r.snoozed_until,
 i.all_day, i.start_at, i.end_at, ` + eventColumns + eventJoins + `
 JOIN instances i ON i.event_id = e.id JOIN reminders r ON r.collection_id = col.id
 AND r.uid = e.uid AND r.recurrence_id = i.recurrence_id
 WHERE col.kind = 'calendar'` + visibleEvents + ` AND r.dismissed_at IS NULL
 AND (r.snoozed_until IS NULL OR r.snoozed_until <= ?)
 ORDER BY COALESCE(r.snoozed_until, r.trigger_at), r.trigger_at, r.collection_id, r.uid, r.recurrence_id`
	rows, err := d.db.QueryContext(ctx, query, FormatTime(now))
	if err != nil {
		return nil, fmt.Errorf("active reminders: %w", err)
	}
	defer rows.Close()
	out := []ReminderRow{}
	for rows.Next() {
		row, err := scanReminder(rows)
		if err != nil {
			return nil, fmt.Errorf("read active reminder: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func scanReminder(rows *sql.Rows) (ReminderRow, error) {
	var r ReminderRow
	var trigger, fired, start, end, es, ee string
	var snooze sql.NullString
	dest := []any{&r.CollectionID, &r.UID, &r.RecurrenceID, &trigger, &fired, &snooze,
		&r.Instance.AllDay, &start, &end}
	if err := rows.Scan(append(dest, eventScan(&r.Event, &es, &ee)...)...); err != nil {
		return r, err
	}
	var err error
	if r.TriggerAt, err = ParseTime(trigger); err != nil {
		return r, err
	}
	if r.FiredAt, err = ParseTime(fired); err != nil {
		return r, err
	}
	r.DueAt = r.TriggerAt
	if snooze.Valid {
		value, err := ParseTime(snooze.String)
		if err != nil {
			return r, err
		}
		r.SnoozedUntil, r.DueAt = &value, value
	}
	r.Instance.EventID, r.Instance.RecurrenceID = r.Event.ID, r.RecurrenceID
	r.Instance.Start, r.Instance.End, err = parseEventTimes(start, end, r.Instance.AllDay)
	if err != nil {
		return r, err
	}
	r.Event.Start, r.Event.End, err = parseEventTimes(es, ee, r.Event.AllDay)
	return r, err
}

// SnoozesEnded returns the reminders whose snooze ended in (since, until].
func (d *DB) SnoozesEnded(ctx context.Context, since, until time.Time) ([]AlarmKey, error) {
	return d.reminderKeys(ctx, `WHERE dismissed_at IS NULL AND snoozed_until > ? AND snoozed_until <= ?
 ORDER BY snoozed_until, trigger_at, collection_id, uid, recurrence_id`, since, until)
}

func (d *DB) reminderKeys(ctx context.Context, clause string, since, until time.Time) ([]AlarmKey, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT collection_id, uid, recurrence_id, trigger_at FROM reminders `+clause,
		FormatTime(since), FormatTime(until))
	if err != nil {
		return nil, fmt.Errorf("reminder keys: %w", err)
	}
	defer rows.Close()
	out := []AlarmKey{}
	for rows.Next() {
		var key AlarmKey
		var trigger string
		if err := rows.Scan(&key.CollectionID, &key.UID, &key.RecurrenceID, &trigger); err != nil {
			return nil, fmt.Errorf("read reminder key: %w", err)
		}
		key.TriggerAt, err = ParseTime(trigger)
		if err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

// SnoozeReminders shows reminders again at until.
func (t *Tx) SnoozeReminders(ctx context.Context, keys []AlarmKey, until time.Time) (int, error) {
	return t.updateReminders(ctx, keys, "snoozed_until", until)
}

// DismissReminders stops showing reminders.
func (t *Tx) DismissReminders(ctx context.Context, keys []AlarmKey, at time.Time) (int, error) {
	return t.updateReminders(ctx, keys, "dismissed_at", at)
}

func (t *Tx) updateReminders(ctx context.Context, keys []AlarmKey, column string, at time.Time) (int, error) {
	count := 0
	seen := map[[4]string]bool{}
	for _, key := range keys {
		identity := [4]string{fmt.Sprint(key.CollectionID), key.UID, key.RecurrenceID, FormatTime(key.TriggerAt)}
		if seen[identity] {
			continue
		}
		seen[identity] = true
		result, err := t.ExecContext(ctx, `UPDATE reminders SET `+column+` = ? WHERE dismissed_at IS NULL
 AND collection_id = ? AND uid = ? AND recurrence_id = ? AND trigger_at = ?`,
			FormatTime(at), key.CollectionID, key.UID, key.RecurrenceID, FormatTime(key.TriggerAt))
		if err != nil {
			return 0, fmt.Errorf("update reminder: %w", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count updated reminders: %w", err)
		}
		count += int(n)
	}
	return count, nil
}
