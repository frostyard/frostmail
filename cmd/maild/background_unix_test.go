//go:build !windows

package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

// maild's background work reads the store (a view recompute, a reminder
// pass, lodge's unread badge), so maild waits for it before it closes the
// database.
func TestMaildStopsItsBackgroundWorkBeforeClosingTheStore(t *testing.T) {
	newSandbox(t, false)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	hold := make(chan struct{})     // the background goroutines wait here
	started := make(chan string, 2) // each background goroutine, as it starts
	ready, done := make(chan struct{}), make(chan error, 1)
	var mu sync.Mutex
	var order []string // "views", "reminders" as they end; then "closed"
	go func() {
		done <- runWith(ctx, nil, runOptions{
			serving: func() { close(ready) },
			bgStart: func(name string) {
				started <- name
				<-hold
			},
			bgEnd: func(name string) {
				mu.Lock()
				order = append(order, name)
				mu.Unlock()
			},
			closing: func() {
				mu.Lock()
				order = append(order, "closed")
				mu.Unlock()
			},
		})
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("maild stopped before it served: %v", err)
	}
	for range 2 {
		select {
		case <-started:
		case err := <-done:
			t.Fatalf("maild stopped before its background work started: %v", err)
		}
	}
	// Every background goroutine is held, so maild must not return yet: it
	// has work to wait for, and the store must stay open for it.
	cancel()
	select {
	case err := <-done:
		t.Fatalf("maild stopped with its background work still running: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(hold)
	if err := <-done; err != nil {
		t.Fatalf("maild stopped with %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[2] != "closed" {
		t.Fatalf("the background ends and the close = %v; want both ends before the close", order)
	}
}
