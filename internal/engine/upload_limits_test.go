package engine

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// fakeConn is a connection that never ends.
type fakeConn struct{ done chan struct{} }

func (fakeConn) Notify(api.Event) error  { return nil }
func (c fakeConn) Done() <-chan struct{} { return c.done }

func newConn() fakeConn { return fakeConn{done: make(chan struct{})} }

// started begins an upload and ends it when the test does, before
// t.TempDir's cleanup (cleanups run last in, first out): Windows will not
// remove a file that is still open.
func started(t *testing.T, s *uploads, c api.Conn) (*upload, error) {
	t.Helper()
	u, err := s.start(c, 1, "a.txt", 4)
	if err == nil {
		t.Cleanup(u.end)
	}
	return u, err
}

// TestUploadsInAll: 16 uploads in progress across connections, 4 each,
// and a 17th is refused until one ends.
func TestUploadsInAll(t *testing.T) {
	s := newUploads()
	s.dir = t.TempDir()
	var held []*upload
	for range maxUploads / maxUploadsPerConn {
		c := newConn()
		for range maxUploadsPerConn {
			u, err := started(t, s, c)
			if err != nil {
				t.Fatalf("upload %d of %d: %v", len(held)+1, maxUploads, err)
			}
			s.put(u)
			held = append(held, u)
		}
	}
	if _, err := started(t, s, newConn()); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("upload %d = %v, want conflict", maxUploads+1, err)
	}
	u, err := s.take(held[0].conn, held[0].id, 1, "a.txt", 4)
	if err != nil {
		t.Fatal(err)
	}
	u.end()
	if _, err := started(t, s, newConn()); err != nil {
		t.Errorf("an upload once one ended: %v", err)
	}
}

// TestUploadsIdle: an upload that sends nothing for 10 minutes is
// discarded, file and all; one that sent just inside them is kept.
func TestUploadsIdle(t *testing.T) {
	s := newUploads()
	s.dir = t.TempDir()
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	c := newConn()
	u, err := started(t, s, c)
	if err != nil {
		t.Fatal(err)
	}
	s.put(u)
	now = now.Add(uploadIdle - time.Second)
	if u, err = s.take(c, u.id, 1, "a.txt", 4); err != nil {
		t.Fatalf("an upload idle a second short of %v: %v", uploadIdle, err)
	}
	s.put(u)
	file := u.f.Name()
	now = now.Add(uploadIdle)
	if _, err := s.take(c, u.id, 1, "a.txt", 4); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("an upload idle %v = %v, want notFound", uploadIdle, err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the idle upload's file: %v, want it removed", err)
	}
}

// TestUploadsEndOnce: a chunk's request and its connection's end may end
// an upload at the same time; it ends once.
func TestUploadsEndOnce(t *testing.T) {
	s := newUploads()
	s.dir = t.TempDir()
	u, err := started(t, s, newConn())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(u.end)
	}
	wg.Wait()
	if _, err := os.Stat(u.f.Name()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the ended upload's file: %v, want it removed", err)
	}
}
