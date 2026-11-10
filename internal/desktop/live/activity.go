package live

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
)

// Activity follows maild's background activity for the console
// (ADR-0055): on each connection it subscribes, reads the recent entries,
// and hands every entry to its function once, oldest first, on whatever
// goroutine has it.
type Activity struct {
	onEntry func(api.ActivityEntry)

	mu      sync.Mutex
	off     func()
	ready   bool
	pending []api.ActivityEntry
	cut     int64 // this connection's subscription seq
	seen    int64 // the highest seq handed on, on this connection
	lastAt  time.Time
	lastSeq int64 // the newest entry handed on, across connections
}

// NewActivity returns a follower that hands entries to onEntry.
func NewActivity(onEntry func(api.ActivityEntry)) *Activity {
	return &Activity{onEntry: onEntry}
}

// Connect subscribes on c and hands on the recent entries the console has
// not shown: after a reconnect to the same maild, only the newer ones. A
// maild older than the activity domain answers methodNotFound, and the
// console stays empty.
func (a *Activity) Connect(ctx context.Context, c *api.Client, events *Events) error {
	a.mu.Lock()
	if a.off != nil {
		a.off()
	}
	a.ready, a.pending, a.seen = false, nil, 0
	a.off = events.On(a.event) // before subscribing, so no entry is missed
	a.mu.Unlock()

	sub, err := c.Activity().Subscribe(ctx, &api.ActivitySubscribeParams{})
	if errors.Is(err, api.ErrMethodNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	limit := int64(activity.MaxRecent)
	recent, err := c.Activity().Recent(ctx, &api.ActivityRecentParams{Limit: &limit})
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.cut = sub.Seq
	var out []api.ActivityEntry
	for _, e := range recent {
		if e.Seq <= a.cut && a.newer(e) {
			out = append(out, a.take(e))
		}
	}
	for _, e := range a.pending {
		if e.Seq == 0 || e.Seq > a.seen {
			out = append(out, a.take(e))
		}
	}
	a.ready, a.pending = true, nil
	a.mu.Unlock()
	for _, e := range out {
		a.onEntry(e)
	}
	return nil
}

func (a *Activity) event(ev api.Event) {
	e, ok := ev.(api.ActivityAdded)
	if !ok {
		return
	}
	a.mu.Lock()
	if !a.ready {
		a.pending = append(a.pending, e.Entry)
		a.mu.Unlock()
		return
	}
	if e.Entry.Seq != 0 && e.Entry.Seq <= a.seen {
		a.mu.Unlock()
		return
	}
	entry := a.take(e.Entry)
	a.mu.Unlock()
	a.onEntry(entry)
}

// newer reports whether e comes after the newest entry handed on: a maild
// that restarted numbers from 1 again, so time decides first.
func (a *Activity) newer(e api.ActivityEntry) bool {
	return e.At.After(a.lastAt) || (e.At.Equal(a.lastAt) && e.Seq > a.lastSeq)
}

func (a *Activity) take(e api.ActivityEntry) api.ActivityEntry {
	if e.Seq != 0 {
		a.seen = max(a.seen, e.Seq)
		a.lastAt, a.lastSeq = e.At, e.Seq
	}
	return e
}
