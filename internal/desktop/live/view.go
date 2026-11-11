package live

import (
	"context"
	"maps"
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
	// departed are the messages deltas removed from loaded rows, until a
	// reply shows one back or TakeDeparted hands them on.
	departed map[int64]bool
}

// OpenView opens a view of q on c and keeps it current from events.
func OpenView(c *api.Client, events *Events, q api.ViewQuery, notify func()) *View {
	ctx, cancel := context.WithCancel(context.Background())
	v := &View{c: c, query: q, notify: notify, ctx: ctx, cancel: cancel,
		rows: map[int64]api.MessageSummary{}, loading: map[int64]bool{}, departed: map[int64]bool{}}
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

// Rows returns the loaded rows of the given message IDs, in the order of
// ids.
func (v *View) Rows(ids []int64) []api.MessageSummary {
	v.mu.Lock()
	defer v.mu.Unlock()
	byID := make(map[int64]api.MessageSummary, len(ids))
	want := make(map[int64]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	for _, r := range v.rows {
		if want[r.ID] {
			byID[r.ID] = r
		}
	}
	out := make([]api.MessageSummary, 0, len(byID))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out
}

// TakeDeparted returns the messages that have left the view since the last
// call. A row a delta removes may only have moved, so it says nothing while
// the rows in view are still loading: a row that came back is not listed.
// A row dropped to save memory has not left, and stays selectable.
func (v *View) TakeDeparted() []int64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.departed) == 0 || !v.settledLocked() {
		return nil
	}
	out := slices.Collect(maps.Keys(v.departed))
	clear(v.departed)
	return out
}

// Settled reports whether the view has opened and every row in view is
// loaded: a message it does not list by then is not in view.
func (v *View) Settled() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.opened && v.settledLocked()
}

// settledLocked reports whether every row in view is loaded.
func (v *View) settledLocked() bool {
	if len(v.loading) > 0 {
		return false
	}
	last := min(v.window[1], v.total)
	for p := v.window[0] / Page; p*Page < last; p++ {
		if !v.pageLoaded(p) {
			return false
		}
	}
	return true
}

// Snapshot is a view's rows at one moment. Its rows never change, so one
// frame reads one state: a view changes on other goroutines, and two reads
// across a change could show one message at two indices.
type Snapshot struct {
	rows  map[int64]api.MessageSummary
	total int64
}

// Count is the number of rows.
func (s Snapshot) Count() int64 { return s.total }

// Row returns the row at index i when it is loaded.
func (s Snapshot) Row(i int64) (api.MessageSummary, bool) {
	r, ok := s.rows[i]
	return r, ok
}

// Snapshot returns the view as it stands. The map is shared, not copied:
// every change to the rows makes a new one.
func (v *View) Snapshot() Snapshot {
	v.mu.Lock()
	defer v.mu.Unlock()
	return Snapshot{rows: v.rows, total: v.total}
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

// open does not end with Close: maild may have opened the view already,
// and only its answer says which one to close.
func (v *View) open() {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
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
			default:
				v.departed[r.ID] = true
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
	v.storeLocked(p*Page, rows)
	v.evictLocked()
	v.changedLocked()
	v.mu.Unlock()
	v.notify()
}

// storeLocked puts rows at start. A reply to view.range can be newer than
// the deltas applied so far, so a message it holds may still sit at
// another index here: that older copy goes, and is fetched again if its
// index stays in view. A message is at one index only, as a list's keys
// must be unique.
func (v *View) storeLocked(start int64, rows []api.MessageSummary) {
	incoming := make(map[int64]bool, len(rows))
	for _, r := range rows {
		incoming[r.ID] = true
		delete(v.departed, r.ID)
	}
	next := make(map[int64]api.MessageSummary, len(v.rows)+len(rows))
	for i, r := range v.rows {
		if !incoming[r.ID] {
			next[i] = r
		}
	}
	for i, r := range rows {
		next[start+int64(i)] = r
	}
	v.rows = next
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
	next := maps.Clone(v.rows)
	for i, r := range v.rows {
		if s, ok := byID[r.ID]; ok && !v.closed {
			next[i] = s
			changed = true
		}
	}
	if changed {
		v.rows = next
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
	v.rows = maps.Collect(func(yield func(int64, api.MessageSummary) bool) {
		for i, r := range v.rows {
			if i >= lo && i < hi && !yield(i, r) {
				return
			}
		}
	})
}

func (v *View) changedLocked() { v.version++ }
