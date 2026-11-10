package live

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
)

// Page is how many rows one view.range asks for; KeepPages the pages kept
// on either side of the window asked for, beyond which rows are dropped
// and fetched again if they come back into view.
const (
	Page      = 100
	KeepPages = 20
)

// callTimeout bounds each call a View makes.
const callTimeout = 30 * time.Second

// View is one maild view as a list shows it: its count and a sparse window
// of rows. It opens the view when made, applies view.delta, refreshes rows
// named by message.changed, and fetches the rows Ensure asks for. Every
// change calls the notify function given to OpenView, from any goroutine.
type View struct {
	c      *api.Client
	query  api.ViewQuery
	notify func()
	off    func()
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	id      int64
	opened  bool
	err     error
	total   int64
	rows    map[int64]api.MessageSummary
	loading map[int64]bool // pages in flight
	gen     uint64
	window  [2]int64
	closed  bool
	early   []api.ViewDelta // deltas that came before view.open's reply
	version uint64
}

// OpenView opens a view of q on c and keeps it current from events.
func OpenView(c *api.Client, events *Events, q api.ViewQuery, notify func()) *View {
	ctx, cancel := context.WithCancel(context.Background())
	v := &View{c: c, query: q, notify: notify, ctx: ctx, cancel: cancel,
		rows: map[int64]api.MessageSummary{}, loading: map[int64]bool{}}
	v.off = events.On(v.event)
	go v.open()
	return v
}

// Query is the view's query.
func (v *View) Query() api.ViewQuery { return v.query }

// Ready reports whether view.open has answered; Err is its error.
func (v *View) Ready() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.opened
}

// Err is view.open's error, if it failed.
func (v *View) Err() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.err
}

// Count is the number of rows in the view.
func (v *View) Count() int64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.total
}

// Version identifies the view's state; it grows with every change.
func (v *View) Version() uint64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.version
}

// Row returns the row at index i when it is loaded.
func (v *View) Row(i int64) (api.MessageSummary, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	r, ok := v.rows[i]
	return r, ok
}

// IndexOf returns the index of a loaded row by message ID, or -1.
func (v *View) IndexOf(id int64) int64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i, r := range v.rows {
		if r.ID == id {
			return i
		}
	}
	return -1
}

// Ensure asks for rows [start, end) and keeps them as the window in view.
func (v *View) Ensure(start, end int64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.window = [2]int64{max(0, start), max(0, end)}
	v.ensureLocked()
}

func (v *View) ensureLocked() {
	if !v.opened || v.closed {
		return
	}
	last := min(v.window[1], v.total)
	for p := v.window[0] / Page; p*Page < last; p++ {
		if !v.loading[p] && !v.pageLoaded(p) {
			v.loading[p] = true
			go v.fetch(p, v.id, v.gen)
		}
	}
}

// Close closes the view in maild and stops listening.
func (v *View) Close() {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return
	}
	v.closed = true
	id, opened := v.id, v.opened
	v.mu.Unlock()
	v.off()
	v.cancel()
	if opened {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
			defer cancel()
			_ = v.c.View().Close(ctx, &api.ViewCloseParams{ID: id})
		}()
	}
}

func (v *View) open() {
	ctx, cancel := context.WithTimeout(v.ctx, callTimeout)
	defer cancel()
	info, err := v.c.View().Open(ctx, &api.ViewOpenParams{Query: v.query})
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		if err == nil {
			cctx, cancel := context.WithTimeout(context.Background(), callTimeout)
			defer cancel()
			_ = v.c.View().Close(cctx, &api.ViewCloseParams{ID: info.ID})
		}
		return
	}
	if err != nil {
		v.err = err
		v.changedLocked()
		v.mu.Unlock()
		v.notify()
		return
	}
	v.id, v.total, v.opened = info.ID, info.Count, true
	early := v.early
	v.early = nil
	for _, d := range early {
		if d.ID == info.ID {
			v.applyLocked(d)
		}
	}
	v.changedLocked()
	v.ensureLocked()
	v.mu.Unlock()
	v.notify()
}

func (v *View) event(ev api.Event) {
	switch e := ev.(type) {
	case api.ViewDelta:
		v.mu.Lock()
		switch {
		case v.closed:
		case !v.opened: // a delta can overtake view.open's reply
			v.early = append(v.early, e)
		case e.ID == v.id:
			v.applyLocked(e)
			v.changedLocked()
			v.ensureLocked()
			v.mu.Unlock()
			v.notify()
			return
		}
		v.mu.Unlock()
	case api.MessageChanged:
		go v.refresh(e.IDs)
	}
}

// applyLocked applies a delta's ops in order: an insert shifts the rows at
// or after it up, a remove drops its rows and shifts the later ones down.
// Pages in flight carry stale indices, so they are asked for again.
func (v *View) applyLocked(d api.ViewDelta) {
	v.gen++
	for _, op := range d.Ops {
		next := make(map[int64]api.MessageSummary, len(v.rows))
		for i, r := range v.rows {
			switch {
			case op.Op == api.ViewOpKindInsert && i >= op.At:
				next[i+op.Count] = r
			case op.Op == api.ViewOpKindInsert, i < op.At:
				next[i] = r
			case i >= op.At+op.Count:
				next[i-op.Count] = r
			}
		}
		v.rows = next
	}
	v.total = d.Count
	clear(v.loading)
}

func (v *View) fetch(p, id int64, gen uint64) {
	ctx, cancel := context.WithTimeout(v.ctx, callTimeout)
	defer cancel()
	rows, err := v.c.View().Range(ctx, &api.ViewRangeParams{ID: id, Start: p * Page, End: (p + 1) * Page})
	v.mu.Lock()
	if v.closed || gen != v.gen {
		v.mu.Unlock()
		return // a delta came first: it asked again
	}
	delete(v.loading, p)
	if err != nil {
		v.mu.Unlock()
		return
	}
	for i, r := range rows {
		v.rows[p*Page+int64(i)] = r
	}
	v.evictLocked()
	v.changedLocked()
	v.mu.Unlock()
	v.notify()
}

func (v *View) refresh(ids []int64) {
	v.mu.Lock()
	var hit []int64
	for _, r := range v.rows {
		if slices.Contains(ids, r.ID) {
			hit = append(hit, r.ID)
		}
	}
	v.mu.Unlock()
	if len(hit) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(v.ctx, callTimeout)
	defer cancel()
	fresh, err := v.c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: hit})
	if err != nil {
		return
	}
	byID := make(map[int64]api.MessageSummary, len(fresh))
	for _, s := range fresh {
		byID[s.ID] = s
	}
	v.mu.Lock()
	changed := false
	for i, r := range v.rows {
		if s, ok := byID[r.ID]; ok && !v.closed {
			v.rows[i] = s
			changed = true
		}
	}
	if changed {
		v.changedLocked()
	}
	v.mu.Unlock()
	if changed {
		v.notify()
	}
}

func (v *View) pageLoaded(p int64) bool {
	end := min((p+1)*Page, v.total)
	for i := p * Page; i < end; i++ {
		if _, ok := v.rows[i]; !ok {
			return false
		}
	}
	return true
}

func (v *View) evictLocked() {
	lo := (v.window[0]/Page - KeepPages) * Page
	hi := ((v.window[1]+Page-1)/Page + KeepPages) * Page
	for i := range v.rows {
		if i < lo || i >= hi {
			delete(v.rows, i)
		}
	}
}

func (v *View) changedLocked() { v.version++ }
