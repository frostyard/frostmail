// Package rpcserver serves the maild API on a Unix socket: newline-delimited
// JSON-RPC 2.0 framing, peer-UID checks, rpc.hello gating, concurrent
// dispatch and event delivery (docs/specs/rpc-protocol.md).
package rpcserver

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/events"
)

// MaxInFlight bounds concurrently running requests per connection; reading
// pauses until one finishes.
const MaxInFlight = 32

// Options configures a Server.
type Options struct {
	// Name is reported by rpc.hello, e.g. "maild v0.1.0".
	Name   string
	Broker *events.Broker
	Logger *slog.Logger
	// OnAttended, when set, learns when Attended changes.
	OnAttended func(attended bool)
}

// AppClient prefixes the rpc.hello client name of the Frostmail app.
const AppClient = "frostmail-app"

// Server implements api.RPCService and api.EventsService, which need the
// connection, and serves a Router built with them.
type Server struct {
	opts Options
	// apps counts the app's connections that subscribed to events.
	apps atomic.Int64
}

// New returns a Server.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	return &Server{opts: opts}
}

var (
	_ api.RPCService    = (*Server)(nil)
	_ api.EventsService = (*Server)(nil)
)

// Serve accepts connections until ctx ends, then waits for them to finish.
func (s *Server) Serve(ctx context.Context, ln net.Listener, r *api.Router) error {
	stop := context.AfterFunc(ctx, func() { _ = ln.Close() })
	defer stop()
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		nc, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		if err := checkPeer(nc); err != nil {
			s.opts.Logger.Warn("rejected connection", "err", err)
			_ = nc.Close()
			continue
		}
		c := &conn{s: s, r: r, nc: nc, sem: make(chan struct{}, MaxInFlight), caller: api.Caller{Kind: api.CallerKindLocal}}
		wg.Go(func() { c.serve(ctx) })
	}
}

// ServeConn serves one connection the caller has already authenticated, as
// caller, until it ends or ctx does: the dev gateway's (internal/devgw) as
// local, lodge's /api as a session. Unix socket connections go through
// Serve, which checks the peer's UID instead and serves them as local.
func (s *Server) ServeConn(ctx context.Context, nc net.Conn, r *api.Router, caller api.Caller) {
	c := &conn{s: s, r: r, nc: nc, sem: make(chan struct{}, MaxInFlight), caller: caller}
	c.serve(ctx)
}

type ctxKey struct{}

func connFrom(ctx context.Context) *conn {
	c, _ := ctx.Value(ctxKey{}).(*conn)
	return c
}

type conn struct {
	s      *Server
	r      *api.Router
	nc     net.Conn
	sem    chan struct{}
	ctx    context.Context
	caller api.Caller

	wmu   sync.Mutex
	ready atomic.Bool

	mu      sync.Mutex
	app     bool // said AppClient in rpc.hello
	counted bool // counted in Server.apps
	sub     *events.Subscription
	running bool
	wg      sync.WaitGroup
}

func (c *conn) serve(parent context.Context) {
	if c.caller.Kind == "" {
		c.caller.Kind = api.CallerKindLocal
	}
	ctx, cancel := context.WithCancel(api.WithCaller(api.WithConn(context.WithValue(parent, ctxKey{}, c), c), c.caller))
	c.ctx = ctx
	stop := context.AfterFunc(ctx, func() { _ = c.nc.Close() })
	defer func() {
		cancel()
		stop()
		_ = c.nc.Close()
		c.wg.Wait()
		c.mu.Lock()
		if c.sub != nil {
			c.sub.Close()
		}
		if c.counted {
			c.s.countApp(-1)
		}
		c.mu.Unlock()
	}()

	sc := bufio.NewScanner(c.nc)
	sc.Buffer(make([]byte, 64<<10), api.MaxMessageSize)
	for sc.Scan() {
		var m api.Frame
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			c.reply(nil, nil, api.ParseError("%v", err))
			continue
		}
		if m.JSONRPC != "2.0" || m.Method == "" {
			c.reply(m.ID, nil, api.InvalidRequest("expected a JSON-RPC 2.0 request"))
			continue
		}
		if len(m.ID) == 0 {
			continue // the protocol defines no client notifications
		}
		if !c.ready.Load() {
			// Inline until hello succeeds, so a pipelined call after hello
			// sees the connection ready.
			if m.Method != "rpc.hello" {
				c.reply(m.ID, nil, api.NotReady("call rpc.hello first"))
				continue
			}
			c.dispatch(ctx, m)
			continue
		}
		c.sem <- struct{}{}
		c.wg.Go(func() {
			defer func() { <-c.sem }()
			c.dispatch(ctx, m)
		})
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		c.s.opts.Logger.Debug("connection read ended", "err", err)
	}
}

