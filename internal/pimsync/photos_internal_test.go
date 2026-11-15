package pimsync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/graph"
)

// photoServer answers each contact's photo with its id, 404 for ids that
// start with "none", and 500 for "bad".
func photoServer(t *testing.T) *graph.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/me/contacts/"), "/photo/$value")
		switch {
		case id == "bad":
			http.Error(w, `{"error": {"code": "InternalServerError", "message": "no"}}`, http.StatusInternalServerError)
		case strings.HasPrefix(id, "none"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": {"code": "ImageNotFound", "message": "no photo"}}`))
		default:
			_, _ = w.Write([]byte("photo of " + id))
		}
	}))
	t.Cleanup(srv.Close)
	return &graph.Client{Base: srv.URL + "/", HTTP: srv.Client(),
		Token: func(context.Context) (string, error) { return "token", nil }}
}

func TestContactPhotosReadsEachByID(t *testing.T) {
	c := photoServer(t)
	var ids []string
	for i := range 10 {
		ids = append(ids, fmt.Sprint("c", i), fmt.Sprint("none", i))
	}
	photos, err := contactPhotos(t.Context(), c, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != len(ids) {
		t.Fatalf("%d photos for %d contacts", len(photos), len(ids))
	}
	for i := range 10 {
		if got := string(photos[fmt.Sprint("c", i)]); got != fmt.Sprint("photo of c", i) {
			t.Errorf("c%d's photo = %q", i, got)
		}
		if got := photos[fmt.Sprint("none", i)]; got != nil {
			t.Errorf("none%d's photo = %q, want none", i, got)
		}
	}
	if photos, err := contactPhotos(t.Context(), c, nil); err != nil || len(photos) != 0 {
		t.Errorf("no contacts: %v, %v", photos, err)
	}
}

func TestContactPhotosStopsOnAnError(t *testing.T) {
	c := photoServer(t)
	ids := []string{"c1", "c2", "bad", "c3", "c4", "c5", "c6"}
	if _, err := contactPhotos(t.Context(), c, ids); err == nil || !strings.Contains(err.Error(), "contact photo") {
		t.Errorf("a failed photo read = %v, want its error", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := contactPhotos(ctx, c, ids); !errors.Is(err, context.Canceled) {
		t.Errorf("an ended context = %v, want context.Canceled", err)
	}
}

// gatedPhotos is a photo server whose reads wait: each read counts itself
// in flight, says so on arrived, and answers when release lets it go.
type gatedPhotos struct {
	inflight, peak atomic.Int64
	arrived        chan struct{}
	release        chan struct{}
}

func newGatedPhotos(t *testing.T) (*gatedPhotos, *graph.Client) {
	t.Helper()
	g := &gatedPhotos{arrived: make(chan struct{}), release: make(chan struct{})}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := g.inflight.Add(1)
		defer g.inflight.Add(-1)
		for p := g.peak.Load(); n > p && !g.peak.CompareAndSwap(p, n); p = g.peak.Load() {
		}
		select {
		case g.arrived <- struct{}{}:
		case <-r.Context().Done():
			return
		}
		select {
		case <-g.release:
			_, _ = w.Write([]byte("photo"))
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	return g, &graph.Client{Base: srv.URL + "/", HTTP: srv.Client(),
		Token: func(context.Context) (string, error) { return "token", nil }}
}

// arrival waits for the next read to arrive, or fails the test when none
// comes: a pool that reads fewer at once than it should never lets the
// test release a read.
func (g *gatedPhotos) arrival(t *testing.T, what string) {
	t.Helper()
	select {
	case <-g.arrived:
	case <-time.After(10 * time.Second):
		t.Fatalf("no photo read arrived %s (%d in flight)", what, g.inflight.Load())
	}
}

// The pool reads photoWorkers photos at once: more than 1, never more.
func TestContactPhotosReadsAtMostFourAtOnce(t *testing.T) {
	g, c := newGatedPhotos(t)
	const total = 10
	var ids []string
	for i := range total {
		ids = append(ids, fmt.Sprint("c", i))
	}
	done := make(chan error, 1)
	go func() {
		_, err := contactPhotos(t.Context(), c, ids)
		done <- err
	}()
	waiting := 0
	for served := 0; served < total; {
		g.arrival(t, fmt.Sprintf("with %d waiting and %d served", waiting, served))
		waiting++
		// Release a read only when every worker holds one.
		for waiting >= min(photoWorkers, total-served) && waiting > 0 {
			g.release <- struct{}{}
			waiting--
			served++
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if p := g.peak.Load(); p != photoWorkers {
		t.Errorf("%d photo reads ran at once, want %d", p, photoWorkers)
	}
}

// An error ends the reads in flight, and so does the end of the parent
// context; contactPhotos returns.
func TestContactPhotosStopsReadsInFlight(t *testing.T) {
	g, c := newGatedPhotos(t)
	ids := []string{"c0", "c1", "c2", "c3", "c4", "c5"}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := contactPhotos(ctx, c, ids)
		done <- err
	}()
	for i := range photoWorkers {
		g.arrival(t, fmt.Sprint("for worker ", i))
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("an ended context = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("contactPhotos did not return after its context ended (%d in flight)", g.inflight.Load())
	}
}

// A failed read cancels the reads in flight; contactPhotos returns its
// error.
func TestContactPhotosErrorStopsReadsInFlight(t *testing.T) {
	blocked := make(chan struct{}, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/bad/") {
			// Fail only when the other workers hold their reads.
			for range photoWorkers - 1 {
				select {
				case <-blocked:
				case <-r.Context().Done():
					return
				}
			}
			http.Error(w, `{"error": {"code": "InternalServerError", "message": "no"}}`, http.StatusInternalServerError)
			return
		}
		blocked <- struct{}{}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	c := &graph.Client{Base: srv.URL + "/", HTTP: srv.Client(),
		Token: func(context.Context) (string, error) { return "token", nil }}
	done := make(chan error, 1)
	go func() {
		_, err := contactPhotos(t.Context(), c, []string{"c0", "c1", "bad", "c3", "c4", "c5"})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "contact photo") || errors.Is(err, context.Canceled) {
			t.Errorf("a failed read = %v, want its own error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("contactPhotos did not return after a read failed")
	}
}
