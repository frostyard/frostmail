package calendar

import (
	"errors"
	"time"
)

var errAllEventsNotYet = errors.New("calendar: All Events comes with task card T-0141")

// patchAll changes the whole series from one of its occurrences
// (ADR-0031, All Events).
func patchAll(src []byte, p occurrencePatch, c EventChange, now time.Time) ([]byte, error) {
	return nil, errAllEventsNotYet
}
