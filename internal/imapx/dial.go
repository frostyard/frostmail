// Package imapx is frostmail's only door to IMAP. It wraps the patched
// go-imap v2 client in third_party/go-imap (docs/adr/0004-go-imap-fork.md),
// so sync code never imports go-imap directly and the fork stays replaceable.
package imapx

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
	"github.com/frostyard/frostmail/api"

	"mime"
)

// DialOptions says how to reach and log in to one IMAP server.
type DialOptions struct {
	Host     string
	Port     int
	TLS      api.TLSMode
	Username string
	Password string

	// InsecureSkipVerify accepts any certificate; only for test servers
	// with self-signed certificates.
	InsecureSkipVerify bool
	// Trace receives the raw protocol exchange when set
	// (MAILD_IMAP_TRACE, docs/design/testing.md); credentials included.
	Trace io.Writer
	// Timeout bounds connecting and logging in; zero means 30s.
	Timeout time.Duration
}

// Dial connects, secures the connection as opts.TLS says, and logs in.
func Dial(ctx context.Context, opts DialOptions) (*imapclient.Client, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))
	tlsConfig := &tls.Config{ServerName: opts.Host, InsecureSkipVerify: opts.InsecureSkipVerify} //nolint:gosec // opt-in for test servers
	copts := &imapclient.Options{
		TLSConfig:   tlsConfig,
		DebugWriter: opts.Trace,
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
	}

	var d net.Dialer
	var conn net.Conn
	var err error
	switch opts.TLS {
	case api.TLSModeTLS:
		td := tls.Dialer{NetDialer: &d, Config: tlsConfig}
		conn, err = td.DialContext(ctx, "tcp", addr)
	case api.TLSModeStartTLS, api.TLSModeInsecure:
		conn, err = d.DialContext(ctx, "tcp", addr)
	default:
		return nil, fmt.Errorf("imap %s: unknown TLS mode %q", addr, opts.TLS)
	}
	if err != nil {
		return nil, fmt.Errorf("imap %s: %w", addr, err)
	}
	// Bound the greeting, STARTTLS and LOGIN by the context deadline.
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	var c *imapclient.Client
	if opts.TLS == api.TLSModeStartTLS {
		c, err = imapclient.NewStartTLS(conn, copts)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("imap %s: starttls: %w", addr, err)
		}
	} else {
		c = imapclient.New(conn, copts)
	}
	if err := c.Login(opts.Username, opts.Password).Wait(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("imap %s: login: %w", addr, err)
	}
	if !stop() {
		_ = c.Close()
		return nil, fmt.Errorf("imap %s: %w", addr, ctx.Err())
	}
	return c, nil
}
