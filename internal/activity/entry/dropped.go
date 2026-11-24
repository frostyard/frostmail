// Package entry contains wire-level activity helpers shared by maild and its clients.
package entry

import (
	"github.com/frostyard/frostmail/api"
	"time"
)

// MaxRecent bounds an activity.recent request.
const MaxRecent = 500

func DroppedForAccount(account *int64, n int64) api.ActivityEntry {
	e := api.ActivityEntry{Level: api.ActivityLevelWarn, Kind: api.ActivityKindDropped, N: &n}
	if account != nil {
		id := *account
		e.AccountID = &id
	}
	return e
}

// Dropped: a client lost n entries behind its own buffer, as of at. Like a
// subscription's dropped entry it has seq 0 and names no account.
func Dropped(n int64, at time.Time) api.ActivityEntry {
	e := DroppedForAccount(nil, n)
	e.At = at
	return e
}
