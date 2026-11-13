package lodge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/frostyard/lodge/kit/lodgeapp"
	"github.com/frostyard/lodge/kit/mcpsession"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/mcpserver"
	"github.com/frostyard/frostmail/internal/rpcserver"
)

// ID is the mail app's id in lodge: its socket's name and URL segment.
const ID = "mail"

// Options are what maild's lodge socket serves.
type Options struct {
	// Ctx bounds the agents' connections to maild, which outlive the
	// request that opened their MCP session, and the MCP sessions' janitor:
	// maild's own context.
	Ctx context.Context
	// Version is maild's, for /health.
	Version string
	// RPC and Router serve /api, as the private socket does.
	RPC    *rpcserver.Server
	Router *api.Router
	// Content serves /content/.
	Content *Content
	Log     *slog.Logger
	// Now is the MCP tools' clock; nil is time.Now.
	Now func() time.Time
	// Unread is the count for lodge's rail badge (lodge ADR-0020): the
	// unread messages in every inbox. nil sends no badge.
	Unread func(ctx context.Context) (int, error)
	// Agents tracks the agents' connections to maild, so that maild can
	// wait for them before it closes the store (Agents.Close). nil tracks
	// them in an Agents that nobody closes.
	Agents *Agents
}

// Agents are the agents' connections to maild, which outlive the request
// that opened their MCP session and the lodge socket's own Serve. Close
// refuses new connections and waits for the ones that run.
type Agents struct {
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// start runs serve in a goroutine that Close waits for, and reports
// whether it did: once Agents is closed, or ctx has ended, it refuses.
func (a *Agents) start(ctx context.Context, serve func()) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || ctx.Err() != nil {
		return false
	}
	a.wg.Go(serve)
	return true
}

// Close refuses new connections and waits for the ones that run: each ends
// when maild's context does, after its calls under way return.
func (a *Agents) Close() {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	a.wg.Wait()
}

// NewServer is maild's side of lodge's app socket, through the kit: /health,
// /api as caller session, /content/ as caller content and /mcp as caller
// agent. lodge's other routes answer 404.
func NewServer(o Options) *lodgeapp.Server {
	log := o.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if o.Ctx == nil {
		o.Ctx = context.Background()
	}
	if o.Agents == nil {
		o.Agents = new(Agents)
	}
	mcp := mcpsession.New(mcpserver.Handler(mcpserver.HTTPOptions{Open: o.openAgent, Version: o.Version, Log: log, Now: o.Now}),
		mcpsession.Options{CheckScope: checkScope, Logger: log})
	// The sessions' janitor runs until maild stops serving.
	context.AfterFunc(o.Ctx, func() { _ = mcp.Close() })
	var badge *badgeCounter
	if o.Unread != nil {
		badge = &badgeCounter{unread: o.Unread, log: log}
	}
	return &lodgeapp.Server{
		Health: func(ctx context.Context) lodgeapp.Health {
			h := lodgeapp.Health{OK: true, Version: o.Version, Protocol: api.Protocol}
			if badge != nil {
				h.Badge = badge.badge(ctx)
			}
			return h
		},
		API: func(ctx context.Context, nc net.Conn, c lodgeapp.Caller) {
			log.Info("lodge session started", "session", c.Session, "node", c.NodeName)
			o.RPC.ServeConn(ctx, nc, o.Router, Caller(c))
			log.Info("lodge session ended", "session", c.Session, "node", c.NodeName)
		},
		Content: o.Content,
		MCP:     mcp,
		Logger:  log,
	}
}

// badgeCounter reads the count lodge draws in its rail (lodge ADR-0020). It
// runs one count at a time, and a count never holds /health past the 1 s
// lodge's poll allows: the query may outlive its context (the driver's
// PRAGMAs and the 5000 ms busy timeout run first).
type badgeCounter struct {
	unread func(ctx context.Context) (int, error)
	log    *slog.Logger
	busy   atomic.Bool
}

// badgeResult is a count, or the error that stopped it.
type badgeResult struct {
	n   int
	err error
}

// badge returns the unread count, or 0 when the count failed, took longer
// than 1 s, or still runs from an earlier call. A 0 badge never makes the
// app degraded.
func (b *badgeCounter) badge(ctx context.Context) int {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	// A count that outlives this call is interrupted where the driver can.
	defer cancel()
	if !b.busy.CompareAndSwap(false, true) {
		return 0
	}
	// The buffer lets a count that ends after the timeout go.
	done := make(chan badgeResult, 1)
	go func() {
		defer b.busy.Store(false)
		n, err := b.unread(ctx)
		done <- badgeResult{n: n, err: err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			b.log.Warn("inbox unread count failed", "err", res.err)
			return 0
		}
		return res.n
	case <-ctx.Done():
		b.log.Warn("inbox unread count timed out")
		return 0
	}
}

// checkScope refuses, with 403, a token whose scope this maild cannot
// grant (docs/specs/lodge-app.md, /mcp).
func checkScope(scope string) error {
	_, err := api.ParseScope(scope)
	return err
}

// openAgent connects an MCP session to maild in process, as caller agent
// with the token's name and scope, which the engine enforces. o.Agents
// tracks the connection; once maild stops, it refuses a new one.
func (o Options) openAgent(ctx context.Context, token string, scope api.Scope) (*api.Client, error) {
	server, client := net.Pipe()
	caller := api.Caller{Kind: api.CallerKindAgent, Token: token, Scope: scope}
	if !o.Agents.start(o.Ctx, func() { o.RPC.ServeConn(o.Ctx, server, o.Router, caller) }) {
		_ = server.Close()
		_ = client.Close()
		return nil, errors.New("maild is stopping")
	}
	c := api.NewClient(client)
	if _, err := c.RPC().Hello(ctx, &api.RPCHelloParams{Protocol: api.Protocol, Client: "mcp " + token}); err != nil {
		_ = c.Close()
		return nil, err
	}
	// The connection subscribes to nothing, but its views notify it.
	go func() {
		for range c.Notifications() {
		}
	}()
	return c, nil
}

// Caller is the RPC caller of a session lodge sent to /api.
func Caller(c lodgeapp.Caller) api.Caller {
	return api.Caller{Kind: api.CallerKindSession, Login: c.Login, NodeName: c.NodeName, DisplayName: c.DisplayName, Session: c.Session}
}

// Listen opens maild's lodge socket: one systemd passed, else
// /run/lodge/apps/mail.sock, or the same under LODGE_ROOT
// (docs/specs/lodge-app.md, Listeners). It admits lodge's uid only. lodge
// runs on Linux, so elsewhere (a Windows install) it refuses.
func Listen() (*lodgeapp.Listener, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("lodge runs on Linux; maild serves its local socket on %s, without --lodge", runtime.GOOS)
	}
	env, err := lodgeapp.LoadEnv()
	if err != nil {
		return nil, err
	}
	return env.Listen(ID)
}
