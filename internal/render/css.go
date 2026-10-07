package render

import (
	"strconv"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

// blockedFunctions name CSS functions that load resources or run code in some
// engine; a call is replaced with none. url() is rewritten instead.
var blockedFunctions = map[string]bool{
	"expression": true, "image": true, "image-set": true, "-webkit-image-set": true,
	"cross-fade": true, "-webkit-cross-fade": true, "element": true, "-moz-element": true,
	"paint": true, "src": true,
}

// sanitizeCSS returns src without constructs that load resources or run code:
// @import, @font-face, @namespace and @charset rules, blocked functions, and
// the behavior and -moz-binding properties. Every url() goes through resolve,
// which returns the URL to use or "" to drop it (the url() becomes none).
// Names are compared after resolving CSS escapes, and "<" is escaped in the
// output so the text cannot close a <style> element.
func sanitizeCSS(src string, resolve func(raw string) string) string {
	l := css.NewLexer(parse.NewInputString(src))
	var b strings.Builder
	for {
		tt, data := l.Next()
		switch tt {
		case css.ErrorToken:
			return strings.ReplaceAll(b.String(), "<", `\3c `)
		case css.CommentToken, css.CDOToken, css.CDCToken:
		case css.AtKeywordToken:
			switch strings.ToLower(cssUnescape(string(data[1:]))) {
			case "import", "namespace", "charset":
				skipStatement(l)
			case "font-face":
				skipStatement(l)
			default:
				b.Write(data)
			}
		case css.URLToken:
			writeURL(&b, resolve(urlTokenValue(data)))
		case css.BadURLToken:
			b.WriteString("none")
		case css.FunctionToken:
			name := strings.ToLower(cssUnescape(string(data[:len(data)-1])))
			switch {
			case name == "url":
				writeURL(&b, resolve(functionArgument(l)))
			case blockedFunctions[name]:
				skipFunction(l)
				b.WriteString("none")
			default:
				b.Write(data)
			}
		case css.IdentToken:
			switch strings.ToLower(cssUnescape(string(data))) {
			case "behavior", "-moz-binding":
				b.WriteString("x-blocked")
			default:
				b.Write(data)
			}
		default:
			b.Write(data)
		}
	}
}

// skipStatement skips an at-rule's prelude and its block or semicolon.
func skipStatement(l *css.Lexer) {
	depth := 0
	for {
		tt, _ := l.Next()
		switch tt {
		case css.ErrorToken:
			return
		case css.LeftBraceToken:
			depth++
		case css.RightBraceToken:
			depth--
			if depth <= 0 {
				return
			}
		case css.SemicolonToken:
			if depth == 0 {
				return
			}
		}
	}
}

// skipFunction skips to the parenthesis that closes the current function.
func skipFunction(l *css.Lexer) {
	depth := 1
	for depth > 0 {
		tt, _ := l.Next()
		switch tt {
		case css.ErrorToken:
			return
		case css.FunctionToken, css.LeftParenthesisToken:
			depth++
		case css.RightParenthesisToken:
			depth--
		}
	}
}

// functionArgument reads a url( function's argument up to its closing
// parenthesis: a string's value, or the raw text of other tokens.
func functionArgument(l *css.Lexer) string {
	var raw strings.Builder
	depth := 1
	for {
		tt, data := l.Next()
		switch tt {
		case css.ErrorToken:
			return strings.TrimSpace(raw.String())
		case css.FunctionToken, css.LeftParenthesisToken:
			depth++
		case css.RightParenthesisToken:
			depth--
			if depth == 0 {
				return strings.TrimSpace(raw.String())
			}
		case css.StringToken, css.BadStringToken:
			raw.WriteString(cssUnescape(stringValue(data)))
			continue
		case css.WhitespaceToken, css.CommentToken:
			continue
		}
		raw.WriteString(cssUnescape(string(data)))
	}
}

// stringValue returns a string token's content without its quotes; an
// unterminated string at the end of input has only the opening quote.
func stringValue(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	s := string(data[1:])
	if len(s) > 0 && s[len(s)-1] == data[0] {
		s = s[:len(s)-1]
	}
	return s
}

// urlTokenValue extracts the URL of an unquoted url(...) token.
func urlTokenValue(data []byte) string {
	s := string(data)
	open := strings.IndexByte(s, '(')
	end := strings.LastIndexByte(s, ')')
	if open < 0 || end < open {
		return ""
	}
	inner := strings.TrimSpace(s[open+1 : end])
	if len(inner) >= 2 && (inner[0] == '"' || inner[0] == '\'') && inner[len(inner)-1] == inner[0] {
		inner = inner[1 : len(inner)-1]
	}
	return cssUnescape(inner)
}

// writeURL writes url("u") with u quoted for CSS, or none for "".
func writeURL(b *strings.Builder, u string) {
	if u == "" {
		b.WriteString("none")
		return
	}
	b.WriteString(`url("`)
	for _, r := range u {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			b.WriteString(`\` + strconv.FormatInt(int64(r), 16) + " ")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteString(`")`)
}

// cssUnescape resolves CSS escapes: a backslash and 1-6 hex digits with one
// optional following whitespace, or a backslash and any other character.
func cssUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		j := i + 1
		for j < len(s) && j-i <= 6 && isHex(s[j]) {
			j++
		}
		if j == i+1 {
			if s[j] != '\n' {
				b.WriteByte(s[j])
			}
			i = j
			continue
		}
		n, _ := strconv.ParseUint(s[i+1:j], 16, 32)
		if n == 0 || n > 0x10ffff || (n >= 0xd800 && n <= 0xdfff) {
			n = 0xfffd
		}
		b.WriteRune(rune(n))
		if j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n') {
			j++
		}
		i = j - 1
	}
	return b.String()
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
