package smtpx

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/frostyard/frostmail/api"
)

// received is what the fake server saw in one accepted transaction.
type received struct {
	user, from string
	to         []string
	data       []byte
	tls        bool
}

// fakeServer is an in-process SMTP server whose behaviour each test sets.
type fakeServer struct {
	mechs    []string // AUTH mechanisms; nil means PLAIN
	password string
	rcptErr  map[string]error // RCPT replies by address

	mu   sync.Mutex
	got  []received
	data int // DATA commands seen
}

func (f *fakeServer) accepted() []received {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.got)
}

func (f *fakeServer) dataCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data
}

type fakeSession struct {
	f    *fakeServer
	conn *smtp.Conn
	cur  received
}

func (f *fakeServer) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &fakeSession{f: f, conn: c}, nil
}

func (s *fakeSession) AuthMechanisms() []string {
	if s.f.mechs != nil {
		return s.f.mechs
	}
	return []string{sasl.Plain}
}

func (s *fakeSession) Auth(mech string) (sasl.Server, error) {
	check := func(user, pass string) error {
		if pass != s.f.password {
			return &smtp.SMTPError{Code: 535, EnhancedCode: smtp.EnhancedCode{5, 7, 8}, Message: "bad credentials"}
		}
		s.cur.user = user
		return nil
	}
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(_, user, pass string) error { return check(user, pass) }), nil
	case sasl.Login:
		return &loginServer{check: check}, nil
	}
	return nil, smtp.ErrAuthUnknownMechanism
}

func (s *fakeSession) Mail(from string, _ *smtp.MailOptions) error {
	s.cur.from = from
	return nil
}

func (s *fakeSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	if err := s.f.rcptErr[to]; err != nil {
		return err
	}
	s.cur.to = append(s.cur.to, to)
	return nil
}

func (s *fakeSession) Data(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	_, s.cur.tls = s.conn.TLSConnectionState()
	s.cur.data = b
	s.f.mu.Lock()
	s.f.got = append(s.f.got, s.cur)
	s.f.data++
	s.f.mu.Unlock()
	return nil
}

func (s *fakeSession) Reset()        { s.cur = received{user: s.cur.user} }
func (s *fakeSession) Logout() error { return nil }

// loginServer is the server side of AUTH LOGIN, which go-sasl lacks.
type loginServer struct {
	check func(user, pass string) error
	step  int
	user  string
}

func (l *loginServer) Next(resp []byte) ([]byte, bool, error) {
	l.step++
	switch l.step {
	case 1:
		if resp != nil {
			l.user = string(resp)
			l.step++
			return []byte("Password:"), false, nil
		}
		return []byte("Username:"), false, nil
	case 2:
		l.user = string(resp)
		return []byte("Password:"), false, nil
	default:
		return nil, true, l.check(l.user, string(resp))
	}
}

// listen starts the fake server in the given TLS mode and returns the
// options to reach it.
func listen(t *testing.T, f *fakeServer, mode api.TLSMode, configure func(*smtp.Server)) Options {
	t.Helper()
	s := smtp.NewServer(f)
	s.Domain = "mail.test"
	s.AllowInsecureAuth = true
	cert := selfSigned(t)
	if mode == api.TLSModeStartTLS {
		s.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}
	if configure != nil {
		configure(s)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if mode == api.TLSModeTLS {
		l = tls.NewListener(l, &tls.Config{Certificates: []tls.Certificate{cert}})
	}
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Close() })
	_, port, _ := net.SplitHostPort(l.Addr().String())
	p, _ := strconv.Atoi(port)
	return Options{
		Host: "127.0.0.1", Port: p, TLS: mode,
		Username: "ann", Password: "secret",
		InsecureSkipVerify: true, Timeout: 5 * time.Second,
	}
}

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mail.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

const testMessage = "From: ann@x.test\r\nTo: bob@x.test\r\nSubject: hi\r\n\r\nHello.\r\n.leading dot\r\n"

func send(t *testing.T, opts Options, to ...string) error {
	t.Helper()
	return Send(t.Context(), opts, Envelope{From: "ann@x.test", To: to}, int64(len(testMessage)), strings.NewReader(testMessage))
}

func TestSendModes(t *testing.T) {
	for _, mode := range []api.TLSMode{api.TLSModeInsecure, api.TLSModeStartTLS, api.TLSModeTLS} {
		t.Run(string(mode), func(t *testing.T) {
			f := &fakeServer{password: "secret"}
			opts := listen(t, f, mode, nil)
			if err := send(t, opts, "bob@x.test", "carol@x.test"); err != nil {
				t.Fatalf("Send: %v", err)
			}
			got := f.accepted()
			if len(got) != 1 {
				t.Fatalf("server accepted %d messages, want 1", len(got))
			}
			r := got[0]
			if r.user != "ann" || r.from != "ann@x.test" || !slices.Equal(r.to, []string{"bob@x.test", "carol@x.test"}) {
				t.Errorf("transaction = user %q from %q to %q", r.user, r.from, r.to)
			}
			if string(r.data) != testMessage {
				t.Errorf("data = %q, want %q", r.data, testMessage)
			}
			if wantTLS := mode != api.TLSModeInsecure; r.tls != wantTLS {
				t.Errorf("TLS = %v, want %v", r.tls, wantTLS)
			}
		})
	}
}

