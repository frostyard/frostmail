// Package rpctest runs a real maild API server (store, broker, engine,
// rpcserver) on a temporary socket, for tests of clients such as mailctl.
package rpctest

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/events"
	"github.com/frostyard/frostmail/internal/rpcserver"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// Name is what the test server reports from rpc.hello.
const Name = "maild rpctest"

// Server is a running test server; it stops when the test ends.
type Server struct {
	Socket  string
	DB      *store.DB
	Broker  *events.Broker
	Secrets *secrets.File
}

// Start runs a server with a fresh database. The socket lives in a short
// private directory because Unix socket paths are limited to 108 bytes.
func Start(t testing.TB) *Server {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	broker := events.NewBroker(db)
	db.OnCommit = broker.Publish

	dir, err := os.MkdirTemp("", "fm")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "maild.sock")
	ln, err := rpcserver.Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := rpcserver.New(rpcserver.Options{Name: Name, Broker: broker})
	sec := secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json"))
	eng := engine.New(db, sec, slog.New(slog.DiscardHandler))
	router, err := api.NewRouter(api.Services{
		RPC: srv, Events: srv, Account: eng.Accounts(), Mailbox: eng.Mailboxes(),
		Message: eng.Messages(), Sync: eng.Sync(), View: eng.Views(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln, router) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not stop")
		}
		_ = db.Close()
		_ = os.RemoveAll(dir)
	})
	return &Server{Socket: socket, DB: db, Broker: broker, Secrets: sec}
}

// Dial connects and completes rpc.hello; the client closes when the test ends.
func (s *Server) Dial(t testing.TB) *api.Client {
	t.Helper()
	c, _, err := api.Dial(context.Background(), s.Socket, "rpctest")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
