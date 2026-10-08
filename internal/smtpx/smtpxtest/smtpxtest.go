// Package smtpxtest runs an in-process SMTP submission server for tests of
// sending: it records accepted messages and can refuse or hold them.
package smtpxtest

import (
	"io"
	"net"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/frostyard/frostmail/api"
)

// Message is one accepted message.
type Message struct {
	From string
	To   []string
	Data []byte
}

// Server is a running test server. It accepts AUTH PLAIN with any user
// name and its password, over plain TCP.
type Server struct {
	Addr     string
	password string

	mu      sync.Mutex
	changed chan struct{} // closed and replaced whenever state changes
	msgs    []Message
	rcpt    []error // replies for the next RCPT commands
	hold    *hold
	held    int
}

// hold makes DATA wait until done is closed; accept is set before.
type hold struct {
	done   chan struct{}
	accept bool
}

// Start runs a server until the test ends.
func Start(t testing.TB, password string) *Server {
	t.Helper()
	s := &Server{password: password, changed: make(chan struct{})}
	srv := smtp.NewServer(s)
	srv.Domain = "smtp.test"
	srv.AllowInsecureAuth = true
	srv.ReadTimeout = 10 * time.Second
	srv.WriteTimeout = 10 * time.Second
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.Addr = ln.Addr().String()
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		s.mu.Lock()
		if h := s.hold; h != nil {
			s.hold = nil
			close(h.done)
		}
		s.mu.Unlock()
		_ = srv.Close()
	})
	return s
}

// Config is the account setting that reaches the server.
func (s *Server) Config(username string) *api.ServerConfig {
	host, port, _ := net.SplitHostPort(s.Addr)
	p, _ := strconv.Atoi(port)
	return &api.ServerConfig{Host: host, Port: int64(p), TLS: api.TLSModeInsecure, Username: username}
}

// Messages returns the accepted messages so far.
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.msgs)
}

// wait waits until cond holds, failing the test after timeout.
func (s *Server) wait(t testing.TB, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		s.mu.Lock()
		ok, ch := cond(), s.changed
		s.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-ch:
		case <-deadline:
			t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
	}
}

// WaitMessages waits until n messages were accepted and returns them.
func (s *Server) WaitMessages(t testing.TB, n int, timeout time.Duration) []Message {
	t.Helper()
	s.wait(t, timeout, strconv.Itoa(n)+" messages", func() bool { return len(s.msgs) >= n })
	return s.Messages()
}

// RefuseRecipients makes the next RCPT commands fail with these replies,
// one each, before accepting again.
func (s *Server) RefuseRecipients(errs ...*smtp.SMTPError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range errs {
		s.rcpt = append(s.rcpt, e)
	}
}

// Hold makes DATA wait, once the whole message has arrived, until release
// is called: release(true) accepts every held message and release(false)
// refuses them with a 451.
func (s *Server) Hold() (release func(accept bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := &hold{done: make(chan struct{})}
	s.hold = h
	return func(accept bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.hold != h {
			return
		}
		s.hold = nil
		h.accept = accept
		close(h.done)
	}
}

// WaitHeld waits until n messages are held in DATA.
func (s *Server) WaitHeld(t testing.TB, n int, timeout time.Duration) {
	t.Helper()
	s.wait(t, timeout, strconv.Itoa(n)+" held messages", func() bool { return s.held >= n })
}

func (s *Server) notify() { close(s.changed); s.changed = make(chan struct{}) }

// NewSession implements smtp.Backend.
func (s *Server) NewSession(*smtp.Conn) (smtp.Session, error) { return &session{s: s}, nil }

type session struct {
	s    *Server
	from string
	to   []string
}

func (ss *session) AuthMechanisms() []string { return []string{sasl.Plain} }

func (ss *session) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, _, pass string) error {
		if pass != ss.s.password {
			return &smtp.SMTPError{Code: 535, EnhancedCode: smtp.EnhancedCode{5, 7, 8}, Message: "bad credentials"}
		}
		return nil
	}), nil
}

func (ss *session) Mail(from string, _ *smtp.MailOptions) error {
	ss.from = from
	return nil
}

func (ss *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	ss.s.mu.Lock()
	defer ss.s.mu.Unlock()
	if len(ss.s.rcpt) > 0 {
		err := ss.s.rcpt[0]
		ss.s.rcpt = ss.s.rcpt[1:]
		return err
	}
	ss.to = append(ss.to, to)
	return nil
}

func (ss *session) Data(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	ss.s.mu.Lock()
	h := ss.s.hold
	if h != nil {
		ss.s.held++
		ss.s.notify()
	}
	ss.s.mu.Unlock()
	if h != nil {
		<-h.done
		if !h.accept {
			return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "held message dropped"}
		}
	}
	ss.s.mu.Lock()
	defer ss.s.mu.Unlock()
	ss.s.msgs = append(ss.s.msgs, Message{From: ss.from, To: slices.Clone(ss.to), Data: data})
	ss.s.notify()
	return nil
}

func (ss *session) Reset() {
	ss.from, ss.to = "", nil
}

func (ss *session) Logout() error { return nil }
