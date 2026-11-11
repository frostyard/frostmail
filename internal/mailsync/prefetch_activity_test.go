package mailsync

import (
	"context"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
)

// A prefetch that cannot open C3 still finishes its summary, and a later
// retry starts a new summary rather than carrying the failed run forward.
func TestFailedPrefetchFinishesItsActivity(t *testing.T) {
	e := newGmailEnv(t)
	e.g.add(gAll, "To fetch", 0, `\Inbox`)
	e.pass()
	hub := activity.New(nil)
	e.a.m.cfg.Activity = hub
	// The fixture has no token source: opening C3 fails before connecting.
	e.a.prefetch = make(chan struct{}, 1)
	if ids := e.a.prefetchQueue(t.Context(), nil); len(ids) != 1 {
		t.Fatalf("prefetch candidates = %v, want one", ids)
	}
	sub, _ := hub.Subscribe()
	defer sub.Close()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { e.a.bodyLoop(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	for range 2 {
		e.a.prefetch <- struct{}{}
		select {
		case entry := <-sub.C():
			if entry.Kind != api.ActivityKindFetched || entry.N == nil || *entry.N != 0 || entry.Bytes == nil || *entry.Bytes != 0 {
				t.Fatalf("failed prefetch = %+v, want a zero-body summary", entry)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("failed prefetch lost its summary")
		}
	}
}
