package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// IMAP data as imaprec reads it: a command's or response's arguments are
// a sequence of nodes. Each node keeps the separator before it, so data
// prints back exactly as it was read, apart from the strings imaprec
// replaces and the literal sizes that follow them.

type kind int

const (
	atom    kind = iota // atoms, numbers, NIL, flags, sections like BODY[1]<0>
	quoted              // "a quoted string"
	literal             // {n} or ~{n} followed by n bytes
	list                // (a parenthesized list)
)

type node struct {
	kind  kind
	pre   string  // the separator before the node
	text  string  // an atom, or a string's value
	items []*node // a list's items
	end   string  // a list's separator before ")"
	plus  bool    // a literal written {n+}
	bin   bool    // a literal8, ~{n}
}

func (n *node) isString() bool { return n.kind == quoted || n.kind == literal }

// upper is an atom's or string's text in upper case.
func (n *node) upper() string { return strings.ToUpper(n.text) }

func (n *node) write(b *strings.Builder) {
	b.WriteString(n.pre)
	switch n.kind {
	case atom:
		b.WriteString(n.text)
	case quoted:
		b.WriteByte('"')
		for i := 0; i < len(n.text); i++ {
			if c := n.text[i]; c == '"' || c == '\\' {
				b.WriteByte('\\')
			}
			b.WriteByte(n.text[i])
		}
		b.WriteByte('"')
	case literal:
		if n.bin {
			b.WriteByte('~')
		}
		plus := ""
		if n.plus {
			plus = "+"
		}
		fmt.Fprintf(b, "{%d%s}\r\n%s", len(n.text), plus, n.text)
	case list:
		b.WriteByte('(')
		for _, item := range n.items {
			item.write(b)
		}
		b.WriteString(n.end)
		b.WriteByte(')')
	}
}

// format prints nodes back as IMAP data.
func format(nodes []*node) string {
	var b strings.Builder
	for _, n := range nodes {
		n.write(&b)
	}
	return b.String()
}

// parseSeq reads s as a sequence of nodes; literals are inline, their
// size line ending in CRLF.
func parseSeq(s string) ([]*node, error) {
	p := &parser{s: s}
	nodes, end, err := p.seq(false)
	if err != nil {
		return nil, err
	}
	if end != "" {
		nodes = append(nodes, &node{kind: atom, pre: end})
	}
	return nodes, nil
}

type parser struct {
	s string
	i int
}

// seq reads nodes up to the end of the input or, in a list, its ")". It
// returns the separator left before that end.
func (p *parser) seq(inList bool) ([]*node, string, error) {
	var out []*node
	for {
		start := p.i
		for p.i < len(p.s) && p.s[p.i] == ' ' {
			p.i++
		}
		pre := p.s[start:p.i]
		if p.i == len(p.s) {
			if inList {
				return nil, "", errors.New("a list is not closed")
			}
			return out, pre, nil
		}
		if p.s[p.i] == ')' {
			if !inList {
				return nil, "", fmt.Errorf("unexpected ) at %d", p.i)
			}
			p.i++
			return out, pre, nil
		}
		n, err := p.node()
		if err != nil {
			return nil, "", err
		}
		n.pre = pre
		out = append(out, n)
	}
}

func (p *parser) node() (*node, error) {
	switch c := p.s[p.i]; {
	case c == '(':
		p.i++
		items, end, err := p.seq(true)
		if err != nil {
			return nil, err
		}
		return &node{kind: list, items: items, end: end}, nil
	case c == '"':
		return p.quoted()
	case c == '{':
		return p.literal(false)
	case c == '~' && p.i+1 < len(p.s) && p.s[p.i+1] == '{':
		p.i++
		return p.literal(true)
	default:
		return p.atom()
	}
}

func (p *parser) quoted() (*node, error) {
	var b strings.Builder
	for p.i++; p.i < len(p.s); p.i++ {
		switch c := p.s[p.i]; c {
		case '\\':
			p.i++
			if p.i == len(p.s) {
				return nil, errors.New("a quoted string ends in a backslash")
			}
			b.WriteByte(p.s[p.i])
		case '"':
			p.i++
			return &node{kind: quoted, text: b.String()}, nil
		default:
			b.WriteByte(c)
		}
	}
	return nil, errors.New("a quoted string is not closed")
}

func (p *parser) literal(bin bool) (*node, error) {
	brace := strings.IndexByte(p.s[p.i:], '}')
	if brace < 0 {
		return nil, errors.New("a literal's size is not closed")
	}
	spec := p.s[p.i+1 : p.i+brace]
	plus := strings.HasSuffix(spec, "+")
	n, err := strconv.Atoi(strings.TrimSuffix(spec, "+"))
	if err != nil || n < 0 {
		return nil, fmt.Errorf("bad literal size %q", spec)
	}
	p.i += brace + 1
	if !strings.HasPrefix(p.s[p.i:], "\r\n") || len(p.s)-p.i-2 < n {
		return nil, fmt.Errorf("a literal of %d bytes is cut short", n)
	}
	p.i += 2
	text := p.s[p.i : p.i+n]
	p.i += n
	return &node{kind: literal, text: text, plus: plus, bin: bin}, nil
}

// atom reads up to a space or parenthesis; a section in brackets, such as
// BODY[HEADER.FIELDS (A B)], belongs to the atom with its spaces.
func (p *parser) atom() (*node, error) {
	start := p.i
	for p.i < len(p.s) {
		switch c := p.s[p.i]; c {
		case ' ', '(', ')', '\r', '\n':
			return &node{kind: atom, text: p.s[start:p.i]}, nil
		case '[':
			bracket := strings.IndexByte(p.s[p.i:], ']')
			if bracket < 0 {
				return nil, fmt.Errorf("a section is not closed in %q", p.s[start:])
			}
			p.i += bracket + 1
		default:
			p.i++
		}
	}
	return &node{kind: atom, text: p.s[start:]}, nil
}

// literalAt finds the literal size a line ends with: "{n}", "{n+}" or
// "~{n}". start is where the marker begins.
func literalAt(line string) (start, size int, ok bool) {
	if !strings.HasSuffix(line, "}") {
		return 0, 0, false
	}
	open := strings.LastIndexByte(line, '{')
	if open < 0 {
		return 0, 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(line[open+1:len(line)-1], "+"))
	if err != nil || n < 0 {
		return 0, 0, false
	}
	if open > 0 && line[open-1] == '~' {
		open--
	}
	if open > 0 && line[open-1] != ' ' && line[open-1] != '(' {
		return 0, 0, false
	}
	return open, n, true
}
