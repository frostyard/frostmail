package calendar

import (
	"errors"
	"time"
)

// errNotYet marks what task card T-0067 has yet to write.
var errNotYet = errors.New("calendar: not implemented yet")

// Event is one VEVENT: a single event, a series' master, or an override of
// one occurrence of a series (RecurrenceID set).
type Event struct {
	UID string
	// RecurrenceID is "" for a single event or a master; for an override,
	// the start of the occurrence it replaces, formatted as RecurrenceKey.
	RecurrenceID string
	Summary      string
	Location     string
	Description  string
	AllDay       bool
	// Start and End: for a timed event, instants in UTC (End exclusive);
	// for an all-day event, midnight UTC of the first day and of the day
	// after the last.
	Start, End time.Time
	// TZID is the IANA zone DTSTART was resolved to: "UTC" for UTC times,
	// "" for floating times and all-day events.
	TZID string
	// Floating is a timed event without a zone, read in Options.Local.
	Floating bool
	// Recurrence is the RRULE, RDATE and EXDATE content lines as written,
	// unfolded and joined by "\n"; "" for an event that is not a series.
	Recurrence  string
	Status      string // confirmed, tentative or cancelled
	Transparent bool   // TRANSP:TRANSPARENT: does not count as busy
	Organizer   *Attendee
	Attendees   []Attendee // without the organizer
	// PartStat is the user's answer when one of Options.UserEmails is an
	// attendee: needsaction, accepted, declined, tentative, delegated; "".
	PartStat string
	Sequence int
	Alarms   []Alarm
	// Zone is the location DTSTART is read in, for expanding the series
	// in its own wall time: the TZID's zone, Options.Local when floating,
	// UTC for UTC and all-day events.
	Zone *time.Location
}

// Attendee is an ORGANIZER or ATTENDEE.
type Attendee struct {
	Email    string // lowercased, without mailto:
	Name     string // CN
	Role     string // chair, required, optional, none
	PartStat string // needsaction, accepted, declined, tentative, delegated
}

// Alarm is a DISPLAY or AUDIO VALARM.
type Alarm struct {
	Action string // display or audio
	// Offset is relative to the occurrence's start, or its end when
	// RelatedEnd; negative is before. Unused when At is set.
	Offset     time.Duration
	RelatedEnd bool
	At         time.Time // an absolute trigger, in UTC; zero for a relative one
}

// Options say how to read an object's events.
type Options struct {
	// Local is the zone of floating times; nil means time.Local.
	Local *time.Location
	// UserEmails are the account's own addresses, lowercased: the attendee
	// with one of them is the user.
	UserEmails []string
}

// Parse reads the VEVENTs of a calendar object. Task T-0067 writes it.
func Parse(raw []byte, opts Options) ([]Event, error) {
	return nil, errNotYet
}

// RecurrenceKey formats an occurrence's original start as events,
// instances and RECURRENCE-IDs store it: YYYY-MM-DD for all-day, else the
// UTC instant in store.TimeFormat. Task T-0067 writes it.
func RecurrenceKey(t time.Time, allDay bool) string {
	return ""
}
