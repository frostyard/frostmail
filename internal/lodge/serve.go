package lodge

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"runtime"

	"github.com/frostyard/lodge/kit/lodgeapp"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpcserver"
)

// ID is the mail app's id in lodge: its socket's name and URL segment.
const ID = "mail"

// Options are what maild's lodge socket serves.
type Options struct {
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
// /api as caller session and /content/ as caller content. lodge's other
// routes answer 404.
func NewServer(o Options) *lodgeapp.Server {
	log := o.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
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
		Logger:  log,
	}
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
