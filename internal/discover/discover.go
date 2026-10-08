// Package discover finds an address's mail servers (docs/design/accounts.md,
// Discovery): provider profiles, Mozilla autoconfig documents (also served
// by the ISPDB) and RFC 6186 SRV records. The readers here are pure; the
// engine fetches documents and queries DNS.
package discover

import (
	"context"
	"errors"
	"net"

	"github.com/frostyard/frostmail/api"
)

// ErrNothing means a source offered no usable server.
var ErrNothing = errors.New("discover: no usable server")

// Server is one server's settings.
type Server struct {
	Host     string
	Port     int
	TLS      api.TLSMode // tls or starttls; never insecure
	Username string
}

// Settings are the servers a source found; either may be nil.
type Settings struct {
	IMAP *Server
	SMTP *Server
}

// Resolver looks up SRV records; *net.Resolver satisfies it.
type Resolver interface {
	LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
}
