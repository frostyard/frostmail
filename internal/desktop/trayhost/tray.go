// Package trayhost says whether a panel shows the app's tray icon now, so that
// a close of the window hides it in the tray only where the icon can be
// seen (docs/adr/0060-close-to-the-tray-is-an-opt-in-setting.md). It was
// frostui's TrayHosted (v0.10.1), asked over godbus in place of GIO.
package trayhost

import (
	"sync"
	"time"
)

// watcherName is the D-Bus name of the StatusNotifierWatcher, which a
// panel owns while it can show AppIndicator icons.
const watcherName = "org.kde.StatusNotifierWatcher"

// hostedWait is the longest Hosted waits for an answer.
const hostedWait = 500 * time.Millisecond

// query is 1 watcher query: done closes once owned and err are set.
type query struct {
	done  chan struct{}
	owned bool
	err   error
}

// Hosts asks whether a panel hosts tray icons. Its zero value is not
// ready: use New. It is safe from any goroutine.
type Hosts struct {
	// ask reports whether a D-Bus name has an owner on the session bus,
	// and deadline starts the wait of 1 Hosted call. A test replaces them.
	ask      func(name string) (bool, error)
	deadline func() <-chan time.Time

	mu       sync.Mutex
	inFlight *query
}

// New returns the check of this platform: on Linux it asks the session
// bus; elsewhere every answer is true, since Windows and macOS always show
// the icon.
func New() *Hosts {
	return &Hosts{ask: nameHasOwner, deadline: func() <-chan time.Time { return time.After(hostedWait) }}
}

// Hosted reports whether a panel shows the tray icon now: on Linux,
// whether org.kde.StatusNotifierWatcher has an owner on the session bus.
// It waits half a second at most: a query that takes longer goes on in the
// background and the answer is false, and the calls after it wait on that
// same query, so a stalled bus never piles queries up. Call it on the UI
// thread only where that wait is acceptable, as at a close.
func (h *Hosts) Hosted() bool {
	h.mu.Lock()
	q := h.inFlight
	if q == nil {
		q = &query{done: make(chan struct{})}
		h.inFlight = q
		ask := h.ask
		go func() {
			q.owned, q.err = ask(watcherName)
			h.mu.Lock()
			h.inFlight = nil
			h.mu.Unlock()
			close(q.done)
		}()
	}
	h.mu.Unlock()
	select {
	case <-q.done:
		return q.err == nil && q.owned
	case <-h.deadline():
		return false
	}
}
