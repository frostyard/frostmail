//go:build !windows

package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// syncAccount stores one account with no password, whose sync actor waits for
// it until ctx ends: an actor is running while maild serves.
func (sb sandbox) syncAccount(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	if err := os.MkdirAll(sb.data, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, filepath.Join(sb.data, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	err = db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.InsertAccount(ctx, store.Account{
			Kind: api.AccountKindIMAP, Email: "wait@mailtest.test", DisplayName: "Wait", Auth: api.AuthKindPassword,
			IMAP: store.ServerConfig{Host: "imap.mailtest.test", Port: 993, TLS: api.TLSModeTLS, Username: "wait"},
			SMTP: store.ServerConfig{Host: "smtp.mailtest.test", Port: 587, TLS: api.TLSModeStartTLS, Username: "wait"},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// maild's background work reads the store (a view recompute, a reminder
// pass, lodge's unread badge), so maild waits for it before it closes the
// database.
func TestMaildStopsItsBackgroundWorkBeforeClosingTheStore(t *testing.T) {
	newSandbox(t, false)
	ctx, cancel := context.WithCancel(t.Context())
	hold := make(chan struct{})     // the background goroutines wait here
	started := make(chan string, 2) // each background goroutine, as it starts
	var once sync.Once
	letGo := func() { once.Do(func() { close(hold) }) }
	var mu sync.Mutex
	var order []string // "views", "reminders" as they end; then "closed"
	var runErr error
	finished := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		letGo()
		<-finished
	})
	go func() {
		defer close(finished)
		runErr = runWith(ctx, nil, runOptions{
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
	for range 2 {
		select {
		case <-started:
		case <-finished:
			t.Fatalf("maild stopped before its background work started: %v", runErr)
		}
	}
	// Every background goroutine is held, so maild must not return yet: it
	// has work to wait for, and the store must stay open for it.
	cancel()
	select {
	case <-finished:
		t.Fatalf("maild stopped with its background work still running: %v", runErr)
	case <-time.After(50 * time.Millisecond):
	}
	letGo()
	<-finished
	if runErr != nil {
		t.Fatalf("maild stopped with %v", runErr)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[2] != "closed" {
		t.Fatalf("the background ends and the close = %v; want both ends before the close", order)
	}
}

// An error from the RPC socket while ctx is live stops maild: it cancels ctx,
// joins the sync actor and the background work, and closes the store after
// them, rather than waiting for a ctx that nobody ends.
func TestMaildJoinsItsWorkersWhenServingFails(t *testing.T) {
	sb := newSandbox(t, false)
	sb.syncAccount(t)
	t.Setenv("FROSTMAIL_SYNC", "") // sync is on: the account's actor runs until ctx ends
	ctx, cancel := context.WithCancel(t.Context())
	var mu sync.Mutex
	var order []string // "views", "reminders" as they end; then "closed"
	var runErr error
	finished := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-finished
	})
	go func() {
		defer close(finished)
		var ln net.Listener
		runErr = runWith(ctx, nil, runOptions{
			listener: func(l net.Listener) { ln = l },
			// The socket stops answering while ctx is live: Serve returns an
			// error, not a clean stop.
			serving: func() { _ = ln.Close() },
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
	case <-finished:
		if runErr == nil {
			t.Fatal("maild stopped cleanly; want the socket's error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("maild did not stop: ctx was never cancelled, so its workers were never joined")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[2] != "closed" {
		t.Fatalf("the background ends and the close = %v; want both ends before the close", order)
	}
}
