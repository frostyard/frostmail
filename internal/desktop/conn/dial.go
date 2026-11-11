package conn

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/coder/websocket"

	"github.com/frostyard/frostmail/api"
)

// maxFrame is the largest WebSocket frame lodge sends: 16 MiB, its
// app contract's limit.
const maxFrame = 16 << 20

// Dial connects to the target's maild and completes rpc.hello as client.
func Dial(ctx context.Context, t Target, client string) (*api.Client, *api.Hello, error) {
	if t.Lodge == nil {
		return dialLocal(ctx, t.Socket, client)
	}
	nc, err := dialLodge(ctx, *t.Lodge)
	if err != nil {
		return nil, nil, err
	}
	c := api.NewClient(nc)
	h, err := c.RPC().Hello(ctx, &api.RPCHelloParams{Protocol: api.Protocol, Client: client})
	if err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	return c, h, nil
}

// dialLodge opens lodge's /app/mail/api with lodge's Origin, as a native
// client does (lodge's decision D7), and carries one JSON-RPC message per
// text frame as one per line.
func dialLodge(ctx context.Context, l Lodge) (net.Conn, error) {
	ws, _, err := websocket.Dial(ctx, l.APIURL(), &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {l.Origin()}},
	})
	if err != nil {
		return nil, fmt.Errorf("connect to lodge at %s: %w", l.Origin(), err)
	}
	ws.SetReadLimit(maxFrame)
	return lineConn(ws), nil
}

// lineConn is one end of a pipe whose other end moves lines: each text
// frame from ws arrives as a line, and each line written goes to ws as one
// text frame without its newline. A binary
// frame, or a line with no end when the pipe closes, ends the connection.
func lineConn(ws *websocket.Conn) net.Conn {
	app, wire := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	stop := func() {
		cancel()
		_ = wire.Close()
		_ = ws.Close(websocket.StatusNormalClosure, "")
	}
	go func() { // frames in
		defer stop()
		for {
			typ, data, err := ws.Read(ctx)
			if err != nil || typ != websocket.MessageText {
				return
			}
			data = bytes.TrimRight(data, "\n")
			if _, err := wire.Write(append(data, '\n')); err != nil {
				return
			}
		}
	}()
	go func() { // lines out
		defer stop()
		r := bufio.NewReaderSize(wire, 64<<10)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			if err := ws.Write(ctx, websocket.MessageText, bytes.TrimRight(line, "\n")); err != nil {
				return
			}
		}
	}()
	return app
}
