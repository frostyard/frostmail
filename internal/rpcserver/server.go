// Package rpcserver serves the maild API on lodge's kit
// (github.com/frostyard/lodge/kit/jsonrpc): rpc.hello, events.subscribe, a
// caller per connection and whether the app attends. The kit does the
// framing, the gating on rpc.hello, concurrent dispatch, panics and
// oversized replies.
package rpcserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/frostyard/lodge/kit/jsonrpc"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/events"
)

// Options configures a Server.
type Options struct {
	// Name is reported by rpc.hello, e.g. "maild v0.1.0".
	Name   string
	Broker *events.Broker
	Logger *slog.Logger
	// OnAttended, when set, learns when Attended changes.
	OnAttended func(attended bool)
	// Build is reported by rpc.hello as build: the Flatpak build maild runs
	// from. Empty means none (outside the Flatpak, or unknown).
	Build string
	// Shutdown stops maild, as SIGTERM does (rpc.shutdown). It must return
	// at once: the server calls it after the reply is on the wire, so the
	// caller always has its answer. Nil means this server cannot be stopped
	// by a call, and rpc.shutdown answers unavailable.
	Shutdown func()
}

// AppClient prefixes the rpc.hello client name of the Frostmail app.
const AppClient = "frostmail-app"

// Server implements api.RPCService and api.EventsService, which need the
// connection, and serves a Router built with them.
type Server struct {
	opts Options
	rpc  *jsonrpc.Server[api.ErrorCode]
	// apps counts the app's connections that subscribed to events.
	apps atomic.Int64
}

// New returns a Server.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	return &Server{opts: opts, rpc: jsonrpc.NewServer[api.ErrorCode](jsonrpc.ServerOptions{
		Hello: "rpc.hello", Logger: opts.Logger})}
}

var (
	_ api.RPCService    = (*Server)(nil)
	_ api.EventsService = (*Server)(nil)
)

// Serve accepts connections until ctx ends, then waits for them to finish.
// Each serves a local caller.
func (s *Server) Serve(ctx context.Context, ln net.Listener, r *api.Router) error {
	return s.rpc.Serve(ctx, ln, r, func(ctx context.Context) context.Context {
		return withState(ctx, api.Caller{Kind: api.CallerKindLocal})
	})
}

// ServeConn serves one connection the caller has already authenticated, as
// caller, until it ends or ctx does: the dev gateway's as local, lodge's
// /api as a session.
func (s *Server) ServeConn(ctx context.Context, nc net.Conn, r *api.Router, caller api.Caller) {
	if caller.Kind == "" {
		caller.Kind = api.CallerKindLocal
	}
	s.rpc.ServeConn(withState(ctx, caller), nc, r)
}

// state is a connection's own.
type state struct {
	mu         sync.Mutex
	subscribed bool
	app        bool // said AppClient in rpc.hello
}

type stateKey struct{}

func withState(ctx context.Context, caller api.Caller) context.Context {
	return context.WithValue(api.WithCaller(ctx, caller), stateKey{}, &state{})
}

// Hello implements rpc.hello.
func (s *Server) Hello(ctx context.Context, p *api.RPCHelloParams) (*api.Hello, error) {
	if p.Protocol != api.Protocol {
		return nil, api.ProtocolMismatch("maild speaks protocol %d, not %d", api.Protocol, p.Protocol)
	}
	caller := api.CallerFrom(ctx)
	if c, st := jsonrpc.ConnFrom(ctx), stateFrom(ctx); c != nil && st != nil {
		st.mu.Lock()
		st.app = strings.HasPrefix(p.Client, AppClient)
		st.mu.Unlock()
		c.SetReady(p.Client)
		s.opts.Logger.Debug("client connected", "client", p.Client, "caller", caller.Kind)
	}
	h := &api.Hello{Protocol: api.Protocol, Server: s.opts.Name}
	h.Caller = &caller.Kind
	h.Build = optional(s.opts.Build)
	if caller.Kind == api.CallerKindSession {
		h.Login = optional(caller.Login)
		h.NodeName = optional(caller.NodeName)
		h.DisplayName = optional(caller.DisplayName)
	}
	return h, nil
}

func stateFrom(ctx context.Context) *state {
	st, _ := ctx.Value(stateKey{}).(*state)
	return st
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Ping implements rpc.ping.
func (s *Server) Ping(context.Context, *api.RPCPingParams) error { return nil }

// Shutdown implements rpc.shutdown: the reply goes out first, then
// Options.Shutdown runs (jsonrpc.AfterReply), so the caller has its answer
// before maild stops. A call that fails never stops maild.
func (s *Server) Shutdown(ctx context.Context, _ *api.RPCShutdownParams) error {
	if s.opts.Shutdown == nil {
		return api.Unavailable("this server cannot be stopped by a call")
	}
	s.opts.Logger.Info("rpc.shutdown: stopping", "caller", api.CallerFrom(ctx).Kind)
	jsonrpc.AfterReply(ctx, s.opts.Shutdown)
	return nil
}

// Subscribe implements events.subscribe; delivery starts after the reply.
func (s *Server) Subscribe(ctx context.Context, p *api.EventsSubscribeParams) (*api.Subscription, error) {
	c, st := jsonrpc.ConnFrom(ctx), stateFrom(ctx)
	if c == nil || st == nil || s.opts.Broker == nil {
		return nil, api.Unavailable("events are not available on this connection")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.subscribed {
		return nil, api.Conflict("this connection is already subscribed")
	}
	sub, info, err := s.opts.Broker.Subscribe(ctx, p.SinceSeq)
	if err != nil {
		return nil, err
	}
	st.subscribed = true
	c.OnClose(sub.Close)
	if st.app {
		s.countApp(1)
		c.OnClose(func() { s.countApp(-1) })
	}
	jsonrpc.AfterReply(ctx, func() {
		c.Go(func(ctx context.Context) {
			err := sub.Run(ctx, func(ev api.EventEnvelope) error { return c.Notify(api.EventMethod, ev) })
			if errors.Is(err, events.ErrLagged) {
				s.opts.Logger.Warn("dropping lagging event subscriber")
				_ = c.Close() // the client resumes with sinceSeq
			}
		})
	})
	return &info, nil
}

// Attended reports whether the app is connected and receiving events: then
// what needs the user (reminders) goes to its windows rather than the
// desktop's notifications.
func (s *Server) Attended() bool { return s.apps.Load() > 0 }

func (s *Server) countApp(delta int64) {
	n := s.apps.Add(delta)
	if s.opts.OnAttended != nil && (n == 0 || n == delta) {
		s.opts.OnAttended(n > 0)
	}
}
