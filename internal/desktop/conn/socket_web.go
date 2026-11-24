//go:build js && wasm

package conn

import (
	"context"
	"errors"
	"github.com/coder/websocket"
	"syscall/js"
)

// The browser supplies its own Origin and session. The shell admits the app
// before it starts and its API always belongs to the embedding site's origin.
func openLodgeSocket(ctx context.Context, l Lodge) (*websocket.Conn, error) {
	if l.Origin() != js.Global().Get("location").Get("origin").String() {
		return nil, errors.New("the mail API must be on this Lodge's origin")
	}
	ws, _, err := websocket.Dial(ctx, l.APIURL(), nil)
	return ws, err
}
