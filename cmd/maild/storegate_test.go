package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// close waits for the calls under way: a gated read still running keeps the
// store open, and close returns once it ends.
func TestStoreGateCloseWaitsForCallsUnderWay(t *testing.T) {
	gate := newStoreGate()
	entered, release := make(chan struct{}), make(chan struct{})
	unread := unreadGated(gate, func(context.Context) (int, error) {
		close(entered)
		<-release
		return 3, nil
	})
	done := make(chan int, 1)
	go func() {
		n, err := unread(t.Context())
		if err != nil {
			t.Errorf("unread = %v", err)
		}
		done <- n
	}()
	<-entered
	closed := make(chan struct{})
	go func() {
		gate.close()
		close(closed)
	}()
	// The gate must not close while the read runs: it is held.
	select {
	case <-closed:
		t.Fatal("the gate closed while a gated read ran")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if n := <-done; n != 3 {
		t.Errorf("unread = %d, want 3", n)
	}
	<-closed
}

// A read after close is refused, and the store read does not run.
func TestStoreGateRefusesReadsAfterClose(t *testing.T) {
	gate := newStoreGate()
	gate.close()
	called := false
	unread := unreadGated(gate, func(context.Context) (int, error) {
		called = true
		return 7, nil
	})
	n, err := unread(t.Context())
	if err == nil || !strings.Contains(err.Error(), "maild is stopping") {
		t.Fatalf("unread after close = %d, %v; want a refusal", n, err)
	}
	if n != 0 {
		t.Errorf("unread after close = %d; want 0", n)
	}
	if called {
		t.Error("the store read ran after the gate closed")
	}
}
