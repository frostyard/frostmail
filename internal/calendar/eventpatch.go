package calendar

import (
	"errors"
	"time"
)

// EventChange is a change to an event (calendar.update's EventFields,
// ADR-0030): nil fields keep their values. A change of timing gives all
// of it: AllDay, and Start, End and Zone for a timed event or StartDate
// and EndDate for an all-day one.
type EventChange struct {
	Summary     *string
	Location    *string
	Description *string
	AllDay      *bool
	Start, End  *time.Time // timed: instants, written in Zone; End after Start
	Zone        *string    // an IANA zone name, or UTC
	StartDate   *string    // all-day: YYYY-MM-DD
	EndDate     *string    // all-day: the day after the last
	RRule       *string    // the RRULE value; "" removes RRULE
	Alarms      *[]int     // minutes before the start (negative: after); replaces the DISPLAY and AUDIO alarms
	Transparent *bool
}

var errEventPatchNotYet = errors.New("calendar: event patches come with task card T-0139")

// PatchEvent applies c to the single event, or the master of the series,
// in src: its VEVENT without RECURRENCE-ID, rewriting only the lines c
// touches (ADR-0018, ADR-0030, ADR-0033).
func PatchEvent(src []byte, c EventChange, now time.Time) ([]byte, error) {
	return nil, errEventPatchNotYet
}

// NewEvent writes a calendar object holding one new VEVENT, and the
// VTIMEZONE of its zone.
func NewEvent(uid string, c EventChange, now time.Time) ([]byte, error) {
	return nil, errEventPatchNotYet
}
