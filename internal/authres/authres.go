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
// method[/version]=result. Both are lowercased.
func parsePiece(piece string) (method, result string, ok bool) {
	piece = strings.TrimSpace(piece)
	name, rest, found := strings.Cut(piece, "=")
	if !found {
		return "", "", false
	}
	name = strings.TrimSpace(name)
	method, version, hasVersion := strings.Cut(name, "/")
	if method == "" || strings.ContainsAny(name, " \t\"") || (hasVersion && version == "") {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)
	end := strings.IndexAny(rest, " \t")
	if end >= 0 {
		rest = rest[:end]
	}
	if rest == "" || strings.Contains(rest, "\"") {
		return "", "", false
	}
	return strings.ToLower(method), strings.ToLower(rest), true
}
