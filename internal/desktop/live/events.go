// Package live keeps the desktop app's data current from maild
// (docs/design/app.md, Data): an event fan-out per connection, a view
// model per open list, and the mail store. It imports no UI: a window
// reads snapshots and is told of changes through a callback, which it
// coalesces into frames.
package live

import (
	"sync"

	"github.com/frostyard/frostmail/api"
)

// Events reads one client's notifications, decodes each, and hands it to
// the handlers registered with On, in order, on its own goroutine.
type Events struct {
	mu       sync.Mutex
	next     int
	handlers map[int]func(api.Event)
}

// NewEvents returns a fan-out with no handlers.
func NewEvents() *Events { return &Events{handlers: map[int]func(api.Event){}} }

// On registers fn and returns the function that removes it.
func (e *Events) On(fn func(api.Event)) (off func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	id := e.next
	e.next++
	e.handlers[id] = fn
	return func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		delete(e.handlers, id)
	}
}

// Run delivers c's events until c closes. An event this build cannot
// decode (a newer maild's) is skipped.
func (e *Events) Run(c *api.Client) {
	for env := range c.Notifications() {
		ev, err := api.DecodeEvent(env.Event, env.Data)
		if err != nil {
			continue
		}
		e.Deliver(ev)
	}
}

// Deliver hands ev to every handler.
func (e *Events) Deliver(ev api.Event) {
	e.mu.Lock()
	hs := make([]func(api.Event), 0, len(e.handlers))
	for i := range e.next {
		if h, ok := e.handlers[i]; ok {
			hs = append(hs, h)
		}
	}
	e.mu.Unlock()
	for _, h := range hs {
		h(ev)
	}
}
