// Package rpctest runs a real maild API server (store, broker, views,
// engine, rpcserver and optionally sync) on a temporary socket, for tests of
// clients such as mailctl and for end-to-end sync tests.
package rpctest

import (
	"context"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/discover"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/events"
	"github.com/frostyard/frostmail/internal/lodge"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/notify"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/pimsync"
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
	// OAuthEndpoints replace the providers' real endpoints (a fake token
	// server); nil means the real ones, which tests must not reach.
	OAuthEndpoints map[string]oauth.Endpoint
	// Discovery replaces account.discover's network access; zero means the
	// real one (provider profiles need none).
	Discovery discover.Deps
	// PIM, when set, runs contacts, calendar and tasks sync with this
	// configuration (point HTTP at a davtest server's client).
	PIM *pimsync.Config
	// Now, when set, is the clock for stored timestamps and for PIM's
	// instances window (unless PIM sets its own).
	Now func() time.Time
	// Unsubscriber, when set, sends one-click unsubscribe requests
	// (engine.Deps.Unsubscriber); tests pass a fake.
	Unsubscriber engine.Unsubscriber
	// Lodge, when set, serves reader pages and parts for lodge's content
	// route, as maild --lodge does (Server.Content).
	Lodge bool
	// Broadcast announces new mail to connected clients
	// (message.announced), as a maild without a session bus does; it
	// replaces Sync's Announce.
	Broadcast bool
	// OnAttended learns when the app connects and leaves
	// (rpcserver.Options.OnAttended).
	OnAttended func(attended bool)
	// Build is what rpc.hello reports as build (rpcserver.Options.Build).
	Build string
	// Services, when set, may replace the engine's domains with fakes
	// before the router is built, so a test can run the server against
	// them.
	Services func(*api.Services)
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
	PIM     *pimsync.Manager  // nil unless Options.PIM was set
	RPC     *rpcserver.Server
	Router  *api.Router
	Parts   *render.PartsCache
	OAuth   *oauth.Manager
	Content *lodge.Content // nil unless Options.Lodge was set

	ctx     context.Context
	stop    func()
	stopped chan struct{} // closed when Serve has returned
}

// Stopped is closed when the server has stopped serving, which rpc.shutdown
// (or Stop) causes: the socket is closed and answers no more.
func (s *Server) Stopped() <-chan struct{} { return s.stopped }

// Start runs a server without the sync engine.
func Start(t testing.TB) *Server { return StartWith(t, Options{}) }

// StartWith runs a server with a fresh database, on a socket of its own
// (SocketPath).
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
	if o.Now != nil {
		db.Now = o.Now
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
	srv.OAuth = &oauth.Manager{DB: db, Secrets: srv.Secrets, Log: log, Endpoints: o.OAuthEndpoints}
	if o.Lodge {
		if srv.Content, err = lodge.NewContent(srv.Parts, srv.Blobs); err != nil {
			t.Fatal(err)
		}
	}
	deps := engine.Deps{DB: db, Secrets: srv.Secrets, Log: log, Blobs: srv.Blobs, Views: views,
		Render: &render.Renderer{Parts: srv.Parts}, UndoDelay: undo, OAuth: srv.OAuth, Discovery: o.Discovery,
		Unsubscriber: o.Unsubscriber}
	if srv.Content != nil {
		deps.Content = srv.Content
	}
	if o.PIM != nil {
		cfg := *o.PIM
		cfg.Tokens = srv.OAuth
		if cfg.Now == nil {
			cfg.Now = o.Now
		}
		srv.PIM = pimsync.New(db, srv.Secrets, log, cfg)
		deps.PIM = srv.PIM
	}
	if o.Sync != nil {
		cfg := *o.Sync
		if cfg.Parts == nil {
			cfg.Parts = srv.Parts
		}
		cfg.Tokens = srv.OAuth
		if o.Broadcast {
			cfg.Announce = notify.Broadcast(broker.Publish)
		}
		srv.Sync = mailsync.New(db, srv.Secrets, srv.Blobs, log, cfg, broker.Publish)
		deps.Sync = srv.Sync
		srv.OAuth.SignedIn = func(id int64) { _ = srv.Sync.Restart(context.Background(), id) }
	}

	socket, cleanup, err := SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	srv.Socket = socket
	ln, err := rpcserver.Listen(srv.Socket, log)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	// rpc.shutdown ends Serve, as it does maild's.
	rpc := rpcserver.New(rpcserver.Options{Name: Name, Broker: broker, Logger: log, OnAttended: o.OnAttended,
		Build: o.Build, Shutdown: cancel})
	srv.RPC = rpc
	eng := engine.New(deps)
	services := api.Services{
		RPC: rpc, Events: rpc, Account: eng.Accounts(), Mailbox: eng.Mailboxes(),
		Message: eng.Messages(), Sync: eng.Sync(), Thread: eng.Threads(), View: eng.Views(),
		Draft: eng.Drafts(), Outbox: eng.Outbox(), Address: eng.Addresses(), Identity: eng.Identities(),
		Oauth: eng.Oauth(), People: eng.People(), Calendar: eng.Calendar(), Tasks: eng.Tasks(),
		Settings: eng.Settings(), Vip: eng.Vips(), Block: eng.Blocks(), Smart: eng.Smart(), Rule: eng.Rules(),
	}
	if o.Services != nil {
		o.Services(&services)
	}
	router, err := api.NewRouter(services)
	if err != nil {
		t.Fatal(err)
	}
	srv.Router = router
	srv.ctx = ctx
	go views.Run(ctx)
	if srv.Sync != nil {
		if err := srv.Sync.Start(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if srv.PIM != nil {
		if err := srv.PIM.Start(ctx); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 1)
	srv.stopped = make(chan struct{})
	go func() {
		err := rpc.Serve(ctx, ln, router)
		close(srv.stopped)
		done <- err
	}()
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
		if srv.PIM != nil {
			srv.PIM.Wait()
		}
		_ = db.Close()
		cleanup()
	})
	t.Cleanup(srv.stop)
	return srv
}

// Stop shuts the server down now, as a crash between transactions would:
// sync stops, the socket closes and the database is closed. The test's
// cleanup does the same; Stop is for restarting on the same DataDir.
func (s *Server) Stop() { s.stop() }

// DialAs serves a connection as caller, as lodge's /api does for a
// session, and completes rpc.hello on it; the client closes when the test
// ends.
func (s *Server) DialAs(t testing.TB, caller api.Caller) (*api.Client, *api.Hello) {
	t.Helper()
	server, client := net.Pipe()
	go s.RPC.ServeConn(s.ctx, server, s.Router, caller)
	c := api.NewClient(client)
	t.Cleanup(func() { _ = c.Close() })
	h, err := c.RPC().Hello(t.Context(), &api.RPCHelloParams{Protocol: api.Protocol, Client: "rpctest"})
	if err != nil {
		t.Fatal(err)
	}
	return c, h
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
