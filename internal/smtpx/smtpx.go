// Package smtpx is frostmail's only door to SMTP: it hands one built
// message to an account's submission server (docs/design/send.md, Outbox)
// and says whether a failure is worth retrying.
package smtpx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/frostyard/frostmail/api"
)

var (
	// ErrAuth means the server rejected the credentials, or offers no
	// mechanism frostmail can use. Retrying with the same password will not
	// help.
	ErrAuth = errors.New("smtpx: login rejected")
	// ErrTooLarge means the message is over the size the server announced.
	ErrTooLarge = errors.New("smtpx: message larger than the server accepts")
)

// Options says how to reach and log in to one SMTP server.
type Options struct {
	Host     string
	Port     int
	TLS      api.TLSMode
	Username string // "" skips AUTH
	Password string

	// InsecureSkipVerify accepts any certificate; only for test servers
	// with self-signed certificates.
	InsecureSkipVerify bool
	// Trace receives the raw protocol exchange when set; credentials and
	// message included.
	Trace io.Writer
	// Timeout bounds connecting, the greeting, STARTTLS and AUTH; zero means
	// 30s. Later commands use go-smtp's per-command timeouts.
	Timeout time.Duration
}

// Envelope is one message's SMTP envelope.
type Envelope struct {
	From string
	To   []string
}

// Send connects, secures and authenticates as opts says, and submits the
// size bytes read from msg to env's recipients. It returns nil only after
// the server accepted the message. A rejected recipient aborts the whole
// transaction. Cancelling ctx closes the connection.
func Send(ctx context.Context, opts Options, env Envelope, size int64, msg io.Reader) error {
	if len(env.To) == 0 {
		return errors.New("smtpx: no recipients")
	}
	c, err := dial(ctx, opts)
	if err != nil {
		return err
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()

	if max, ok := c.MaxMessageSize(); ok && max > 0 && size > int64(max) {
		return fmt.Errorf("%w: %d bytes, limit %d", ErrTooLarge, size, max)
	}
	if err := c.Mail(env.From, &smtp.MailOptions{Size: size}); err != nil {
		return fmt.Errorf("smtp MAIL FROM %s: %w", env.From, ctxErr(ctx, err))
	}
	for _, to := range env.To {
		if err := c.Rcpt(to, nil); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", to, ctxErr(ctx, err))
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", ctxErr(ctx, err))
	}
	if _, err := io.Copy(w, msg); err != nil {
		return fmt.Errorf("smtp DATA: %w", ctxErr(ctx, err))
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp DATA: %w", ctxErr(ctx, err))
	}
	// The message is accepted; a failed QUIT changes nothing.
	_ = c.Quit()
	return nil
}

// MaxSize connects and logs in as Send does and returns the message size
// limit the server announces (SIZE), or 0 when it announces none.
func MaxSize(ctx context.Context, opts Options) (int64, error) {
	c, err := dial(ctx, opts)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	max, _ := c.MaxMessageSize()
	_ = c.Quit()
	return int64(max), nil
}

// Permanent reports whether err will happen again on a retry: the server
// refused with a 5xx reply, rejected the login, does not support what the
// account needs, or the message is too large. Connection failures, 4xx
// replies and cancellation are temporary.
func Permanent(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrAuth) || errors.Is(err, ErrTooLarge) || errors.Is(err, errConfig) {
		return true
	}
	var se *smtp.SMTPError
	if errors.As(err, &se) {
		return se.Code >= 500 && se.Code < 600
	}
	return false
}

// errConfig marks a server that cannot do what the account says (no
// STARTTLS, no AUTH).
var errConfig = errors.New("smtpx: server does not support the account's settings")

// dial connects, secures the connection, says EHLO and authenticates,
// bounded by opts.Timeout: when it runs out the connection is closed, which
// go-smtp's own per-command deadlines cannot undo.
func dial(ctx context.Context, opts Options) (*smtp.Client, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))
	tlsConfig := &tls.Config{ServerName: opts.Host, InsecureSkipVerify: opts.InsecureSkipVerify} //nolint:gosec // opt-in for test servers
	var d net.Dialer
	var conn net.Conn
	var err error
	switch opts.TLS {
	case api.TLSModeTLS:
		td := tls.Dialer{NetDialer: &d, Config: tlsConfig}
		conn, err = td.DialContext(dctx, "tcp", addr)
	case api.TLSModeStartTLS, api.TLSModeInsecure:
		conn, err = d.DialContext(dctx, "tcp", addr)
	default:
		return nil, fmt.Errorf("smtp %s: %w: unknown TLS mode %q", addr, errConfig, opts.TLS)
	}
	if err != nil {
		return nil, fmt.Errorf("smtp %s: %w", addr, err)
	}
	stop := context.AfterFunc(dctx, func() { _ = conn.Close() })
	defer stop()

	var c *smtp.Client
	if opts.TLS == api.TLSModeStartTLS {
		c, err = smtp.NewClientStartTLS(conn, tlsConfig)
		if err != nil {
			_ = conn.Close()
			// go-smtp reports a missing extension as a plain error.
			if strings.Contains(err.Error(), "doesn't support STARTTLS") {
				err = fmt.Errorf("%w: %w", errConfig, err)
			}
			return nil, fmt.Errorf("smtp %s: starttls: %w", addr, ctxErr(dctx, err))
		}
	} else {
		c = smtp.NewClient(conn)
	}
	c.DebugWriter = opts.Trace
	if opts.Username != "" {
		err = authenticate(c, opts)
	} else {
		err = c.Hello("localhost")
	}
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("smtp %s: %w", addr, ctxErr(dctx, err))
	}
	if !stop() {
		_ = c.Close()
		return nil, fmt.Errorf("smtp %s: %w", addr, dctx.Err())
	}
	return c, nil
}

// authenticate logs in with PLAIN, or LOGIN when the server offers only
// that. A rejection wraps ErrAuth.
func authenticate(c *smtp.Client, opts Options) error {
	var mech sasl.Client
	switch {
	case c.SupportsAuth(sasl.Plain):
		mech = sasl.NewPlainClient("", opts.Username, opts.Password)
	case c.SupportsAuth(sasl.Login):
		mech = sasl.NewLoginClient(opts.Username, opts.Password)
	default:
		return fmt.Errorf("auth: %w: neither PLAIN nor LOGIN offered", ErrAuth)
	}
	if err := c.Auth(mech); err != nil {
		var se *smtp.SMTPError
		if errors.As(err, &se) && se.Code >= 500 {
			return fmt.Errorf("auth: %w: %w", ErrAuth, err)
		}
		return fmt.Errorf("auth: %w", err)
	}
	return nil
}

// ctxErr prefers the context's error when ctx ended, since a closed
// connection's error says less.
func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w (%w)", ctx.Err(), err)
	}
	return err
}
