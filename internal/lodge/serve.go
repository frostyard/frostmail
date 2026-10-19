package lodge

import (
	"context"
	"log/slog"
	"net"

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
	// request that opened their MCP session: maild's own context.
	Ctx context.Context
	// Version is maild's, for /health.
	Version string
	// RPC and Router serve /api, as the private socket does.
	RPC    *rpcserver.Server
	Router *api.Router
	// Content serves /content/.
	Content *Content
	Log     *slog.Logger
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
	mcp := mcpsession.New(mcpserver.Handler(mcpserver.HTTPOptions{Open: o.openAgent, Version: o.Version, Log: log}),
		mcpsession.Options{CheckScope: checkScope, Logger: log})
	return &lodgeapp.Server{
		Health: func(context.Context) lodgeapp.Health {
			return lodgeapp.Health{OK: true, Version: o.Version, Protocol: api.Protocol}
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

// checkScope refuses, with 403, a token whose scope this maild cannot
// grant (docs/specs/lodge-app.md, /mcp).
func checkScope(scope string) error {
	_, err := api.ParseScope(scope)
	return err
}

// openAgent connects an MCP session to maild in process, as caller agent
// with the token's name and scope, which the engine enforces.
func (o Options) openAgent(ctx context.Context, token string, scope api.Scope) (*api.Client, error) {
	server, client := net.Pipe()
	go o.RPC.ServeConn(o.Ctx, server, o.Router, api.Caller{Kind: api.CallerKindAgent, Token: token, Scope: scope})
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
// (docs/specs/lodge-app.md, Listeners). It admits lodge's uid only.
func Listen() (*lodgeapp.Listener, error) {
	env, err := lodgeapp.LoadEnv()
	if err != nil {
		return nil, err
	}
	return env.Listen(ID)
}
