package calendar

import (
	"errors"
	"time"
)

// ErrWholeSeries is DeleteOccurrence's answer to All Future Events from a
// series' first occurrence: that deletes the series, so its object.
var ErrWholeSeries = errors.New("calendar: from its first occurrence, the change takes the whole series")

var errFutureNotYet = errors.New("calendar: All Future Events comes with task card T-0142")

// patchFuture splits the series before one of its occurrences (ADR-0031,
// All Future Events): the old object ends before it, and split, a new
// object under newUID, starts at it with c applied.
func patchFuture(src []byte, p occurrencePatch, c EventChange, newUID string, now time.Time) (patched, split []byte, err error) {
	return nil, nil, errFutureNotYet
}

// deleteFuture ends the series before one of its occurrences.
func deleteFuture(src []byte, p occurrencePatch, now time.Time) ([]byte, error) {
	return nil, errFutureNotYet
}
