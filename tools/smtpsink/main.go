// Command smtpsink is an SMTP submission server for the app's end-to-end
// tests: it accepts AUTH PLAIN with any credentials over plain TCP and
// writes each accepted message to DIR/N.eml with its envelope in DIR/N.json.
// It prints "listening ADDR" once it accepts connections. Never expose it.
package main

import (
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "smtpsink:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("smtpsink", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:0", "listen address (loopback only)")
	dir := fs.String("dir", "", "directory for accepted messages (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return errors.New("-dir is required")
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("-addr %s is not a loopback address", *addr)
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	s := smtp.NewServer(&sink{dir: *dir})
	s.Domain = "smtpsink.test"
	s.AllowInsecureAuth = true
	fmt.Println("listening", ln.Addr().String())
	return s.Serve(ln)
}

type sink struct {
	dir string
	mu  sync.Mutex
	n   int
}

func (k *sink) NewSession(*smtp.Conn) (smtp.Session, error) { return &session{k: k}, nil }

type session struct {
	k    *sink
	from string
	to   []string
}

func (s *session) AuthMechanisms() []string { return []string{sasl.Plain} }

func (s *session) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(string, string, string) error { return nil }), nil
}

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.k.mu.Lock()
	defer s.k.mu.Unlock()
	s.k.n++
	base := filepath.Join(s.k.dir, strconv.Itoa(s.k.n))
	env, err := json.Marshal(map[string]any{"from": s.from, "to": s.to})
	if err != nil {
		return err
	}
	if err := os.WriteFile(base+".json", env, 0o600); err != nil {
		return err
	}
	return os.WriteFile(base+".eml", data, 0o600)
}

func (s *session) Reset()        { s.from, s.to = "", nil }
func (s *session) Logout() error { return nil }
