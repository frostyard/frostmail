// Package replay serves a recorded IMAP session back to a client
// (docs/design/testing.md, transcript replay). A script is a trace as
// imapx writes it: "C: " lines the client sent and "S: " lines the server
// answered. Each command the client sends is answered with the first
// unused recorded exchange for the same command, in its recorded order;
// commands may come in another order than recorded (clients pipeline),
// and the client's tags are mapped to the recorded ones. A command the
// script does not expect fails the test, and so does a recorded command
// the client never sends (LOGOUT aside).
package replay

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Line is one line of a script.
type Line struct {
	Sent bool   // the client sent it ("C: ")
	Text string // without the prefix and the line ending
}

// Parse reads a script; blank lines and lines starting with "#" are skipped.
func Parse(r io.Reader) ([]Line, error) {
	var out []Line
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for n := 1; sc.Scan(); n++ {
		text := sc.Text()
		switch {
		case text == "" || strings.HasPrefix(text, "#"):
		case strings.HasPrefix(text, "C: "):
			out = append(out, Line{Sent: true, Text: text[3:]})
		case strings.HasPrefix(text, "S: "):
			out = append(out, Line{Text: text[3:]})
		default:
			return nil, fmt.Errorf("replay: line %d has no C: or S: prefix", n)
		}
	}
	return out, sc.Err()
}

// Load reads a script from a file.
func Load(path string) ([]Line, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Server plays one script to one connection on a loopback address.
type Server struct {
	ln     net.Listener
	script []Line
	done   chan struct{}
	once   sync.Once

	mu     sync.Mutex
	failed error
}

// Start serves script on 127.0.0.1 for one connection. Unless Close was
// called first, the end of the test closes the server and fails the test
// if the session left the script.
func Start(t testing.TB, script []Line) *Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{ln: ln, script: script, done: make(chan struct{})}
	go s.serve()
	t.Cleanup(func() {
		closed := true
		s.once.Do(func() { closed = false })
		if !closed {
			if err := s.shut(); err != nil {
				t.Errorf("replay: %v", err)
			}
		}
	})
	return s
}

// Close waits for the session to end (the client must have closed its
// connection) and returns the first mismatch, if any.
func (s *Server) Close() error {
	s.once.Do(func() {})
	return s.shut()
}

func (s *Server) shut() error {
	_ = s.ln.Close()
	<-s.done
	return s.Err()
}

// Addr is the host:port to dial (plain IMAP, no TLS).
func (s *Server) Addr() string { return s.ln.Addr().String() }

// Err is the first mismatch, or nil while the session follows the script.
func (s *Server) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed
}

func (s *Server) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed == nil {
		s.failed = err
	}
}

func (s *Server) serve() {
	defer close(s.done)
	conn, err := s.ln.Accept()
	if err != nil {
		s.fail(errors.New("no client connected"))
		return
	}
	defer conn.Close()
	if err := play(conn, s.script); err != nil {
		s.fail(err)
	}
}

// exchange is one recorded command: its command line, then every line
// that belongs to it in recorded order (continuation lines from the
// client, and the server's continuation requests, untagged data and
// tagged completion).
type exchange struct {
	lines []Line
	used  bool
}

// split divides a script into the greeting and the exchanges. Untagged
// server lines belong to the next completion of a command (or, while a
// command waits on a continuation, to that command); lines that continue a
// response, such as literal data, follow the line before them.
func split(script []Line) (greeting []Line, exchanges []*exchange) {
	open := map[string]*exchange{}
	var last, continued *exchange // the newest command; one in continuation
	var pending []Line            // untagged lines awaiting a completion
	prevPending := false          // the previous server line went to pending
	for _, l := range script {
		if l.Sent {
			tag, rest, ok := strings.Cut(l.Text, " ")
			if ok && isTag(tag) && isCommand(rest) && (continued == nil || continued != last) {
				x := &exchange{lines: []Line{l}}
				exchanges = append(exchanges, x)
				open[tag], last = x, x
				continue
			}
			if last != nil {
				last.lines = append(last.lines, l)
			}
			continue
		}
		tag, rest, _ := strings.Cut(l.Text, " ")
		word, _, _ := strings.Cut(rest, " ")
		switch {
		case last == nil:
			greeting = append(greeting, l)
		case strings.HasPrefix(l.Text, "+"):
			last.lines = append(last.lines, l)
			continued, prevPending = last, false
		case open[tag] != nil && (word == "OK" || word == "NO" || word == "BAD"):
			x := open[tag]
			x.lines = append(append(x.lines, pending...), l)
			pending, prevPending = nil, false
			delete(open, tag)
			if continued == x {
				continued = nil
			}
		case continued != nil:
			continued.lines = append(continued.lines, l)
		case tag == "*" || prevPending:
			pending, prevPending = append(pending, l), true
		default:
			last.lines = append(last.lines, l)
		}
	}
	if len(pending) > 0 && last != nil {
		last.lines = append(last.lines, pending...)
	}
	return greeting, exchanges
}

