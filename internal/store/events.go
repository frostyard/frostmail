package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// EventRow is an events row with its attendees and alarms: one VEVENT's
// index (internal/calendar builds it).
type EventRow struct {
	ID           int64 // set by reads and by IndexEvents
	ObjectID     int64
	UID          string
	RecurrenceID string
	Summary      string
	Location     string
	Description  string
	AllDay       bool
	// Start and End: instants, or UTC midnights for all-day events, which
	// are stored as YYYY-MM-DD.
	Start, End    time.Time
	TZID          string
	Recurrence    string
	Status        string // confirmed, tentative, cancelled
	Transparent   bool
	Organizer     string // lowercased email
	OrganizerName string
	PartStat      string // the user's answer, or ""
	Sequence      int
	Attendees     []AttendeeRow
	Alarms        []AlarmRow
	// Set by reads: the event's calendar and account, and whether either
	// is read-only.
	CalendarID int64
	AccountID  int64
	ReadOnly   bool
}

// AttendeeRow is an event_attendees row.
type AttendeeRow struct {
	Email    string
	Name     string
	Role     string // chair, required, optional, none
	PartStat string // needsaction, accepted, declined, tentative, delegated
}

// AlarmRow is an alarms row: relative (Offset set) or absolute (At set).
type AlarmRow struct {
	Action     string // display or audio
	Offset     *time.Duration
	RelatedEnd bool
	At         *time.Time
}

// InstanceRow is an instances row: one occurrence of an event.
type InstanceRow struct {
	EventID      int64
	RecurrenceID string
	AllDay       bool
	Start, End   time.Time // as EventRow's
}

// OccurrenceFilter selects stored occurrences.
type OccurrenceFilter struct {
	// From and To bound timed occurrences ([From, To) overlaps);
	// FromDate and ToDate (YYYY-MM-DD) bound all-day ones.
	From, To         time.Time
	FromDate, ToDate string
	// CalendarIDs, when not empty, keeps only these calendars.
	CalendarIDs []int64
	// WithEmail, when set, keeps only events whose organizer or an
	// attendee has this (lowercased) address.
	WithEmail string
	// Limit, when positive, returns at most this many.
	Limit int
}

// EventsFilter selects events to expand.
type EventsFilter struct {
	// Visible keeps only enabled calendars of accounts with the calendar
	// service on.
	Visible bool
	// CalendarIDs, when not empty, keeps only these calendars.
	CalendarIDs []int64
}

// OccurrenceRow is a stored occurrence with its event.
type OccurrenceRow struct {
	Instance InstanceRow
	Event    EventRow // without Attendees and Alarms
}

func eventTime(t time.Time, allDay bool) string {
	if allDay {
		return t.UTC().Format(time.DateOnly)
	}
	return FormatTime(t)
}

