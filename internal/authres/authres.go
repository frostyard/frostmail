// Package authres works out a sender check from an Authentication-Results
// header (RFC 8601), as design/rendering.md describes (ADR-0050). It reads
// the header text only: it verifies nothing and makes no request.
package authres

import "strings"

// Verdict is the outcome of a sender check.
type Verdict string

// The verdicts. None means the header does not say.
const (
	None Verdict = ""
	Pass Verdict = "pass"
	Fail Verdict = "fail"
)

// SenderCheck returns the verdict for the value of the first
// Authentication-Results header. DMARC decides; DKIM decides only when the
// header has no DMARC result.
func SenderCheck(header string) Verdict {
	var dmarc string
	var haveDMARC, dkimPass, dkimFail bool
	pieces := splitPieces(stripComments(unfold(header)))
	for i, piece := range pieces {
		if i == 0 { // the authserv-id
			continue
		}
		method, result, ok := parsePiece(piece)
		if !ok {
			continue
		}
		switch method {
		case "dmarc":
			if !haveDMARC {
				dmarc, haveDMARC = result, true
			}
		case "dkim":
			dkimPass = dkimPass || result == "pass"
			dkimFail = dkimFail || result == "fail"
		}
	}
	switch {
	case haveDMARC && dmarc == "pass":
		return Pass
	case haveDMARC && dmarc == "fail":
		return Fail
	case haveDMARC:
		return None
	case dkimPass:
		return Pass
	case dkimFail:
		return Fail
	}
	return None
}

// unfold joins folded lines.
func unfold(s string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(s)
}

// stripComments removes parenthesized comments (nested) outside quoted
// strings. Inside a comment, a backslash quotes the next character.
func stripComments(s string) string {
	var b strings.Builder
	depth := 0
	quoted := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case depth > 0:
			switch c {
			case '\\':
				i++
			case '(':
				depth++
			case ')':
				depth--
			}
		case quoted:
			b.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				i++
				b.WriteByte(s[i])
			} else if c == '"' {
				quoted = false
			}
		case c == '"':
			quoted = true
			b.WriteByte(c)
		case c == '(':
			depth = 1
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// splitPieces splits on ";" outside quoted strings.
func splitPieces(s string) []string {
	var out []string
	quoted := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case quoted && c == '\\':
			i++
		case c == '"':
			quoted = !quoted
		case c == ';' && !quoted:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// parsePiece reads the method and the result at the start of a piece:
// method [ "/" version ] "=" result, with white space allowed around "/" and
// "=" (comments are already removed). The method and the result are keywords
// of letters, digits and "-", the version is digits. Both are lowercased.
// Any other form does not parse.
func parsePiece(piece string) (method, result string, ok bool) {
	s := strings.TrimSpace(piece)
	method, s = keyword(s)
	if method == "" {
		return "", "", false
	}
	s = strings.TrimLeft(s, " \t")
	if rest, found := strings.CutPrefix(s, "/"); found {
		rest = strings.TrimLeft(rest, " \t")
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		if end == 0 {
			return "", "", false
		}
		s = strings.TrimLeft(rest[end:], " \t")
	}
	s, found := strings.CutPrefix(s, "=")
	if !found {
		return "", "", false
	}
	result, s = keyword(strings.TrimLeft(s, " \t"))
	if result == "" || (s != "" && s[0] != ' ' && s[0] != '\t') {
		return "", "", false
	}
	return strings.ToLower(method), strings.ToLower(result), true
}

// keyword splits a leading run of letters, digits and "-" from s.
func keyword(s string) (word, rest string) {
	end := 0
	for end < len(s) {
		c := s[end]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' {
			end++
			continue
		}
		break
	}
	return s[:end], s[end:]
}
