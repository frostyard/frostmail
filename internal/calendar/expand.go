package calendar

import (
	"time"

	"github.com/teambition/rrule-go"
)

// Occurrence is one occurrence of an event.
type Occurrence struct {
	// Event is the index, in the events given to Expand, of the single
	// event, the series' master, or the override that replaced this
	// occurrence.
	Event int
	// RecurrenceID is RecurrenceKey of the occurrence's original start; ""
	// for a single event.
	RecurrenceID string
	Start, End   time.Time // as Event's: instants, or UTC midnights for all-day
	AllDay       bool
}

// Expand returns the occurrences of one calendar object's events that
// overlap [from, to). Task T-0068 writes it.
func Expand(events []Event, from, to time.Time) ([]Occurrence, error) {
	return nil, errNotYet
}

// rruleParse is the parser task T-0068 builds on; it keeps the
// dependency in go.mod until then. Task T-0068 removes it.
var _ = rrule.StrToROptionInLocation