func (c *conn) dispatch(ctx context.Context, m api.Frame) {
	res, err := c.call(ctx, m)
	c.reply(m.ID, res, err)
	if err == nil && m.Method == "events.subscribe" {
		c.startEvents() // only after the subscribe result is on the wire
	}
}

// call runs one request. A handler that panics answers internal and is
// logged, rather than ending maild for every client.
func (c *conn) call(ctx context.Context, m api.Frame) (res any, err error) {
	defer func() {
		if p := recover(); p != nil {
			c.s.opts.Logger.Error("request panicked", "method", m.Method, "panic", p, "stack", string(debug.Stack()))
			res, err = nil, api.Internal("%s failed", m.Method)
		}
	}()
	return c.r.Dispatch(ctx, m.Method, m.Params)
}

func (c *conn) reply(id jsontext.Value, result any, err error) {
	if len(id) == 0 {
		id = jsontext.Value("null")
	}
	resp := api.Frame{JSONRPC: "2.0", ID: id}
	if err == nil {
		resp.Result, err = json.Marshal(result)
	}
	if err != nil {
		var ae *api.Error
		if !errors.As(err, &ae) {
			c.s.opts.Logger.Warn("request failed", "err", err)
			ae = api.Internal("%v", err)
		}
		resp.Result = nil
		resp.Error = ae
	}
	if werr := c.write(resp); werr != nil {
		c.s.opts.Logger.Debug("reply not sent", "err", werr)
	}
}

func (c *conn) write(m api.Frame) error {
	line, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(line) >= api.MaxMessageSize {
		if m.Method != "" {
			return fmt.Errorf("a %s of %d bytes is over the %d-byte message limit", m.Method, len(line), api.MaxMessageSize)
		}
		big := len(line)
		line, err = json.Marshal(api.Frame{JSONRPC: "2.0", ID: m.ID,
			Error: api.Internal("the reply is %d bytes; a message is at most %d", big, api.MaxMessageSize)})
		if err != nil {
			return err
		}
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.nc.Write(append(line, '\n'))
	return err
}

// Notify implements api.Conn: a transient event to this connection only.
func (c *conn) Notify(ev api.Event) error {
	env, err := api.NewEnvelope(0, ev)
	if err != nil {
		return err
	}
	params, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return c.write(api.Frame{JSONRPC: "2.0", Method: api.EventMethod, Params: params})
}

// Done implements api.Conn.
func (c *conn) Done() <-chan struct{} { return c.ctx.Done() }

var _ api.Conn = (*conn)(nil)

func (c *conn) startEvents() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sub == nil || c.running {
		return
	}
	c.running = true
	sub := c.sub
	c.wg.Go(func() {
		err := sub.Run(c.ctx, func(ev api.EventEnvelope) error {
			params, err := json.Marshal(ev)
			if err != nil {
				return err
			}
			return c.write(api.Frame{JSONRPC: "2.0", Method: api.EventMethod, Params: params})
		})
		if errors.Is(err, events.ErrLagged) {
			c.s.opts.Logger.Warn("dropping lagging event subscriber")
			_ = c.nc.Close() // the client resumes with sinceSeq
		}
	})
}

// Hello implements rpc.hello.
func (s *Server) Hello(ctx context.Context, p *api.RPCHelloParams) (*api.Hello, error) {
	if p.Protocol != api.Protocol {
		return nil, api.ProtocolMismatch("maild speaks protocol %d, not %d", api.Protocol, p.Protocol)
	}
	if c := connFrom(ctx); c != nil {
		c.mu.Lock()
		c.app = strings.HasPrefix(p.Client, AppClient)
		c.mu.Unlock()
		c.ready.Store(true)
		s.opts.Logger.Debug("client connected", "client", p.Client, "caller", c.caller.Kind)
	}
	h := &api.Hello{Protocol: api.Protocol, Server: s.opts.Name}
	caller := api.CallerFrom(ctx)
	h.Caller = &caller.Kind
	if caller.Kind == api.CallerKindSession {
		h.Login = optional(caller.Login)
		h.NodeName = optional(caller.NodeName)
		h.DisplayName = optional(caller.DisplayName)
	}
	return h, nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Ping implements rpc.ping.
func (s *Server) Ping(context.Context, *api.RPCPingParams) error { return nil }

// Subscribe implements events.subscribe; delivery starts after the reply.
func (s *Server) Subscribe(ctx context.Context, p *api.EventsSubscribeParams) (*api.Subscription, error) {
	c := connFrom(ctx)
	if c == nil || s.opts.Broker == nil {
		return nil, api.Unavailable("events are not available on this connection")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sub != nil {
		return nil, api.Conflict("this connection is already subscribed")
	}
	sub, info, err := s.opts.Broker.Subscribe(ctx, p.SinceSeq)
	if err != nil {
		return nil, err
	}
	c.sub = sub
	if c.app {
		c.counted = true
		s.countApp(1)
	}
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
