package lodge

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/net/websocket"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpcserver"
)

// API serves lodge's /api upgrade: a WebSocket whose text frames each carry
// one JSON-RPC message, as rpc-protocol.md's lines are, for the caller the
// request's Lodge-* headers named. lodge has checked Origin, so this does
// not (the app contract forbids it).
type API struct {
	Ctx    context.Context
	RPC    *rpcserver.Server
	Router *api.Router
	Log    *slog.Logger
}

// Serve upgrades r and serves the connection as caller until either side
// closes it or a.Ctx ends.
func (a *API) Serve(w http.ResponseWriter, r *http.Request, caller api.Caller) {
	websocket.Server{
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
		Handler: func(ws *websocket.Conn) {
			ws.PayloadType = websocket.TextFrame
			ws.MaxPayloadBytes = api.MaxMessageSize + 1 // and its newline
			a.Log.Info("lodge session started", "session", caller.Session, "node", caller.NodeName)
			a.RPC.ServeConn(a.Ctx, &frameConn{Conn: ws}, a.Router, caller)
			a.Log.Info("lodge session ended", "session", caller.Session, "node", caller.NodeName)
		},
	}.ServeHTTP(w, r)
}

// errSeveral means a frame held more than one message.
var errSeveral = errors.New("lodge: a frame holds more than one message")

// frameConn turns each frame into one line for rpcserver: a frame's
// trailing newline is optional, and a frame of several lines ends the
// connection. rpcserver writes each message, newline included, in one
// Write, which websocket sends as one frame.
type frameConn struct {
	*websocket.Conn
	buf []byte
}

func (c *frameConn) Read(p []byte) (int, error) {
	for len(c.buf) == 0 {
		var msg string
		if err := websocket.Message.Receive(c.Conn, &msg); err != nil {
			return 0, err
		}
		line := strings.TrimSuffix(msg, "\n")
		if strings.Contains(line, "\n") {
			_ = c.Conn.Close()
			return 0, errSeveral
		}
		if line != "" {
			c.buf = append([]byte(line), '\n')
		}
	}
	n := copy(p, c.buf)
	c.buf = c.buf[n:]
	return n, nil
}
