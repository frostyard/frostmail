package view

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// Debounce is how long the manager waits after a change before recomputing
// views, so a burst of commits (an initial sync) costs one recompute.
const Debounce = 50 * time.Millisecond

// ErrNotFound means the view does not exist or belongs to another connection.
var ErrNotFound = errors.New("view: not found")

// Manager keeps the open views. Each is an ordered ID snapshot owned by one
// connection; after relevant commits the manager recomputes it and sends the
// owner a view.delta.
type Manager struct {
	db  *store.DB
	log *slog.Logger

	mu     sync.Mutex
	nextID int64
	views  map[int64]*view
	dirty  map[int64]bool // account IDs changed since the last recompute; 0 = all
	kick   chan struct{}
}

type view struct {
	id     int64
	conn   api.Conn
	filter store.ViewFilter
	ids    []int64
}

// NewManager returns a Manager; Run must be running for views to update.
func NewManager(db *store.DB, log *slog.Logger) *Manager {
	return &Manager{db: db, log: log, views: map[int64]*view{}, dirty: map[int64]bool{}, kick: make(chan struct{}, 1)}
}

// Run recomputes dirty views until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	for {
		midnight := time.NewTimer(untilMidnight(time.Now()))
		select {
		case <-ctx.Done():
			midnight.Stop()
			return
		case <-m.kick:
		case <-midnight.C:
			// Conditions' relative dates ("today") move with the day.
			m.mu.Lock()
			m.dirty[0] = true
			m.mu.Unlock()
		}
		midnight.Stop()
		select {
		case <-ctx.Done():
			return
		case <-time.After(Debounce):
		}
		m.recompute(ctx)
	}
}

// Open snapshots f for conn and closes the view when conn ends.
func (m *Manager) Open(ctx context.Context, conn api.Conn, f store.ViewFilter) (int64, int, error) {
	if conn == nil {
		return 0, 0, errors.New("view: views need a connection")
	}
	ids, err := m.db.ViewIDs(ctx, f)
	if err != nil {
		return 0, 0, err
	}
	m.mu.Lock()
	m.nextID++
	v := &view{id: m.nextID, conn: conn, filter: f, ids: ids}
	m.views[v.id] = v
	m.mu.Unlock()
	go func() {
		<-conn.Done()
		m.mu.Lock()
		delete(m.views, v.id)
		m.mu.Unlock()
	}()
	return v.id, len(ids), nil
}

// Range returns the IDs in [start, end) of a view conn owns; end is capped
// at the view's length.
func (m *Manager) Range(conn api.Conn, id int64, start, end int) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.views[id]
	if !ok || v.conn != conn {
		return nil, ErrNotFound
	}
	if start < 0 || end < start {
		return nil, errors.New("view: invalid range")
	}
	end = min(end, len(v.ids))
	if start >= end {
		return []int64{}, nil
	}
	return slices.Clone(v.ids[start:end]), nil
}

// Close closes a view conn owns.
func (m *Manager) Close(conn api.Conn, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.views[id]
	if !ok || v.conn != conn {
		return ErrNotFound
	}
	delete(m.views, id)
	return nil
}

// OnCommit marks views affected by committed events dirty; it is chained
// into the store's OnCommit hook and never blocks.
func (m *Manager) OnCommit(evs []api.EventEnvelope) {
	changed := false
	m.mu.Lock()
	for _, env := range evs {
		ev, err := api.DecodeEvent(env.Event, env.Data)
		if err != nil {
			continue
		}
		switch e := ev.(type) {
		case api.MessageChanged:
			m.dirty[e.AccountID], changed = true, true
		case api.MessageRemoved:
			m.dirty[e.AccountID], changed = true, true
		case api.AccountChanged:
			if e.Deleted {
				m.dirty[0], changed = true, true
			}
		case api.VipChanged, api.PeopleChanged, api.SettingsChanged:
			// Conditions on VIPs and People (ADR-0023) may list other messages.
			m.dirty[0], changed = true, true
		}
	}
	m.mu.Unlock()
	if changed {
		select {
		case m.kick <- struct{}{}:
		default:
		}
	}
}

func (m *Manager) recompute(ctx context.Context) {
	m.mu.Lock()
	dirty := m.dirty
	m.dirty = map[int64]bool{}
	type job struct {
		v *view
		f store.ViewFilter
	}
	var todo []job
	for _, v := range m.views {
		if dirty[0] || v.filter.AccountID == 0 || dirty[v.filter.AccountID] {
			f := v.filter
			if f.Unread != nil {
				// Rows read while the view is open stay (api.ViewQuery);
				// deleted, moved and unflagged ones still leave.
				f.Keep = v.ids
			}
			todo = append(todo, job{v, f})
		}
	}
	m.mu.Unlock()
	for _, j := range todo {
		v := j.v
		ids, err := m.db.ViewIDs(ctx, j.f)
		if err != nil {
			m.log.Warn("recompute view", "view", v.id, "err", err)
			continue
		}
		m.mu.Lock()
		if m.views[v.id] != v { // closed meanwhile
			m.mu.Unlock()
			continue
		}
		ops := Diff(v.ids, ids)
		v.ids = ids
		m.mu.Unlock()
		if len(ops) == 0 {
			continue
		}
		if err := v.conn.Notify(api.ViewDelta{ID: v.id, Count: int64(len(ids)), Ops: ops}); err != nil {
			m.log.Debug("view delta not sent", "view", v.id, "err", err)
		}
	}
}

// untilMidnight is the time from t to the next local midnight.
func untilMidnight(t time.Time) time.Duration {
	next := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
	return next.Sub(t)
}