// isCommand reports whether the text after a tag starts a command: an
// uppercase word, not a response's status.
func isCommand(rest string) bool {
	word, _, _ := strings.Cut(rest, " ")
	if word == "" || word == "OK" || word == "NO" || word == "BAD" || word == "BYE" || word == "PREAUTH" {
		return false
	}
	return strings.ToUpper(word) == word && !strings.ContainsFunc(word, func(r rune) bool { return r < 'A' || r > 'Z' })
}

// play answers each command the client sends with the first unused
// recorded exchange that matches it, so commands the client pipelines in
// another order still replay; within an exchange the recorded order holds.
func play(conn net.Conn, script []Line) error {
	r := bufio.NewReader(conn)
	tags := map[string]string{} // recorded tag → the client's
	greeting, exchanges := split(script)
	for _, l := range greeting {
		if _, err := io.WriteString(conn, l.Text+"\r\n"); err != nil {
			return fmt.Errorf("greeting: %w", err)
		}
	}
	read := func() (string, error) {
		got, err := r.ReadString('\n')
		return strings.TrimRight(got, "\r\n"), err
	}
	for {
		got, err := read()
		if err != nil {
			break
		}
		x := find(exchanges, got)
		if x == nil {
			if _, rest, _ := strings.Cut(got, " "); strings.EqualFold(rest, "LOGOUT") {
				continue
			}
			return fmt.Errorf("the client sent %q, which the script does not expect", got)
		}
		x.used = true
		if err := match(x.lines[0].Text, got, tags); err != nil {
			return err
		}
		for _, l := range x.lines[1:] {
			if !l.Sent {
				if _, err := io.WriteString(conn, mapTag(l.Text, tags)+"\r\n"); err != nil {
					return fmt.Errorf("write: %w", err)
				}
				continue
			}
			got, err := read()
			if err != nil {
				return fmt.Errorf("want %q, the client sent nothing more (%w)", l.Text, err)
			}
			if err := match(l.Text, got, tags); err != nil {
				return err
			}
		}
	}
	for _, x := range exchanges {
		if _, rest, _ := strings.Cut(x.lines[0].Text, " "); !x.used && !strings.EqualFold(rest, "LOGOUT") {
			return fmt.Errorf("the client never sent %q", x.lines[0].Text)
		}
	}
	return nil
}

// find returns the first unused exchange whose command matches got.
func find(exchanges []*exchange, got string) *exchange {
	for _, x := range exchanges {
		if !x.used && match(x.lines[0].Text, got, map[string]string{}) == nil {
			return x
		}
	}
	return nil
}

// match compares a client line with the recorded one, learning the
// client's tag for a command. Redacted credentials match anything.
func match(want, got string, tags map[string]string) error {
	if want == "[redacted]" {
		return nil
	}
	wantTag, wantRest, wantCmd := strings.Cut(want, " ")
	gotTag, gotRest, gotCmd := strings.Cut(got, " ")
	if wantCmd && gotCmd && isTag(wantTag) && isTag(gotTag) {
		if prefix, ok := strings.CutSuffix(wantRest, " [redacted]"); ok && strings.HasPrefix(gotRest, prefix+" ") {
			tags[wantTag] = gotTag
			return nil
		}
		if normalize(wantRest) == normalize(gotRest) {
			tags[wantTag] = gotTag
			return nil
		}
	} else if want == got {
		return nil
	}
	return fmt.Errorf("want %q, the client sent %q", want, got)
}

// normalize sorts the items of a FETCH command, which clients may send in
// any order (go-imap's vary between runs).
func normalize(cmd string) string {
	upper := strings.ToUpper(cmd)
	if !strings.HasPrefix(upper, "FETCH ") && !strings.HasPrefix(upper, "UID FETCH ") {
		return cmd
	}
	open := strings.IndexByte(cmd, '(')
	if open < 0 || !strings.HasSuffix(cmd, ")") {
		return cmd
	}
	items := splitItems(cmd[open+1 : len(cmd)-1])
	slices.Sort(items)
	return cmd[:open+1] + strings.Join(items, " ") + ")"
}

// splitItems splits a list on spaces outside brackets, parentheses and
// quotes.
func splitItems(list string) []string {
	var out []string
	depth, quoted, start := 0, false, 0
	for i := 0; i < len(list); i++ {
		switch c := list[i]; {
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		case c == ' ' && depth == 0:
			out = append(out, list[start:i])
			start = i + 1
		}
	}
	return append(out, list[start:])
}

// isTag reports whether a word can be a command tag: letters and digits
// that are not a continuation or untagged marker.
func isTag(w string) bool {
	if w == "" || w == "*" || w == "+" {
		return false
	}
	return !strings.ContainsFunc(w, func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.'
	})
}

// mapTag rewrites a tagged server line to the client's tag.
func mapTag(line string, tags map[string]string) string {
	tag, rest, ok := strings.Cut(line, " ")
	if mapped, known := tags[tag]; ok && known {
		return mapped + " " + rest
	}
	return line
}
