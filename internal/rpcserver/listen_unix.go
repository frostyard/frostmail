//go:build !windows

package rpcserver

import (
	"log/slog"
	"net"

	"github.com/frostyard/lodge/kit/jsonrpc"
)

// Listen creates the maild socket at path (docs/specs/rpc-protocol.md,
// Transport) with lodge's kit: mode 0600, in a directory that must be
// private (0700 and owned by the user; it is created if missing). A stale
// socket is replaced; a live one means maild is already running. Accept
// admits only processes of the user running maild, and closes the rest,
// logged to log.
func Listen(path string, log *slog.Logger) (net.Listener, error) { return jsonrpc.Listen(path, log) }
