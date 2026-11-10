package settings

import (
	"strings"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// escapeSignature escapes what a line of a signature may not hold as HTML.
func escapeSignature(line string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(line)
}

// signatureHTML is the stored HTML of a plain-text signature, or snippet: a
// paragraph per line (docs/specs/settings-ui.md, Signature text).
func signatureHTML(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(text)
	lines := strings.Split(text, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	var b strings.Builder
	for _, line := range lines {
		if line == "" {
			b.WriteString("<p><br></p>")
			continue
		}
		b.WriteString("<p>" + escapeSignature(line) + "</p>")
	}
	return b.String()
}

// signatureText is the plain text of a stored signature: a line per
// block, with <br> as line breaks.
func signatureText(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	context := &nethtml.Node{Type: nethtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := nethtml.ParseFragment(strings.NewReader(src), context)
	if err != nil {
		return ""
	}
	var lines []string
	inline := ""
	flush := func() {
		if inline != "" {
			lines = append(lines, inline)
		}
		inline = ""
	}
	for _, n := range nodes {
		switch {
		case n.Type == nethtml.ElementNode && n.DataAtom == atom.Br:
			inline += "\n"
		case isBlock(n):
			flush()
			lines = append(lines, blockText(n))
		default:
			inline += textOf(n)
		}
	}
	flush()
	return strings.Join(lines, "\n")
}

func isBlock(n *nethtml.Node) bool {
	if n.Type != nethtml.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.P, atom.Div, atom.Li, atom.Blockquote, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Pre:
		return true
	}
	return false
}

// blockText is a block's text with <br> as line breaks; a block that is
// only a <br> is an empty line.
func blockText(n *nethtml.Node) string {
	var out strings.Builder
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		switch {
		case k.Type == nethtml.ElementNode && k.DataAtom == atom.Br:
			out.WriteString("\n")
		case k.Type == nethtml.ElementNode:
			out.WriteString(blockText(k))
		default:
			out.WriteString(textOf(k))
		}
	}
	if out.String() == "\n" {
		return ""
	}
	return out.String()
}

// textOf is a node's text: its own, or all of its children's.
func textOf(n *nethtml.Node) string {
	if n.Type == nethtml.TextNode {
		return n.Data
	}
	var out strings.Builder
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		out.WriteString(textOf(k))
	}
	return out.String()
}