// IndexEvents stores an object's events, which have distinct recurrence
// IDs. An event whose recurrence ID the object already had keeps its ID,
// since clients hold event IDs; the object's other events are deleted, and
// its instances are dropped for ReplaceInstances to store again.
func (t *Tx) IndexEvents(ctx context.Context, objectID int64, events []EventRow) ([]int64, error) {
	if _, err := t.ExecContext(ctx, "DELETE FROM instances WHERE event_id IN (SELECT id FROM events WHERE object_id = ?)", objectID); err != nil {
		return nil, fmt.Errorf("delete instances: %w", err)
	}
	kept, err := t.objectEventIDs(ctx, objectID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(events))
	for _, e := range events {
		id, ok := kept[e.RecurrenceID]
		delete(kept, e.RecurrenceID)
		if id, err = t.putEvent(ctx, objectID, id, ok, e); err != nil {
			return nil, err
		}
		if err := t.insertEventDetails(ctx, id, e); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	for _, id := range kept {
		if _, err := t.ExecContext(ctx, "DELETE FROM events WHERE id = ?", id); err != nil {
			return nil, fmt.Errorf("delete event: %w", err)
		}
	}
	return ids, nil
}

// objectEventIDs maps an object's events' recurrence IDs to their IDs.
func (t *Tx) objectEventIDs(ctx context.Context, objectID int64) (map[string]int64, error) {
	rows, err := t.QueryContext(ctx, "SELECT recurrence_id, id FROM events WHERE object_id = ?", objectID)
	if err != nil {
		return nil, fmt.Errorf("object events: %w", err)
	}
	defer rows.Close()
	ids := map[string]int64{}
	for rows.Next() {
		var rid string
		var id int64
		if err := rows.Scan(&rid, &id); err != nil {
			return nil, fmt.Errorf("read object event: %w", err)
		}
		ids[rid] = id
	}
	return ids, rows.Err()
}

// putEvent updates the event with an ID the object had (clearing its
// attendees and alarms) or inserts a new one, and returns its ID.
func (t *Tx) putEvent(ctx context.Context, objectID, id int64, exists bool, e EventRow) (int64, error) {
	args := []any{e.UID, e.Summary, e.Location, e.Description, e.AllDay, eventTime(e.Start, e.AllDay),
		eventTime(e.End, e.AllDay), e.TZID, e.Recurrence, e.Status, e.Transparent, e.Organizer, e.OrganizerName,
		e.PartStat, e.Sequence}
	if !exists {
		err := t.QueryRowContext(ctx, `INSERT INTO events
 (uid, summary, location, description, all_day, start_at, end_at, tzid, recurrence, status, transparent,
 organizer, organizer_name, partstat, sequence, object_id, recurrence_id)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			append(args, objectID, e.RecurrenceID)...).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("insert event: %w", err)
		}
		return id, nil
	}
	_, err := t.ExecContext(ctx, `UPDATE events SET uid = ?, summary = ?, location = ?, description = ?,
 all_day = ?, start_at = ?, end_at = ?, tzid = ?, recurrence = ?, status = ?, transparent = ?, organizer = ?,
 organizer_name = ?, partstat = ?, sequence = ? WHERE id = ?`, append(args, id)...)
	if err != nil {
		return 0, fmt.Errorf("update event: %w", err)
	}
	for _, table := range []string{"event_attendees", "alarms"} {
		if _, err := t.ExecContext(ctx, "DELETE FROM "+table+" WHERE event_id = ?", id); err != nil {
			return 0, fmt.Errorf("clear event details: %w", err)
		}
	}
	return id, nil
}

func (t *Tx) insertEventDetails(ctx context.Context, id int64, e EventRow) error {
	for i, a := range e.Attendees {
		if _, err := t.ExecContext(ctx, `INSERT INTO event_attendees (event_id, position, email, name, role, partstat) VALUES (?, ?, ?, ?, ?, ?)`, id, i, a.Email, a.Name, a.Role, a.PartStat); err != nil {
			return fmt.Errorf("insert attendee: %w", err)
		}
	}
	for i, a := range e.Alarms {
		var offset, at any
		if a.Offset != nil {
			offset = int64(*a.Offset / time.Second)
		}
		if a.At != nil {
			at = FormatTime(*a.At)
		}
		if _, err := t.ExecContext(ctx, `INSERT INTO alarms (event_id, position, action, offset_seconds, related_end, trigger_at) VALUES (?, ?, ?, ?, ?, ?)`, id, i, a.Action, offset, a.RelatedEnd, at); err != nil {
			return fmt.Errorf("insert alarm: %w", err)
		}
	}
	return nil
}

// ReplaceInstances replaces the stored occurrences of an object's events.
func (t *Tx) ReplaceInstances(ctx context.Context, objectID int64, rows []InstanceRow) error {
	if _, err := t.ExecContext(ctx, "DELETE FROM instances WHERE event_id IN (SELECT id FROM events WHERE object_id = ?)", objectID); err != nil {
		return fmt.Errorf("delete instances: %w", err)
	}
	for _, r := range rows {
		var owner int64
		if err := t.QueryRowContext(ctx, "SELECT object_id FROM events WHERE id = ?", r.EventID).Scan(&owner); err != nil {
			return fmt.Errorf("instance event: %w", err)
		}
		if owner != objectID {
			return fmt.Errorf("instance event %d belongs to another object", r.EventID)
		}
		if _, err := t.ExecContext(ctx, `INSERT INTO instances (event_id, recurrence_id, all_day, start_at, end_at) VALUES (?, ?, ?, ?, ?)`, r.EventID, r.RecurrenceID, r.AllDay, eventTime(r.Start, r.AllDay), eventTime(r.End, r.AllDay)); err != nil {
			return fmt.Errorf("insert instance: %w", err)
		}
	}
	return nil
}

// InstanceWindow returns the dates the instances cover.
func (d *DB) InstanceWindow(ctx context.Context) (from, to time.Time, ok bool, err error) {
	var a, b string
	err = d.db.QueryRowContext(ctx, "SELECT from_date, to_date FROM instance_window WHERE id = 1").Scan(&a, &b)
	if err == sql.ErrNoRows {
		return from, to, false, nil
	}
	if err != nil {
		return from, to, false, fmt.Errorf("instance window: %w", err)
	}
	from, err = time.Parse(time.DateOnly, a)
	if err != nil {
		return from, to, false, err
	}
	to, err = time.Parse(time.DateOnly, b)
	return from, to, err == nil, err
}

// SetInstanceWindow records the dates the instances cover.
func (t *Tx) SetInstanceWindow(ctx context.Context, from, to time.Time) error {
	_, err := t.ExecContext(ctx, `INSERT INTO instance_window (id, from_date, to_date) VALUES (1, ?, ?)
 ON CONFLICT(id) DO UPDATE SET from_date = excluded.from_date, to_date = excluded.to_date`, from.UTC().Format(time.DateOnly), to.UTC().Format(time.DateOnly))
	if err != nil {
		return fmt.Errorf("set instance window: %w", err)
	}
	return nil
}

const eventColumns = `e.id, e.object_id, e.uid, e.recurrence_id, e.summary, e.location, e.description,
 e.all_day, e.start_at, e.end_at, e.tzid, e.recurrence, e.status, e.transparent, e.organizer,
 e.organizer_name, e.partstat, e.sequence, col.id, col.account_id, (col.read_only OR a.read_only)`
const eventJoins = ` FROM events e JOIN objects o ON o.id = e.object_id
 JOIN collections col ON col.id = o.collection_id JOIN accounts a ON a.id = col.account_id`
const visibleEvents = ` AND col.enabled = 1 AND EXISTS (SELECT 1 FROM account_services s
 WHERE s.account_id = col.account_id AND s.service = 'calendar' AND s.enabled = 1)`

func calendarFilter(ids []int64) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return " AND col.id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")", args
}

func eventScan(e *EventRow, start, end *string) []any {
	return []any{&e.ID, &e.ObjectID, &e.UID, &e.RecurrenceID, &e.Summary, &e.Location, &e.Description,
		&e.AllDay, start, end, &e.TZID, &e.Recurrence, &e.Status, &e.Transparent, &e.Organizer,
		&e.OrganizerName, &e.PartStat, &e.Sequence, &e.CalendarID, &e.AccountID, &e.ReadOnly}
}

func parseEventTimes(start, end string, allDay bool) (time.Time, time.Time, error) {
	layout := TimeFormat
	if allDay {
		layout = time.DateOnly
	}
	a, err := time.Parse(layout, start)
	if err != nil {
		return a, time.Time{}, err
	}
	b, err := time.Parse(layout, end)
	return a, b, err
}

// Occurrences returns stored occurrences.
func (d *DB) Occurrences(ctx context.Context, f OccurrenceFilter) ([]OccurrenceRow, error) {
	query := "SELECT i.recurrence_id, i.all_day, i.start_at, i.end_at, " + eventColumns + eventJoins + ` JOIN instances i ON i.event_id = e.id
 WHERE col.kind = 'calendar'` + visibleEvents + ` AND ((i.all_day = 0 AND i.start_at < ?
 AND (i.end_at > ? OR (i.start_at = i.end_at AND i.start_at >= ?)))
 OR (i.all_day = 1 AND i.start_at < ? AND i.end_at > ?))`
	args := []any{FormatTime(f.To), FormatTime(f.From), FormatTime(f.From), f.ToDate, f.FromDate}
	filter, ids := calendarFilter(f.CalendarIDs)
	query += filter
	args = append(args, ids...)
	if f.WithEmail != "" {
		query += ` AND (e.organizer = ? OR EXISTS (SELECT 1 FROM event_attendees ea WHERE ea.event_id = e.id AND ea.email = ?))`
		args = append(args, f.WithEmail, f.WithEmail)
	}
	query += " ORDER BY i.start_at, i.all_day DESC, e.id, i.recurrence_id"
	if f.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, f.Limit)
	}
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("occurrences: %w", err)
	}
	defer rows.Close()
	out := []OccurrenceRow{}
	for rows.Next() {
		var r OccurrenceRow
		var start, end, es, ee string
		dest := []any{&r.Instance.RecurrenceID, &r.Instance.AllDay, &start, &end}
		if err := rows.Scan(append(dest, eventScan(&r.Event, &es, &ee)...)...); err != nil {
			return nil, fmt.Errorf("read occurrence: %w", err)
		}
		r.Instance.EventID = r.Event.ID
		r.Instance.Start, r.Instance.End, err = parseEventTimes(start, end, r.Instance.AllDay)
		if err != nil {
			return nil, err
		}
		r.Event.Start, r.Event.End, err = parseEventTimes(es, ee, r.Event.AllDay)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Event returns one event with its attendees, alarms and calendar.
func (d *DB) Event(ctx context.Context, id int64) (EventRow, error) {
	var e EventRow
	var start, end string
	err := d.db.QueryRowContext(ctx, "SELECT "+eventColumns+eventJoins+" WHERE e.id = ?", id).Scan(eventScan(&e, &start, &end)...)
	if err == sql.ErrNoRows {
		return e, ErrNotFound
	}
	if err != nil {
		return e, fmt.Errorf("read event: %w", err)
	}
	e.Start, e.End, err = parseEventTimes(start, end, e.AllDay)
	if err != nil {
		return e, err
	}
	if err := d.eventAttendees(ctx, &e); err != nil {
		return e, err
	}
	err = d.eventAlarms(ctx, &e)
	return e, err
}

func (d *DB) eventAttendees(ctx context.Context, e *EventRow) error {
	rows, err := d.db.QueryContext(ctx, "SELECT email, name, role, partstat FROM event_attendees WHERE event_id = ? ORDER BY position", e.ID)
	if err != nil {
		return fmt.Errorf("event attendees: %w", err)
	}
	defer rows.Close()
	e.Attendees = []AttendeeRow{}
	for rows.Next() {
		var a AttendeeRow
		if err := rows.Scan(&a.Email, &a.Name, &a.Role, &a.PartStat); err != nil {
			return fmt.Errorf("read attendee: %w", err)
		}
		e.Attendees = append(e.Attendees, a)
	}
	return rows.Err()
}

func (d *DB) eventAlarms(ctx context.Context, e *EventRow) error {
	rows, err := d.db.QueryContext(ctx, "SELECT action, offset_seconds, related_end, trigger_at FROM alarms WHERE event_id = ? ORDER BY position", e.ID)
	if err != nil {
		return fmt.Errorf("event alarms: %w", err)
	}
	defer rows.Close()
	e.Alarms = []AlarmRow{}
	for rows.Next() {
		var a AlarmRow
		var offset sql.NullInt64
		var at sql.NullString
		if err := rows.Scan(&a.Action, &offset, &a.RelatedEnd, &at); err != nil {
			return fmt.Errorf("read alarm: %w", err)
		}
		if offset.Valid {
			duration := time.Duration(offset.Int64) * time.Second
			a.Offset = &duration
		}
		if at.Valid {
			instant, err := ParseTime(at.String)
			if err != nil {
				return err
			}
			a.At = &instant
		}
		e.Alarms = append(e.Alarms, a)
	}
	return rows.Err()
}

// Events returns events without their attendees and alarms, ordered by object and event ID.
func (d *DB) Events(ctx context.Context, f EventsFilter) ([]EventRow, error) {
	query := "SELECT " + eventColumns + eventJoins + " WHERE col.kind = 'calendar'"
	if f.Visible {
		query += visibleEvents
	}
	filter, args := calendarFilter(f.CalendarIDs)
	rows, err := d.db.QueryContext(ctx, query+filter+" ORDER BY e.object_id, e.id", args...)
	if err != nil {
		return nil, fmt.Errorf("events: %w", err)
	}
	defer rows.Close()
	out := []EventRow{}
	for rows.Next() {
		var e EventRow
		var start, end string
		if err := rows.Scan(eventScan(&e, &start, &end)...); err != nil {
			return nil, fmt.Errorf("read event: %w", err)
		}
		e.Start, e.End, err = parseEventTimes(start, end, e.AllDay)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
