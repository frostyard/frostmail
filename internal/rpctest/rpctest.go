// Package rpctest runs a real maild API server (store, broker, views,
// engine, rpcserver and optionally sync) on a temporary socket, for tests of
// clients such as mailctl and for end-to-end sync tests.
package rpctest

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/events"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/render"
	"github.com/frostyard/frostmail/internal/rpcserver"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/view"
)

// Name is what the test server reports from rpc.hello.
const Name = "maild rpctest"

// Options configure a test server.
type Options struct {
	// Sync, when set, runs the sync engine with this configuration.
	Sync *mailsync.Config
	// Log receives the server's logs; nil discards them.
	Log *slog.Logger
	// UndoDelay is how long sent mail waits; zero means 50ms, so tests
	// do not wait out the real delay.
	UndoDelay time.Duration
	// DataDir holds the database, secrets, blobs and parts; empty means a
	// fresh temporary directory. Reusing one after Stop restarts maild on
	// the same data.
	DataDir string
}

// Server is a running test server; it stops when the test ends.
type Server struct {
	Socket  string
	DB      *store.DB
	Broker  *events.Broker
	Secrets *secrets.File
	Blobs   *blob.Store
	Views   *view.Manager
	Sync    *mailsync.Manager // nil unless Options.Sync was set
	Parts   *render.PartsCache

	stop func()
}

// Start runs a server without the sync engine.
func Start(t testing.TB) *Server { return StartWith(t, Options{}) }

// StartWith runs a server with a fresh database. The socket lives in a short
// private directory because Unix socket paths are limited to 108 bytes.
func StartWith(t testing.TB, o Options) *Server {
	t.Helper()
	log := o.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	data := o.DataDir
	if data == "" {
		data = t.TempDir()
	}
	db, err := store.Open(context.Background(), filepath.Join(data, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	broker := events.NewBroker(db)
	views := view.NewManager(db, log)
	db.OnCommit = func(evs []api.EventEnvelope) {
		broker.Publish(evs)
		views.OnCommit(evs)
	}
	srv := &Server{
		DB: db, Broker: broker, Views: views,
		Secrets: secrets.NewFile(filepath.Join(data, "secrets.json")),
		Blobs:   blob.New(filepath.Join(data, "blobs")),
		Parts:   &render.PartsCache{Root: filepath.Join(data, "parts")},
	}
	// Remote images are never fetched in tests: the renderer has no fetcher.
	undo := o.UndoDelay
	if undo == 0 {
		undo = 50 * time.Millisecond
	}
	deps := engine.Deps{DB: db, Secrets: srv.Secrets, Log: log, Blobs: srv.Blobs, Views: views,
		Render: &render.Renderer{Parts: srv.Parts}, UndoDelay: undo}
	if o.Sync != nil {
		cfg := *o.Sync
		if cfg.Parts == nil {
			cfg.Parts = srv.Parts
		}
		srv.Sync = mailsync.New(db, srv.Secrets, srv.Blobs, log, cfg, broker.Publish)
		deps.Sync = srv.Sync
	}

	dir, err := os.MkdirTemp("", "fm")
	if err != nil {
		t.Fatal(err)
	}
	srv.Socket = filepath.Join(dir, "maild.sock")
	ln, err := rpcserver.Listen(srv.Socket)
	if err != nil {
		t.Fatal(err)
	}
	rpc := rpcserver.New(rpcserver.Options{Name: Name, Broker: broker, Logger: log})
	eng := engine.New(deps)
	router, err := api.NewRouter(api.Services{
		RPC: rpc, Events: rpc, Account: eng.Accounts(), Mailbox: eng.Mailboxes(),
		Message: eng.Messages(), Sync: eng.Sync(), Thread: eng.Threads(), View: eng.Views(),
		Draft: eng.Drafts(), Outbox: eng.Outbox(), Address: eng.Addresses(), Identity: eng.Identities(),
		Oauth: eng.Oauth(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go views.Run(ctx)
	if srv.Sync != nil {
		if err := srv.Sync.Start(ctx); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(ctx, ln, router) }()
	srv.stop = sync.OnceFunc(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not stop")
		}
		if srv.Sync != nil {
			srv.Sync.Wait()
		}
		_ = db.Close()
		_ = os.RemoveAll(dir)
	})
	t.Cleanup(srv.stop)
	return srv
}

// Stop shuts the server down now, as a crash between transactions would:
// sync stops, the socket closes and the database is closed. The test's
// cleanup does the same; Stop is for restarting on the same DataDir.
func (s *Server) Stop() { s.stop() }

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
