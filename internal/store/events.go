package store

import (
	"context"
	"errors"
	"time"
)

// errEventsNotYet marks what task card T-0069 has yet to write.
var errEventsNotYet = errors.New("store: events not implemented yet")

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

// Task T-0069 writes the functions below.

// IndexEvents replaces an object's events.
func (t *Tx) IndexEvents(ctx context.Context, objectID int64, events []EventRow) ([]int64, error) {
	return nil, errEventsNotYet
}

// ReplaceInstances replaces the stored occurrences of an object's events.
func (t *Tx) ReplaceInstances(ctx context.Context, objectID int64, rows []InstanceRow) error {
	return errEventsNotYet
}

// InstanceWindow returns the dates the instances cover.
func (d *DB) InstanceWindow(ctx context.Context) (from, to time.Time, ok bool, err error) {
	return time.Time{}, time.Time{}, false, errEventsNotYet
}

// SetInstanceWindow records the dates the instances cover.
func (t *Tx) SetInstanceWindow(ctx context.Context, from, to time.Time) error {
	return errEventsNotYet
}

// Occurrences returns stored occurrences.
func (d *DB) Occurrences(ctx context.Context, f OccurrenceFilter) ([]OccurrenceRow, error) {
	return nil, errEventsNotYet
}

// Event returns one event with its attendees, alarms and calendar.
func (d *DB) Event(ctx context.Context, id int64) (EventRow, error) {
	return EventRow{}, errEventsNotYet
}

// Events returns events without their attendees and alarms, ordered by
// object and event ID: for expanding them over dates beyond the window.
func (d *DB) Events(ctx context.Context, f EventsFilter) ([]EventRow, error) {
	return nil, errEventsNotYet
}
