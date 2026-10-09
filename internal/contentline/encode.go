package contentline

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// EscapeText escapes a TEXT value: backslash, semicolon, comma and
// newline (RFC 5545 §3.3.11). Carriage returns are dropped.
func EscapeText(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", "")
	return r.Replace(s)
}

// JoinFields escapes each field and joins them with semicolons, for N,
// ADR and ORG.
func JoinFields(fields ...string) string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = EscapeText(f)
	}
	return strings.Join(out, ";")
}

// Encode formats the property as content lines folded at 75 octets, each
// ending in eol ("\r\n", or "\n" to match a source that uses it). Start
// and End are ignored.
func (p Prop) Encode(eol string) string {
	var b strings.Builder
	if p.Group != "" {
		b.WriteString(p.Group)
		b.WriteByte('.')
	}
	b.WriteString(p.Name)
	for _, q := range p.Params {
		b.WriteByte(';')
		b.WriteString(q.Name)
		b.WriteByte('=')
		for i, v := range q.Values {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(quoteParam(v))
		}
	}
	b.WriteByte(':')
	b.WriteString(p.Value)
	return Fold(b.String(), eol)
}

// quoteParam encodes a parameter value with RFC 6868's caret escapes and
// quotes it when it holds a colon, semicolon or comma.
func quoteParam(v string) string {
	v = strings.NewReplacer("^", "^^", "\r\n", "^n", "\n", "^n", `"`, "^'").Replace(v)
	if strings.ContainsAny(v, ":;,") {
		return `"` + v + `"`
	}
	return v
}

// Fold splits a logical line into lines of at most 75 octets, never inside
// a UTF-8 sequence; each continuation starts with a space, and every line
// ends in eol.
func Fold(s, eol string) string {
	var b strings.Builder
	limit := 75
	for len(s) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		b.WriteString(s[:cut])
		b.WriteString(eol)
		b.WriteByte(' ')
		s = s[cut:]
		limit = 74 // the leading space counts
	}
	b.WriteString(s)
	b.WriteString(eol)
	return b.String()
}

// LineEnding returns the line ending src uses: "\n" when its first line
// ends in a bare LF, else "\r\n".
func LineEnding(src []byte) string {
	i := bytes.IndexByte(src, '\n')
	if i > 0 && src[i-1] != '\r' {
		return "\n"
	}
	return "\r\n"
}

// Edit replaces src[Start:End] with Text. Start == End inserts.
type Edit struct {
	Start, End int
	Text       string
}

// Apply returns src with the edits made. Edits may come in any order but
// must not overlap; two insertions at the same place keep their order.
func Apply(src []byte, edits ...Edit) ([]byte, error) {
	sorted := slices.Clone(edits)
	slices.SortStableFunc(sorted, func(a, b Edit) int { return cmp.Compare(a.Start, b.Start) })
	var out bytes.Buffer
	at := 0
	for _, e := range sorted {
		if e.Start < at || e.End < e.Start || e.End > len(src) {
			return nil, fmt.Errorf("contentline: edit [%d,%d) overlaps another or leaves the source", e.Start, e.End)
		}
		out.Write(src[at:e.Start])
		out.WriteString(e.Text)
		at = e.End
	}
	out.Write(src[at:])
	return out.Bytes(), nil
}
