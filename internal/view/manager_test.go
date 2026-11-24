package view

import (
	"errors"
	"log/slog"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

type filterConn struct{ done chan struct{} }

func (c *filterConn) Notify(api.Event) error { return nil }
func (c *filterConn) Done() <-chan struct{}  { return c.done }

func TestManagerFilter(t *testing.T) {
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := NewManager(db, slog.Default())
	owner := &filterConn{done: make(chan struct{})}
	t.Cleanup(func() { close(owner.done) })
	f := store.ViewFilter{Role: "inbox", Threads: true, Accounts: []int64{1}}
	id, _, err := m.Open(t.Context(), owner, f)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Filter(owner, id)
	if err != nil || !reflect.DeepEqual(got, f) {
		t.Fatalf("filter = %+v, %v; want %+v", got, err, f)
	}
	other := &filterConn{done: make(chan struct{})}
	for _, tc := range []struct {
		conn api.Conn
		id   int64
	}{{other, id}, {owner, id + 1}} {
		if _, err := m.Filter(tc.conn, tc.id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("filter error = %v, want ErrNotFound", err)
		}
	}
}
