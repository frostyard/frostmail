package mailsync

import (
	"time"

	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// The sync window (ADR-0016): an account keeps the messages that arrived
// in its last SyncDays days. A pass that searches asks the server for the
// UIDs SINCE the window's first day, so stored messages outside the window
// leave the store as messages deleted on the server do, and a wider window
// fetches what it adds.

// windowAt is the first day, in UTC, of a window of days as of t; zero
// for a window that keeps every message.
func windowAt(days int, t time.Time) time.Time {
	if days <= 0 {
		return time.Time{}
	}
	y, m, d := t.UTC().AddDate(0, 0, -days).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// mustSearch reports whether a pass must ask the server for its UIDs: on
// the first pass, and whenever messages arrived or left (UIDNEXT or the
// count moved). A windowed account also searches when the window's first
// day moved since the last pass, as mail ages out; an account without a
// window also searches when the store's count differs from the server's.
func (a *actor) mustSearch(ok bool, st store.SyncState, sel imapx.Selected, stored int, now time.Time) bool {
	if !ok || sel.UIDNext != st.UIDNext || sel.Messages != st.ServerCount {
		return true
	}
	if days := a.acct.SyncDays; days > 0 {
		return !windowAt(days, st.LastSyncAt).Equal(windowAt(days, now))
	}
	return uint32(stored) != sel.Messages
}
