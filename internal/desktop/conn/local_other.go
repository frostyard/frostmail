//go:build !windows

package conn

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// dialLocal connects to maild's socket. On Linux the launcher or the user
// service starts maild; the app never does.
func dialLocal(ctx context.Context, socket, client string) (*api.Client, *api.Hello, error) {
	return api.Dial(ctx, socket, client)
}
