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

// layerPrefix starts the name of every cascade layer a message declares, so
// that none is, or reopens, a layer of the page around it: the desktop
// app's print guard is the page's first layer (internal/desktop/mail,
// PrintGuard), and a rule a message put into it would beat the guard's own.
const layerPrefix = "message-"

// sanitizeCSS returns src without constructs that load resources or run code:
// @import, @font-face, @namespace and @charset rules, blocked functions, and
// the behavior, -moz-binding and color-scheme properties. It also drops @page
// rules, whose margin boxes would print text outside the page's content, and
// puts layerPrefix before each name in an @layer rule. Every url() goes through
// resolve, which returns the URL to use or "" to drop it (the url() becomes
// none). Names are compared after resolving CSS escapes, removals leave a
// space so neighboring tokens cannot join, and "<" is escaped in the output
// so the text cannot close a <style> element.
//
// A message's CSS also sees a light page, whatever the appearance of the
// desktop. The reader page is white in both (pageStyle), but the engine
// reports prefers-color-scheme: dark when the desktop is dark, so a
// newsletter's dark @media rule would draw a mostly black area on that white
// page (frostyard/frostmail#269). So a prefers-color-scheme feature in an
// @media prelude answers light: a value of light becomes grid:0 (always true,
// on a screen and on paper) and any other value becomes grid:1 (always
// false), which keeps not, and and comma lists correct. The color-scheme
// property is renamed x-blocked, an unknown property the browser drops, so the
// page keeps color-scheme: light for light-dark() and the system colors.
func sanitizeCSS(src string, resolve func(raw string) string) string {
	r := &tokens{l: css.NewLexer(parse.NewInputString(src))}
	var b strings.Builder
	// layer is true from an @layer to the end of its prelude in the
	// output, the next {, ; or } written. Every word written in between
	// starts with layerPrefix, whatever wrote it: a name, a replacement,
	// or a name a removal brought in.
	layer := false
	// media is true from an @media keyword to the end of its prelude.
	// property is true where a declaration's property name may start: at
	// the start of the input, or after a {, a ; or a } (a nested rule can
	// come before a declaration), with only whitespace and comments since.
	media, property := false, true
	word := func() {
		if layer {
			b.WriteString(layerPrefix)
		}
	}
	for {
		tt, data := r.Next()
		decl := property
		switch tt {
		case css.WhitespaceToken, css.CommentToken:
			// These alone may stand before a property name.
		default:
			property = false
		}
		switch tt {
		case css.ErrorToken:
			return strings.ReplaceAll(b.String(), "<", `\3c `)
		case css.CommentToken, css.CDOToken, css.CDCToken:
			// A removal leaves a space, so its neighbors cannot join into a
			// new token ("@" + "import").
			b.WriteByte(' ')
		case css.AtKeywordToken:
			switch strings.ToLower(cssUnescape(string(data[1:]))) {
			case "import", "namespace", "charset", "font-face", "page":
				skipStatement(r)
				b.WriteByte(' ')
			case "layer":
				b.Write(data)
				layer = true
			case "media":
				b.Write(data)
				media = true
			default:
				b.Write(data)
			}
		case css.URLToken:
			word()
			writeURL(&b, resolve(urlTokenValue(data)))
		case css.BadURLToken:
			word()
			b.WriteString("none")
		case css.FunctionToken:
			word()
			name := strings.ToLower(cssUnescape(string(data[:len(data)-1])))
			switch {
			case name == "url":
				writeURL(&b, resolve(functionArgument(r)))
			case blockedFunctions[name]:
				skipFunction(r)
				b.WriteString("none")
			default:
				b.Write(data)
			}
		case css.IdentToken:
			name := strings.ToLower(cssUnescape(string(data)))
			if media && name == "prefers-color-scheme" {
				if light, ok := lightFeature(r); ok {
					// grid is the media feature that is always true on a
					// screen and on paper, and never true for 1.
					b.WriteString(light)
					continue
				}
			}
			word()
			switch {
			case name == "color-scheme" && decl && followedByColon(r):
				// The page's own color-scheme: light stays the page's.
				b.WriteString("x-blocked")
			case name == "behavior" || name == "-moz-binding":
				b.WriteString("x-blocked")
			default:
				b.Write(data)
			}
		case css.LeftBraceToken, css.SemicolonToken, css.RightBraceToken:
			layer = false
			media = false
			property = true
			b.Write(data)
		default:
			b.Write(data)
		}
	}
}

// lexer is what reads tokens: a css.Lexer, or tokens over one.
type lexer interface {
	Next() (css.TokenType, []byte)
}

// tokens reads a CSS lexer one token at a time, with a small stack of tokens
// to read again: sanitizeCSS looks ahead at the colon and value of a media
// feature and of a declaration.
type tokens struct {
	l    *css.Lexer
	back []cssToken
}

type cssToken struct {
	tt   css.TokenType
	data []byte
}

// Next returns the next token: one read again first, else the lexer's.
func (r *tokens) Next() (css.TokenType, []byte) {
	if n := len(r.back); n > 0 {
		t := r.back[n-1]
		r.back = r.back[:n-1]
		return t.tt, t.data
	}
	return r.l.Next()
}

// unread makes tok be the next token read.
func (r *tokens) unread(tok cssToken) {
	r.back = append(r.back, tok)
}

// unreadAll makes the tokens in seen be read again, in their order.
func unreadAll(r *tokens, seen []cssToken) {
	for i := len(seen) - 1; i >= 0; i-- {
		r.unread(seen[i])
	}
}

// followedByColon reports whether a colon comes next, with only whitespace
// and comments before it. Every token it reads is read again, so the caller
// writes them as they came.
func followedByColon(r *tokens) bool {
	var seen []cssToken
	colon := false
	for {
		tt, data := r.Next()
		seen = append(seen, cssToken{tt, data})
		if tt == css.WhitespaceToken || tt == css.CommentToken {
			continue
		}
		colon = tt == css.ColonToken
		break
	}
	unreadAll(r, seen)
	return colon
}

// lightFeature reads the rest of a media feature whose name was just read: a
// colon and its value, each with only whitespace and comments before it. It
// returns the feature to write in its place and true; the page is always
// light, so light is always true (grid:0) and any other value is never true
// (grid:1). It returns false, with every token it read readable again, for the
// boolean form with no value and for anything else the caller must keep.
func lightFeature(r *tokens) (string, bool) {
	var seen []cssToken
	for {
		tt, data := r.Next()
		seen = append(seen, cssToken{tt, data})
		if tt == css.WhitespaceToken || tt == css.CommentToken {
			continue
		}
		if tt != css.ColonToken {
			unreadAll(r, seen)
			return "", false
		}
		break
	}
	// The value, again with only whitespace and comments before it.
	for {
		tt, data := r.Next()
		seen = append(seen, cssToken{tt, data})
		if tt == css.WhitespaceToken || tt == css.CommentToken {
			continue
		}
		if tt != css.IdentToken {
			unreadAll(r, seen)
			return "", false
		}
		if strings.EqualFold(cssUnescape(string(data)), "light") {
			return "grid:0", true
		}
		return "grid:1", true
	}
}

// skipStatement skips an at-rule's prelude and its block or semicolon.
func skipStatement(r lexer) {
	depth := 0
	for {
		tt, _ := r.Next()
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
func skipFunction(r lexer) {
	depth := 1
	for depth > 0 {
		tt, _ := r.Next()
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
func functionArgument(r lexer) string {
	var raw strings.Builder
	depth := 1
	for {
		tt, data := r.Next()
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
