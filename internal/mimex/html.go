package mimex

import (
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// HTMLToText renders an HTML message part as plain text for previews and the
// search index. Task T-0007 implements it; the stub only pins the x/net
// dependency it uses.
func HTMLToText(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	skip := 0
	var b strings.Builder
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.TextToken:
			if skip > 0 {
				continue
			}
			writeText(&b, string(z.Text()))
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			n := string(name)
			if isSkipped(n) {
				skip++
			} else if skip == 0 {
				startTag(&b, n)
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			n := string(name)
			if isSkipped(n) {
				if skip > 0 {
					skip--
				}
			} else if skip == 0 {
				endTag(&b, n)
			}
		}
	}
	return normalizeLines(b.String())
}

// writeText appends a token's entity-decoded text, collapsing whitespace runs
// to single spaces and dropping leading spaces after a break.
func writeText(b *strings.Builder, text string) {
	t := collapseSpaces(text)
	if t == "" {
		return
	}
	out := b.String()
	if out == "" || strings.HasSuffix(out, "\n") {
		t = strings.TrimLeft(t, " ")
	}
	b.WriteString(t)
}

// startTag applies the block-level effect of a start (or self-closing) tag.
func startTag(b *strings.Builder, name string) {
	switch {
	case name == "br":
		b.WriteString("\n")
	case isParagraph(name):
		paraBreak(b)
	case isLineBreak(name):
		lineBreak(b)
		if name == "li" {
			b.WriteString("- ")
		}
	}
}

// endTag applies the block-level effect of an end tag.
func endTag(b *strings.Builder, name string) {
	switch {
	case isParagraph(name):
		paraBreak(b)
	case isLineBreak(name):
		lineBreak(b)
	case name == "td" || name == "th":
		b.WriteString(" ")
	}
}

// paraBreak pads the output so it ends with a blank line.
func paraBreak(b *strings.Builder) {
	if b.Len() == 0 {
		return
	}
	for !strings.HasSuffix(b.String(), "\n\n") {
		b.WriteByte('\n')
	}
}

// lineBreak ends the current output line if it is not already ended.
func lineBreak(b *strings.Builder) {
	if b.Len() == 0 || strings.HasSuffix(b.String(), "\n") {
		return
	}
	b.WriteByte('\n')
}

// isSkipped reports whether name is an element whose contents never produce text.
func isSkipped(name string) bool {
	switch name {
	case "script", "style", "head", "title", "template", "noscript":
		return true
	}
	return false
}

// isParagraph reports whether name is a block element separated by a blank line.
func isParagraph(name string) bool {
	switch name {
	case "p", "blockquote", "table", "ul", "ol", "pre", "hr":
		return true
	}
	return len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6'
}

// isLineBreak reports whether name is a block element separated by one newline.
func isLineBreak(name string) bool {
	switch name {
	case "div", "li", "tr", "section", "article", "header", "footer", "dt", "dd":
		return true
	}
	return false
}

// collapseSpaces replaces each run of whitespace with a single space, keeping
// the edges so adjacent tokens do not glue together.
func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inRun := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			inRun = true
			continue
		}
		if inRun {
			b.WriteByte(' ')
			inRun = false
		}
		b.WriteRune(r)
	}
	if inRun {
		b.WriteByte(' ')
	}
	return b.String()
}

// normalizeLines trims every line and squeezes runs of blank lines to one.
func normalizeLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(collapseSpaces(line))
	}
	out := strings.Join(lines, "\n")
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}
