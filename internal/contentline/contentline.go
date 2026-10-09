// Package contentline reads and patches the content lines of vCard
// (RFC 6350) and iCalendar (RFC 5545) objects. Parse keeps each property's
// place in the source, so a change replaces the lines it touches and leaves
// every other byte as the server sent it (ADR-0018).
package contentline

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Prop is one property: a content line, unfolded.
type Prop struct {
	Group  string  // the vCard group, such as item1; empty when there is none
	Name   string  // uppercased
	Params []Param // in source order
	Value  string  // as written: escapes intact
	// Start and End are the folded line's bytes in the source, its line
	// ending included.
	Start, End int
}

// Param is one property parameter. A parameter may repeat; each
// occurrence is its own Param.
type Param struct {
	Name   string   // uppercased
	Values []string // unquoted, RFC 6868 caret escapes decoded
}

// Component is a BEGIN/END block: a VCARD, VCALENDAR, VEVENT, VALARM, ….
type Component struct {
	Name     string // uppercased
	Props    []Prop // the component's own properties, without BEGIN and END
	Children []*Component
	// Start and End are the block's bytes in the source, from its BEGIN
	// line to its END line's line ending.
	Start, End int
}

// ErrSyntax wraps every parse error.
var ErrSyntax = errors.New("contentline: syntax error")

// Parse reads the top-level components in src. Lines end in CRLF or LF; a
// line that starts with a space or tab continues the one before it. Empty
// lines are skipped.
func Parse(src []byte) ([]*Component, error) {
	var (
		top   []*Component
		stack []*Component
	)
	for l, err := range lines(src) {
		if err != nil {
			return nil, err
		}
		p, err := parseLine(l.text)
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: %w", ErrSyntax, l.number, err)
		}
		p.Start, p.End = l.start, l.end
		switch {
		case p.Name == "BEGIN":
			c := &Component{Name: strings.ToUpper(strings.TrimSpace(p.Value)), Start: l.start}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, c)
			} else {
				top = append(top, c)
			}
			stack = append(stack, c)
		case p.Name == "END":
			name := strings.ToUpper(strings.TrimSpace(p.Value))
			if len(stack) == 0 || stack[len(stack)-1].Name != name {
				return nil, fmt.Errorf("%w: line %d: END:%s does not close an open component", ErrSyntax, l.number, name)
			}
			stack[len(stack)-1].End = l.end
			stack = stack[:len(stack)-1]
		case len(stack) == 0:
			return nil, fmt.Errorf("%w: line %d: %s outside a component", ErrSyntax, l.number, p.Name)
		default:
			c := stack[len(stack)-1]
			c.Props = append(c.Props, p)
		}
	}
	if len(stack) > 0 {
		return nil, fmt.Errorf("%w: %s is not closed", ErrSyntax, stack[len(stack)-1].Name)
	}
	return top, nil
}

// line is one logical line: its unfolded text and its place in the source.
type line struct {
	text       string
	start, end int
	number     int // of its first physical line, from 1
}

// lines yields the logical lines of src, skipping empty ones.
func lines(src []byte) func(yield func(line, error) bool) {
	return func(yield func(line, error) bool) {
		var (
			cur     line
			buf     strings.Builder
			have    bool
			lineNum int
		)
		flush := func() bool {
			if !have {
				return true
			}
			cur.text = buf.String()
			have = false
			return yield(cur, nil)
		}
		for off := 0; off < len(src); {
			lineNum++
			end, next := lineEnd(src, off)
			phys := src[off:end]
			if len(phys) > 0 && (phys[0] == ' ' || phys[0] == '\t') {
				if !have {
					if !yield(line{}, fmt.Errorf("%w: line %d: continuation without a line to continue", ErrSyntax, lineNum)) {
						return
					}
					return
				}
				buf.Write(phys[1:])
				cur.end = next
				off = next
				continue
			}
			if !flush() {
				return
			}
			if len(bytes.TrimSpace(phys)) == 0 {
				off = next
				continue
			}
			buf.Reset()
			buf.Write(phys)
			cur = line{start: off, end: next, number: lineNum}
			have = true
			off = next
		}
		flush()
	}
}

// lineEnd returns where the physical line starting at off ends (before
// its CR LF or LF) and where the next one starts.
func lineEnd(src []byte, off int) (end, next int) {
	i := bytes.IndexByte(src[off:], '\n')
	if i < 0 {
		return len(src), len(src)
	}
	end, next = off+i, off+i+1
	if end > off && src[end-1] == '\r' {
		end--
	}
	return end, next
}

// parseLine splits "group.NAME;PARAM=a,"b";P2=c:value".
func parseLine(s string) (Prop, error) {
	var p Prop
	i := strings.IndexAny(s, ";:")
	if i < 0 {
		return p, errors.New("no colon")
	}
	name := s[:i]
	if g, n, ok := strings.Cut(name, "."); ok {
		p.Group, name = g, n
	}
	if name == "" || !isName(name) || (p.Group != "" && !isName(p.Group)) {
		return p, fmt.Errorf("bad property name %q", s[:i])
	}
	p.Name = strings.ToUpper(name)
	rest := s[i:]
	for rest != "" && rest[0] == ';' {
		var (
			param Param
			err   error
		)
		param, rest, err = parseParam(rest[1:])
		if err != nil {
			return p, fmt.Errorf("%s: %w", p.Name, err)
		}
		p.Params = append(p.Params, param)
	}
	if rest == "" || rest[0] != ':' {
		return p, fmt.Errorf("%s: no colon", p.Name)
	}
	p.Value = rest[1:]
	return p, nil
}