func TestSendLoginOnly(t *testing.T) {
	f := &fakeServer{password: "secret", mechs: []string{sasl.Login}}
	opts := listen(t, f, api.TLSModeStartTLS, nil)
	if err := send(t, opts, "bob@x.test"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := f.accepted(); len(got) != 1 || got[0].user != "ann" {
		t.Fatalf("accepted = %+v, want one message from ann", got)
	}
}

func TestSendWithoutAuth(t *testing.T) {
	f := &fakeServer{}
	opts := listen(t, f, api.TLSModeInsecure, nil)
	opts.Username = ""
	if err := send(t, opts, "bob@x.test"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := f.accepted(); len(got) != 1 || got[0].user != "" {
		t.Fatalf("accepted = %+v, want one unauthenticated message", got)
	}
}

func TestSendErrors(t *testing.T) {
	temp := &smtp.SMTPError{Code: 450, EnhancedCode: smtp.EnhancedCode{4, 2, 0}, Message: "mailbox busy"}
	perm := &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "no such user"}
	cases := []struct {
		name      string
		server    *fakeServer
		mode      api.TLSMode
		configure func(*smtp.Server)
		opts      func(*Options)
		wantIs    error
		permanent bool
	}{
		{name: "wrong password", server: &fakeServer{password: "other"}, mode: api.TLSModeInsecure,
			wantIs: ErrAuth, permanent: true},
		{name: "no usable mechanism", server: &fakeServer{password: "secret", mechs: []string{"CRAM-MD5"}},
			mode: api.TLSModeInsecure, wantIs: ErrAuth, permanent: true},
		{name: "no STARTTLS", server: &fakeServer{password: "secret"}, mode: api.TLSModeInsecure,
			opts: func(o *Options) { o.TLS = api.TLSModeStartTLS }, wantIs: errConfig, permanent: true},
		{name: "recipient rejected", server: &fakeServer{password: "secret", rcptErr: map[string]error{"carol@x.test": perm}},
			mode: api.TLSModeInsecure, permanent: true},
		{name: "recipient deferred", server: &fakeServer{password: "secret", rcptErr: map[string]error{"carol@x.test": temp}},
			mode: api.TLSModeInsecure, permanent: false},
		{name: "too large", server: &fakeServer{password: "secret"}, mode: api.TLSModeInsecure,
			configure: func(s *smtp.Server) { s.MaxMessageBytes = 10 }, wantIs: ErrTooLarge, permanent: true},
		{name: "unknown mode", server: &fakeServer{password: "secret"}, mode: api.TLSModeInsecure,
			opts: func(o *Options) { o.TLS = "bogus" }, wantIs: errConfig, permanent: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := c.server
			opts := listen(t, f, c.mode, c.configure)
			if c.opts != nil {
				c.opts(&opts)
			}
			err := send(t, opts, "bob@x.test", "carol@x.test")
			if err == nil {
				t.Fatal("Send succeeded")
			}
			if c.wantIs != nil && !errors.Is(err, c.wantIs) {
				t.Errorf("Send = %v, want %v", err, c.wantIs)
			}
			if Permanent(err) != c.permanent {
				t.Errorf("Permanent(%v) = %v, want %v", err, !c.permanent, c.permanent)
			}
			if n := f.dataCalls(); n != 0 {
				t.Errorf("server took %d messages, want none", n)
			}
		})
	}
}

func TestSendConnectionFailuresAreTemporary(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	_ = l.Close()
	opts := Options{Host: "127.0.0.1", Port: addr.Port, TLS: api.TLSModeInsecure, Timeout: 5 * time.Second}
	err = send(t, opts, "bob@x.test")
	if err == nil || Permanent(err) {
		t.Fatalf("Send to a closed port = %v, permanent %v; want a temporary error", err, Permanent(err))
	}
}

func TestSendTimeoutOnSilentServer(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	conns := make(chan net.Conn, 1)
	go func() {
		c, err := l.Accept()
		if err == nil {
			conns <- c // never greet
		}
	}()
	t.Cleanup(func() {
		select {
		case c := <-conns:
			_ = c.Close()
		default:
		}
	})
	opts := Options{Host: "127.0.0.1", Port: l.Addr().(*net.TCPAddr).Port, TLS: api.TLSModeInsecure, Timeout: 200 * time.Millisecond}
	start := time.Now()
	err = send(t, opts, "bob@x.test")
	if err == nil || Permanent(err) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Send = %v; want a temporary deadline error", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("Send took %v despite a 200ms timeout", d)
	}
}

func TestMaxSize(t *testing.T) {
	f := &fakeServer{password: "secret"}
	opts := listen(t, f, api.TLSModeStartTLS, func(s *smtp.Server) { s.MaxMessageBytes = 52428800 })
	got, err := MaxSize(t.Context(), opts)
	if err != nil || got != 52428800 {
		t.Fatalf("MaxSize = %d, %v; want 52428800", got, err)
	}
}

func TestSendNeedsRecipients(t *testing.T) {
	err := Send(t.Context(), Options{}, Envelope{From: "ann@x.test"}, 1, bytes.NewReader(nil))
	if err == nil {
		t.Fatal("Send without recipients succeeded")
	}
}

func TestPermanent(t *testing.T) {
	cases := map[error]bool{
		nil:                        false,
		errors.New("reset"):        false,
		context.Canceled:           false,
		ErrAuth:                    true,
		ErrTooLarge:                true,
		&smtp.SMTPError{Code: 421}: false,
		&smtp.SMTPError{Code: 554}: true,
	}
	for err, want := range cases {
		if got := Permanent(err); got != want {
			t.Errorf("Permanent(%v) = %v, want %v", err, got, want)
		}
	}
}
