// Package replay serves a recorded IMAP session back to a client
// (docs/design/testing.md, transcript replay). A script is a trace as
// imapx writes it: "C: " lines the client sent and "S: " lines the server
// answered. Each command the client sends is answered with the first
// unused recorded exchange for the same command, in its recorded order;
// commands may come in another order than recorded (clients pipeline),
// and the client's tags are mapped to the recorded ones. A command the
// script does not expect fails the test, and so does a recorded command
// the client never sends (LOGOUT aside, which is answered when the script
// has none).
package replay

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strconv"
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
		case text == "C:" || text == "S:": // an empty line whose space an editor trimmed
			out = append(out, Line{Sent: text == "C:"})
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
	lines   []Line
	used    bool
	mailbox string // selected when the command was recorded
}

// Split divides a script into the server's greeting and its exchanges:
// each exchange is a command line followed by every line that belongs to
// it (see split), so tools/imaprec can rewrite a script command by
// command.
func Split(script []Line) (greeting []Line, exchanges [][]Line) {
	greeting, xs := split(script)
	for _, x := range xs {
		exchanges = append(exchanges, x.lines)
	}
	return greeting, exchanges
}

// split divides a script into the greeting and the exchanges. Untagged
// server lines belong to the next completion of a command (or, while a
// command waits on a continuation, to that command). A literal's data,
// whatever its lines look like, follows the line that announced it: a
// body line starting with "+" is not a continuation request, and one that
// reads like a command is not a new command. A trace drops the CR of each
// line ending, so literal sizes are counted assuming CRLF; a tagged
// completion of an open command ends any literal that miscounted.
func split(script []Line) (greeting []Line, exchanges []*exchange) {
	open := map[string]*exchange{}
	var last, continued *exchange // the newest command; one in continuation
	var pending []Line            // untagged lines awaiting a completion
	prevPending := false          // the previous server line went to pending
	var into *[]Line              // where the previous server line went
	sentLeft, recvLeft := 0, 0    // bytes left of a literal, each way
	for _, l := range script {
		if l.Sent {
			if sentLeft > 0 && last != nil {
				sentLeft = afterLiteral(l.Text, sentLeft)
				last.lines = append(last.lines, l)
				continue
			}
			sentLeft = literalSize(l.Text)
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
		completes := open[tag] != nil && (word == "OK" || word == "NO" || word == "BAD")
		if recvLeft > 0 && into != nil && !completes {
			recvLeft = afterLiteral(l.Text, recvLeft)
			*into = append(*into, l)
			continue
		}
		recvLeft = literalSize(l.Text)
		switch {
		case last == nil:
			greeting = append(greeting, l)
			into = &greeting
		case strings.HasPrefix(l.Text, "+"):
			last.lines = append(last.lines, l)
			continued, prevPending, into = last, false, &last.lines
		case completes:
			x := open[tag]
			x.lines = append(append(x.lines, pending...), l)
			pending, prevPending, into = nil, false, &x.lines
			delete(open, tag)
			if continued == x {
				continued = nil
			}
		case continued != nil:
			continued.lines = append(continued.lines, l)
			into = &continued.lines
		case tag == "*" || prevPending:
			pending, prevPending, into = append(pending, l), true, &pending
		default:
			last.lines = append(last.lines, l)
			into = &last.lines
		}
	}
	if len(pending) > 0 && last != nil {
		last.lines = append(last.lines, pending...)
	}
	selected := ""
	for _, x := range exchanges {
		x.mailbox = selected
		selected = selectedAfter(selected, x.lines[0].Text)
	}
	return greeting, exchanges
}

// selectedAfter is the mailbox selected after a command: the one SELECT or
// EXAMINE names, none after CLOSE or UNSELECT, else the same.
func selectedAfter(selected, cmd string) string {
	_, rest, _ := strings.Cut(cmd, " ")
	name, args, _ := strings.Cut(rest, " ")
	switch strings.ToUpper(name) {
	case "SELECT", "EXAMINE":
		mailbox := firstArg(args)
		if strings.EqualFold(mailbox, "INBOX") {
			return "INBOX"
		}
		return mailbox
	case "CLOSE", "UNSELECT":
		return ""
	}
	return selected
}

// firstArg reads a command's first argument, an atom or a quoted string.
func firstArg(args string) string {
	if !strings.HasPrefix(args, `"`) {
		arg, _, _ := strings.Cut(args, " ")
		return arg
	}
	var b strings.Builder
	for i := 1; i < len(args); i++ {
		switch c := args[i]; c {
		case '\\':
			if i++; i < len(args) {
				b.WriteByte(args[i])
			}
		case '"':
			return b.String()
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// anywhere lists the commands that do not act on the selected mailbox,
// which a client may send in another order than recorded whatever is
// selected. Every other command must come with the mailbox selected that
// was selected when it was recorded: a UID SEARCH reads the same in every
// mailbox, and must get the answer of the right one.
var anywhere = map[string]bool{
	"SELECT": true, "EXAMINE": true, "LIST": true, "LSUB": true, "XLIST": true, "STATUS": true,
	"CAPABILITY": true, "ID": true, "LOGIN": true, "AUTHENTICATE": true, "ENABLE": true,
	"NAMESPACE": true, "LOGOUT": true, "APPEND": true, "CREATE": true, "DELETE": true, "RENAME": true,
	"SUBSCRIBE": true, "UNSUBSCRIBE": true, "COMPRESS": true,
}

// literalSize is the size of the literal a line ends with ({n}, {n+} or
// ~{n}), or 0.
func literalSize(text string) int {
	body, ok := strings.CutSuffix(text, "}")
	open := strings.LastIndexByte(body, '{')
	if !ok || open < 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSuffix(body[open+1:], "+"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// afterLiteral counts a line of literal data against the left bytes and
// returns what is left; when the literal ends inside the line, the rest of
// the line may announce the next literal.
func afterLiteral(text string, left int) int {
	switch {
	case len(text)+2 <= left:
		return left - len(text) - 2
	case len(text) < left: // the literal ends between CR and LF
		return 0
	default:
		return literalSize(text[left:])
	}
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
	selected := ""              // the client's selected mailbox
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
		x := find(exchanges, got, selected)
		if x == nil {
			if tag, rest, _ := strings.Cut(got, " "); strings.EqualFold(rest, "LOGOUT") {
				// A script cut short of the session's end: answer as servers do.
				_, _ = io.WriteString(conn, "* BYE logging out\r\n"+tag+" OK LOGOUT completed\r\n")
				continue
			}
			if selected != "" {
				return fmt.Errorf("the client sent %q with %q selected, which the script does not expect", got, selected)
			}
			return fmt.Errorf("the client sent %q, which the script does not expect", got)
		}
		x.used = true
		if err := match(x.lines[0].Text, got, tags); err != nil {
			return err
		}
		selected = selectedAfter(selected, got)
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

// find returns the first unused exchange whose command matches got and,
// for a command that acts on the selected mailbox, was recorded with the
// same mailbox selected.
func find(exchanges []*exchange, got, selected string) *exchange {
	_, rest, _ := strings.Cut(got, " ")
	name, _, _ := strings.Cut(rest, " ")
	free := anywhere[strings.ToUpper(name)]
	for _, x := range exchanges {
		if !x.used && (free || x.mailbox == selected) && match(x.lines[0].Text, got, map[string]string{}) == nil {
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

// normalize sorts the items of a FETCH or STATUS command, which clients
// may send in any order (go-imap's vary between runs).
func normalize(cmd string) string {
	upper := strings.ToUpper(cmd)
	if !strings.HasPrefix(upper, "FETCH ") && !strings.HasPrefix(upper, "UID FETCH ") && !strings.HasPrefix(upper, "STATUS ") {
		return cmd
	}
	open, end := itemList(cmd)
	if open < 0 {
		return cmd
	}
	items := splitItems(cmd[open+1 : end])
	slices.Sort(items)
	return cmd[:open+1] + strings.Join(items, " ") + cmd[end:]
}

// itemList finds the command's first parenthesized list outside quotes:
// FETCH's items, which modifiers such as (CHANGEDSINCE n) may follow, or
// STATUS's, after a mailbox name that may hold parentheses. It returns the
// indexes of the list's brackets, or -1 when there is none.
func itemList(cmd string) (open, end int) {
	depth, quoted, escaped := 0, false, false
	open = -1
	for i := 0; i < len(cmd); i++ {
		switch c := cmd[i]; {
		case escaped:
			escaped = false
		case quoted && c == '\\':
			escaped = true
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '(':
			if depth == 0 {
				open = i
			}
			depth++
		case c == ')' && depth > 0:
			if depth--; depth == 0 {
				return open, i
			}
		}
	}
	return -1, -1
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

// mapTag rewrites a server line to the client's tags: a tagged line's
// tag, and the command an ESEARCH result names, (TAG "a5"), by which the
// client matches the result to its search.
func mapTag(line string, tags map[string]string) string {
	tag, rest, ok := strings.Cut(line, " ")
	if mapped, known := tags[tag]; ok && known {
		return mapped + " " + rest
	}
	if after, found := strings.CutPrefix(line, `* ESEARCH (TAG "`); found {
		recorded, more, _ := strings.Cut(after, `"`)
		if mapped, known := tags[recorded]; known {
			return `* ESEARCH (TAG "` + mapped + `"` + more
		}
	}
	return line
}
