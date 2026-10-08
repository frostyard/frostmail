// Package imapx is frostmail's only door to IMAP. It wraps the patched
// go-imap v2 client in third_party/go-imap (docs/adr/0004-go-imap-fork.md),
// so sync code never imports go-imap directly and the fork stays replaceable.
package imapx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/oauth"

	"mime"
)

// ErrAuth means the server rejected the credentials. Retrying with the same
// password will not help.
var ErrAuth = errors.New("imapx: login rejected")

// ErrReadOnly means a read-only session was asked to change the server.
var ErrReadOnly = errors.New("imapx: the session is read-only")

// DialOptions says how to reach and log in to one IMAP server.
type DialOptions struct {
	Host     string
	Port     int
	TLS      api.TLSMode
	Username string
	Password string
	// OAuth makes Password an OAuth access token, sent with AUTHENTICATE
	// XOAUTH2 instead of LOGIN.
	OAuth bool
	// ReadOnly opens mailboxes with EXAMINE, and the session refuses every
	// command that would change the server with ErrReadOnly.
	ReadOnly bool

	// InsecureSkipVerify accepts any certificate; only for test servers
	// with self-signed certificates.
	InsecureSkipVerify bool
	// Trace, when set, receives the session's lines with their direction
	// and credentials redacted (trace.go); a Closer is closed with the
	// connection. STARTTLS connections are not traced.
	Trace io.Writer
	// Timeout bounds connecting and logging in; zero means 30s.
	Timeout time.Duration
	// OnUpdate, if set, is called from the connection's reader for every
	// unsolicited EXISTS, EXPUNGE or FETCH. It must not block.
	OnUpdate func()
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
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
	}
	if f := opts.OnUpdate; f != nil {
		copts.UnilateralDataHandler = &imapclient.UnilateralDataHandler{
			Expunge: func(uint32) { f() },
			Mailbox: func(d *imapclient.UnilateralDataMailbox) {
				if d.NumMessages != nil {
					f()
				}
			},
			Fetch: func(*imapclient.FetchMessageData) { f() },
		}
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
	if opts.Trace != nil && opts.TLS != api.TLSModeStartTLS {
		conn = &traceConn{Conn: conn, t: newTrace(opts.Trace)}
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
	if err := login(c, opts); err != nil {
		_ = c.Close()
		var imapErr *imap.Error
		if errors.As(err, &imapErr) && imapErr.Type == imap.StatusResponseTypeNo {
			return nil, fmt.Errorf("imap %s: login: %w: %w", addr, ErrAuth, err)
		}
		return nil, fmt.Errorf("imap %s: login: %w", addr, err)
	}
	if !stop() {
		_ = c.Close()
		return nil, fmt.Errorf("imap %s: %w", addr, ctx.Err())
	}
	return c, nil
}

// login signs in with LOGIN, or AUTHENTICATE XOAUTH2 for OAuth; a refused
// token carries the server's failure details.
func login(c *imapclient.Client, opts DialOptions) error {
	if !opts.OAuth {
		return c.Login(opts.Username, opts.Password).Wait()
	}
	sc := oauth.SASL(opts.Username, opts.Password)
	if err := c.Authenticate(sc); err != nil {
		if details := sc.Err(); details != nil {
			return fmt.Errorf("%w (%w)", err, details)
		}
		return err
	}
	return nil
}
