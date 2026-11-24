//go:build !js

package conn

import (
	"context"
	"errors"
	"github.com/coder/websocket"
	"net/http"
)

var errRedirected = errors.New("lodge answered with a redirect, which the app does not follow")

// dialClient follows no redirect: one could take the upgrade to plain http
// or to another origin, past ParseLodge's rule.
var dialClient = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errRedirected }}

func openLodgeSocket(ctx context.Context, l Lodge) (*websocket.Conn, error) {
	ws, _, err := websocket.Dial(ctx, l.APIURL(), &websocket.DialOptions{
		HTTPClient: dialClient,
		HTTPHeader: http.Header{"Origin": {l.Origin()}},
	})
	return ws, err
}
