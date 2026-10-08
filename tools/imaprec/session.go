package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/frostyard/frostmail/internal/imapx/replay"
)

// exchange is one command and everything that belongs to it, as messages:
// the client's lines, and the server's responses with their literals
// inline.
type exchange struct {
	tag  string
	msgs []msg
}

type msg struct {
	sent bool
	text string
}

// options steer convert.
type options struct {
	through string // the last command to keep; "" keeps every complete exchange
	keep    int    // messages to keep per mailbox; 0 keeps all
	key     []byte // the scrubber's secret
	note    string // the script's first comment
}

// convert turns a trace into a scrubbed replay script.
func convert(trace []replay.Line, opts options) ([]byte, error) {
	greeting, raw := replay.Split(trace)
	raw, err := cut(raw, opts.through)
	if err != nil {
		return nil, err
	}
	xs := make([]*exchange, 0, len(raw))
	for _, lines := range raw {
		x, err := assemble(lines)
		if err != nil {
			return nil, err
		}
		xs = append(xs, x)
	}
	if opts.keep > 0 {
		if xs, err = trim(xs, opts.keep); err != nil {
			return nil, err
		}
	}
	s := newScrubber(opts.key)
	var out []string
	emit := func(sent bool, text string) {
		prefix := "S: "
		if sent {
			prefix = "C: "
		}
		for line := range strings.SplitSeq(text, "\r\n") {
			out = append(out, prefix+line)
		}
	}
	for _, l := range greeting {
		text, err := s.response(l.Text)
		if err != nil {
			return nil, fmt.Errorf("greeting: %w", err)
		}
		emit(false, text)
	}
	for _, x := range xs {
		for _, m := range x.msgs {
			scrub := s.response
			if m.sent {
				scrub = s.command
			}
			text, err := scrub(m.text)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", x.tag, err)
			}
			emit(m.sent, text)
		}
	}
	body := strings.Join(out, "\n") + "\n"
	if leaks := s.leaks(body); len(leaks) > 0 {
		return nil, fmt.Errorf("replaced words remain in the script: %s", strings.Join(leaks, ", "))
	}
	var head strings.Builder
	if opts.note != "" {
		for line := range strings.SplitSeq(opts.note, "\n") {
			head.WriteString("# " + line + "\n")
		}
	}
	head.WriteString("# Scrubbed by tools/imaprec: names, addresses, subjects, bodies and\n")
	head.WriteString("# mailbox names are stable fakes; dates, structure, flags and UIDs are\n")
	head.WriteString("# as recorded.\n")
	return []byte(head.String() + body), nil
}

// cut keeps the exchanges up to and including the command tagged through,
// each of which must be complete. Without through it keeps the complete
// exchanges before the first incomplete one (a session that ended in
// IDLE).
func cut(xs [][]replay.Line, through string) ([][]replay.Line, error) {
	for i, x := range xs {
		tag, _, _ := strings.Cut(x[0].Text, " ")
		if !complete(x, tag) {
			if through == "" {
				return xs[:i], nil
			}
			return nil, fmt.Errorf("%s has no tagged completion before %s", tag, through)
		}
		if tag == through {
			return xs[:i+1], nil
		}
	}
	if through != "" {
		return nil, fmt.Errorf("the trace has no command tagged %s", through)
	}
	return xs, nil
}

// complete reports whether an exchange ends with its command's tagged
// completion.
func complete(x []replay.Line, tag string) bool {
	last := x[len(x)-1]
	if last.Sent {
		return false
	}
	t, rest, _ := strings.Cut(last.Text, " ")
	word, _, _ := strings.Cut(rest, " ")
	return t == tag && (word == "OK" || word == "NO" || word == "BAD")
}

// assemble joins an exchange's lines into messages. The client's literals
// are refused: a command that carries data (APPEND) cannot replay against
// a client that builds that data itself.
func assemble(lines []replay.Line) (*exchange, error) {
	x := &exchange{}
	x.tag, _, _ = strings.Cut(lines[0].Text, " ")
	for i := 0; i < len(lines); {
		if lines[i].Sent {
			if _, _, ok := literalAt(lines[i].Text); ok {
				return nil, fmt.Errorf("%s: the client sent a literal, which imaprec does not rewrite; cut before it", x.tag)
			}
			x.msgs = append(x.msgs, msg{sent: true, text: lines[i].Text})
			i++
			continue
		}
		text, n, err := response(lines[i:])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", x.tag, err)
		}
		x.msgs = append(x.msgs, msg{text: text})
		i += n
	}
	return x, nil
}

// response joins a server response that starts at lines[0] with its
// literals and returns it with the number of lines it used. A trace drops
// the CR of each line ending, so a literal's size is counted assuming
// CRLF line endings, and if the response then fails to parse, assuming
// bare LF.
func response(lines []replay.Line) (string, int, error) {
	if _, _, ok := literalAt(lines[0].Text); !ok {
		return lines[0].Text, 1, nil // free text need not parse
	}
	text, n, err := joinLiterals(lines, 2)
	if err == nil && parses(text) {
		return text, n, nil
	}
	if text2, n2, err2 := joinLiterals(lines, 1); err2 == nil && parses(text2) {
		return text2, n2, nil
	}
	if err == nil {
		err = fmt.Errorf("cannot parse the response %.80q", text)
	}
	return "", 0, err
}

// joinLiterals reads a response whose literals take eol bytes for each
// line ending. Literal contents come back with CRLF line endings and
// sizes to match.
func joinLiterals(lines []replay.Line, eol int) (string, int, error) {
	var b strings.Builder
	cur, used := lines[0].Text, 1
	next := func() (string, error) {
		if used == len(lines) || lines[used].Sent {
			return "", errors.New("a response ends inside a literal")
		}
		used++
		return lines[used-1].Text, nil
	}
	for {
		start, size, ok := literalAt(cur)
		if !ok {
			b.WriteString(cur)
			return b.String(), used, nil
		}
		var content strings.Builder
		var rest string
		for need := size; ; {
			if need == 0 {
				var err error
				if rest, err = next(); err != nil {
					return "", 0, err
				}
				break
			}
			l, err := next()
			if err != nil {
				return "", 0, err
			}
			if len(l)+eol <= need {
				content.WriteString(l + "\r\n")
				need -= len(l) + eol
				continue
			}
			if len(l) < need {
				return "", 0, errors.New("a literal ends inside a line break")
			}
			content.WriteString(l[:need])
			rest = l[need:]
			break
		}
		if rest != "" && rest[0] != ')' && rest[0] != ' ' {
			return "", 0, fmt.Errorf("a literal is followed by %.20q", rest)
		}
		data := loneCRs(content.String())
		marker := cur[start:]
		prefix := "{"
		if strings.HasPrefix(marker, "~") {
			prefix = "~{"
		}
		plus := ""
		if strings.HasSuffix(marker, "+}") {
			plus = "+"
		}
		fmt.Fprintf(&b, "%s%s%d%s}\r\n%s", cur[:start], prefix, len(data), plus, data)
		cur = rest
	}
}

// loneCRs turns each CR not followed by LF into a space: a partial fetch
// can end between CR and LF, and a script is a text file, which a bare CR
// would not survive.
func loneCRs(data string) string {
	b := []byte(data)
	for i, c := range b {
		if c == '\r' && (i+1 == len(b) || b[i+1] != '\n') {
			b[i] = ' '
		}
	}
	return string(b)
}

// parses reports whether a response's data reads as IMAP data.
func parses(text string) bool {
	_, rest, _ := strings.Cut(text, " ")
	_, err := parseSeq(rest)
	return err == nil
}
