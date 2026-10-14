package calendar

import (
	"errors"
	"time"
)

// Scope says what a change to one occurrence of a series changes
// (ADR-0031).
type Scope int

// The scopes, as calendar.update and calendar.delete take them.
const (
	ScopeThis   Scope = iota // the occurrence: an override, or an EXDATE when deleting
	ScopeFuture              // the occurrence and those after it: the series splits
	ScopeAll                 // the whole series: its master
)

var errOccurrenceNotYet = errors.New("calendar: occurrence changes come with task cards T-0140 and T-0141")

// PatchOccurrence applies c to one occurrence of the series in src, in
// scope (ADR-0031). recurrenceID names the occurrence as Parse and Expand
// do (RecurrenceKey of its original start). For ScopeFuture, split is the
// new series' object, under newUID; it is nil otherwise.
func PatchOccurrence(src []byte, recurrenceID string, scope Scope, c EventChange, newUID string, now time.Time) (patched, split []byte, err error) {
	return nil, nil, errOccurrenceNotYet
}

// DeleteOccurrence deletes one occurrence (ScopeThis) or the series from
// it on (ScopeFuture). Deleting a whole series deletes its object, which
// is not a patch.
func DeleteOccurrence(src []byte, recurrenceID string, scope Scope, now time.Time) ([]byte, error) {
	return nil, errOccurrenceNotYet
}
