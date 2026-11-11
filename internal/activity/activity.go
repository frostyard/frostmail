// Package activity records what maild does in the background as typed
// entries that hold no text (ADR-0055), keeps the newest in memory, and
// delivers them to the connections that subscribed.
//
// Only this package makes an api.ActivityEntry: the sync, send, PIM and
// OAuth code records an Entry made by one of the constructors in
// entries.go, and Classify reduces an error to its class and code without
// reading its text. A test of the module's source holds that rule.
package activity

import (
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
)

// The bounds of ADR-0055.
const (
	// Keep is how many entries a Hub holds.
	Keep = 2000
	// SubscriberBuffer is how many entries a subscription holds before it
	// loses entries.
	SubscriberBuffer = 256
	// PerSecond is how many debug and info entries of one kind an account
	// may record in one second; the rest are counted in a dropped entry.
	// Warnings and errors are always kept.
	PerSecond = 20
	// MaxRecent and DefaultRecent bound activity.recent.
	MaxRecent     = 500
	DefaultRecent = 200
)

// Entry is an entry before a Hub numbers and times it. Only this package's
// constructors make one.
type Entry struct{ e api.ActivityEntry }

// Hub keeps the newest entries and delivers each to the subscriptions. A
// nil Hub records nothing, so code that records needs no check.
type Hub struct {
	now func() time.Time

	mu    sync.Mutex
	seq   int64
	ring  []api.ActivityEntry // at most Keep, oldest at head once full
	head  int
	subs  map[*Sub]struct{}
	rates map[rateKey]*rate
}

// rateKey is what PerSecond limits: one kind of one account's entries.
type rateKey struct {
	account int64
	kind    api.ActivityKind
}

// rate counts one account's entries of one kind in the current second.
type rate struct {
	second  int64
	n       int
	dropped int64
}

// New returns a Hub that times entries with now (time.Now when nil).
func New(now func() time.Time) *Hub {
	if now == nil {
		now = time.Now
	}
	return &Hub{now: now, subs: map[*Sub]struct{}{}, rates: map[rateKey]*rate{}}
}

// Record numbers and times e, keeps it, and delivers it. A debug or info
// entry of a kind an account has recorded PerSecond times in this second
// is lost; the account's next entry of that kind after that second is
// preceded by one dropped entry that counts what it lost. Warnings and
// errors are never lost here.
func (h *Hub) Record(e Entry) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	at := h.now()
	if id := e.e.AccountID; id != nil {
		routine := e.e.Level == api.ActivityLevelDebug || e.e.Level == api.ActivityLevelInfo
		ok, lost := h.allow(rateKey{*id, e.e.Kind}, at, routine)
		if lost > 0 {
			h.add(dropped(e.e.AccountID, lost), at)
		}
		if !ok {
			return
		}
	}
	h.add(e.e, at)
}

func (h *Hub) allow(key rateKey, at time.Time, routine bool) (ok bool, lost int64) {
	r := h.rates[key]
	if r == nil {
		r = &rate{}
		h.rates[key] = r
	}
	if sec := at.Unix(); sec != r.second {
		lost = r.dropped
		r.second, r.n, r.dropped = sec, 0, 0
	}
	if !routine {
		return true, lost
	}
	if r.n >= PerSecond {
		r.dropped++
		return false, lost
	}
	r.n++
	return true, lost
}

// add keeps e and delivers it; h.mu is held.
func (h *Hub) add(e api.ActivityEntry, at time.Time) {
	h.seq++
	e.Seq, e.At = h.seq, at
	if len(h.ring) < Keep {
		h.ring = append(h.ring, e)
	} else {
		h.ring[h.head] = e
		h.head = (h.head + 1) % Keep
	}
	for s := range h.subs {
		s.deliver(e)
	}
}

// Recent returns the newest limit entries, oldest first; limit is clamped to
// 1..MaxRecent, and 0 means DefaultRecent.
func (h *Hub) Recent(limit int) []api.ActivityEntry {
	if h == nil {
		return []api.ActivityEntry{}
	}
	if limit <= 0 {
		limit = DefaultRecent
	}
	limit = min(limit, MaxRecent)
	h.mu.Lock()
	defer h.mu.Unlock()
	n := min(limit, len(h.ring))
	out := make([]api.ActivityEntry, 0, n)
	for i := len(h.ring) - n; i < len(h.ring); i++ {
		out = append(out, h.ring[(h.head+i)%len(h.ring)])
	}
	return out
}

// Subscribe starts a subscription that gets every entry recorded from now
// on, and returns the newest entry's seq (0 when there is none).
func (h *Hub) Subscribe() (*Sub, int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := &Sub{h: h, ch: make(chan api.ActivityEntry, SubscriberBuffer)}
	h.subs[s] = struct{}{}
	return s, h.seq
}

// Sub is one subscription. Its channel closes when it is closed.
type Sub struct {
	h      *Hub
	ch     chan api.ActivityEntry
	lost   int64 // entries not delivered since the last one that was
	closed bool
}

// C is the subscription's entries. A subscription that fell behind gets one
// dropped entry with seq 0 that counts the entries it lost, before the next
// entry it can take.
func (s *Sub) C() <-chan api.ActivityEntry { return s.ch }

// Close ends the subscription; closing twice does nothing.
func (s *Sub) Close() {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	delete(s.h.subs, s)
	close(s.ch)
}

// deliver hands e to the subscription without blocking; h.mu is held.
func (s *Sub) deliver(e api.ActivityEntry) {
	if s.lost > 0 {
		d := dropped(nil, s.lost)
		d.At = e.At
		select {
		case s.ch <- d:
			s.lost = 0
		default:
			s.lost++
			return
		}
	}
	select {
	case s.ch <- e:
	default:
		s.lost++
	}
}
