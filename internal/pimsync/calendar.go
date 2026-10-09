package pimsync

import (
	"errors"
	"time"

	"github.com/frostyard/frostmail/internal/store"
)

// errCalendarNotYet marks what task card T-0069 has yet to write.
var errCalendarNotYet = errors.New("pimsync: calendar indexing not implemented yet")

// Task T-0069 writes the functions below.

// Window returns the dates the instances cover on now's UTC day: from a
// year (365 days) before it to two years (730 days) after (ADR-0018).
func Window(now time.Time) (from, to time.Time) {
	return time.Time{}, time.Time{}
}

// Instances expands one object's stored events (store.DB.Events) over
// [from, to); floating times are read in local.
func Instances(events []store.EventRow, local *time.Location, from, to time.Time) ([]store.InstanceRow, error) {
	return nil, errCalendarNotYet
}