// parseParam reads NAME=v1,"v2" and returns what follows it. A parameter
// without "=" (vCard 2.1's bare TYPE values) becomes TYPE with that value.
func parseParam(s string) (Param, string, error) {
	end := strings.IndexAny(s, "=;:")
	if end < 0 {
		return Param{}, "", errors.New("unterminated parameter")
	}
	if s[end] != '=' {
		if !isName(s[:end]) {
			return Param{}, "", fmt.Errorf("bad parameter %q", s[:end])
		}
		return Param{Name: "TYPE", Values: []string{s[:end]}}, s[end:], nil
	}
	if !isName(s[:end]) {
		return Param{}, "", fmt.Errorf("bad parameter name %q", s[:end])
	}
	param := Param{Name: strings.ToUpper(s[:end])}
	s = s[end+1:]
	for {
		var v string
		if s != "" && s[0] == '"' {
			q := strings.IndexByte(s[1:], '"')
			if q < 0 {
				return Param{}, "", fmt.Errorf("%s: unterminated quote", param.Name)
			}
			v, s = s[1:1+q], s[2+q:]
		} else {
			e := strings.IndexAny(s, ",;:")
			if e < 0 {
				return Param{}, "", fmt.Errorf("%s: no colon", param.Name)
			}
			v, s = s[:e], s[e:]
		}
		param.Values = append(param.Values, decodeCaret(v))
		if s == "" || s[0] != ',' {
			return param, s, nil
		}
		s = s[1:]
	}
}

// isName reports whether s is a property, group or parameter name:
// letters, digits and hyphens (RFC 5545 iana-token and x-name). Some
// servers write underscores too.
func isName(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !letter && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

// decodeCaret decodes RFC 6868's ^n, ^^ and ^' in a parameter value.
func decodeCaret(v string) string {
	if !strings.Contains(v, "^") {
		return v
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] == '^' && i+1 < len(v) {
			switch v[i+1] {
			case 'n', 'N':
				b.WriteByte('\n')
				i++
				continue
			case '^':
				b.WriteByte('^')
				i++
				continue
			case '\'':
				b.WriteByte('"')
				i++
				continue
			}
		}
		b.WriteByte(v[i])
	}
	return b.String()
}

// Param returns the first value of the named parameter, or "".
func (p Prop) Param(name string) string {
	for _, q := range p.Params {
		if strings.EqualFold(q.Name, name) && len(q.Values) > 0 {
			return q.Values[0]
		}
	}
	return ""
}

// ParamValues returns every value of the named parameter across its
// occurrences, with comma-separated values split (TYPE=work,voice).
func (p Prop) ParamValues(name string) []string {
	var out []string
	for _, q := range p.Params {
		if !strings.EqualFold(q.Name, name) {
			continue
		}
		for _, v := range q.Values {
			for part := range strings.SplitSeq(v, ",") {
				if part = strings.TrimSpace(part); part != "" {
					out = append(out, part)
				}
			}
		}
	}
	return out
}

// HasParam reports whether the named parameter has value, ignoring case.
func (p Prop) HasParam(name, value string) bool {
	return slices.ContainsFunc(p.ParamValues(name), func(v string) bool { return strings.EqualFold(v, value) })
}

// Text is the value as TEXT (RFC 5545 §3.3.11, RFC 6350 §3.4): \n and \N
// are newlines, and a backslash before any other character stands for that
// character.
func (p Prop) Text() string { return unescape(p.Value) }

// Fields splits a structured value (N, ADR, ORG, GENDER) at unescaped
// semicolons and unescapes each field.
func (p Prop) Fields() []string {
	var (
		out []string
		cur strings.Builder
	)
	for i := 0; i < len(p.Value); i++ {
		c := p.Value[i]
		switch {
		case c == '\\' && i+1 < len(p.Value):
			cur.WriteByte(c)
			cur.WriteByte(p.Value[i+1])
			i++
		case c == ';':
			out = append(out, unescape(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(out, unescape(cur.String()))
}

func unescape(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' && i+1 < len(v) {
			i++
			if v[i] == 'n' || v[i] == 'N' {
				b.WriteByte('\n')
			} else {
				b.WriteByte(v[i])
			}
			continue
		}
		b.WriteByte(v[i])
	}
	return b.String()
}

// Prop returns the component's first property with the name, or nil.
func (c *Component) Prop(name string) *Prop {
	for i := range c.Props {
		if c.Props[i].Name == name {
			return &c.Props[i]
		}
	}
	return nil
}

// PropsNamed returns the component's properties with the name, in order.
func (c *Component) PropsNamed(name string) []Prop {
	var out []Prop
	for _, p := range c.Props {
		if p.Name == name {
			out = append(out, p)
		}
	}
	return out
}

// ChildrenNamed returns the component's children with the name, in order.
func (c *Component) ChildrenNamed(name string) []*Component {
	var out []*Component
	for _, ch := range c.Children {
		if ch.Name == name {
			out = append(out, ch)
		}
	}
	return out
}
