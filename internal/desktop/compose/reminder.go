package compose

import (
	"regexp"
	"strings"

	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var blocks = map[atom.Atom]bool{atom.P: true, atom.Div: true, atom.Li: true, atom.Blockquote: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true, atom.Pre: true}

func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

func textOf(n *nethtml.Node, b *strings.Builder) {
	switch n.Type {
	case nethtml.TextNode:
		b.WriteString(n.Data)
		return
	case nethtml.ElementNode:
		if n.DataAtom == atom.Br {
			b.WriteByte('\n')
			return
		}
	}
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		textOf(k, b)
	}
	if n.Type == nethtml.ElementNode && blocks[n.DataAtom] {
		b.WriteByte('\n')
	}
}

func body(src string) *nethtml.Node {
	root, err := nethtml.Parse(strings.NewReader(src))
	if err != nil {
		return &nethtml.Node{Type: nethtml.DocumentNode}
	}
	var find func(n *nethtml.Node) *nethtml.Node
	find = func(n *nethtml.Node) *nethtml.Node {
		if n.DataAtom == atom.Body {
			return n
		}
		for k := n.FirstChild; k != nil; k = k.NextSibling {
			if f := find(k); f != nil {
				return f
			}
		}
		return nil
	}
	if b := find(root); b != nil {
		return b
	}
	return root
}

func nodeText(n *nethtml.Node) string {
	var b strings.Builder
	textOf(n, &b)
	return squash(b.String())
}

// HTMLText is the text of HTML: blocks and line breaks end a line, then
// white space is squashed.
func HTMLText(src string) string { return nodeText(body(src)) }

func removeAll(n *nethtml.Node, tag atom.Atom) {
	for k := n.FirstChild; k != nil; {
		next := k.NextSibling
		if k.DataAtom == tag {
			n.RemoveChild(k)
		} else {
			removeAll(k, tag)
		}
		k = next
	}
}

// OwnText is the text the user wrote: no quotes, no attribution line, no
// signature (docs/specs/compose-ui.md, Attachment reminder).
func OwnText(src, signatureHTML string) string {
	b := body(src)
	for q := b.FirstChild; q != nil; q = q.NextSibling {
		if q.DataAtom != atom.Blockquote {
			continue
		}
		before := q.PrevSibling
		for before != nil && before.Type == nethtml.TextNode && strings.TrimSpace(before.Data) == "" {
			before = before.PrevSibling
		}
		if before != nil && before.DataAtom == atom.P {
			if line := nodeText(before); strings.HasSuffix(line, "wrote:") || line == "Begin forwarded message:" {
				b.RemoveChild(before)
			}
		}
	}
	removeAll(b, atom.Blockquote)
	text := nodeText(b)
	if sig := HTMLText(signatureHTML); sig != "" {
		if at := strings.LastIndex(text, sig); at >= 0 {
			text = squash(text[:at] + " " + text[at+len(sig):])
		}
	}
	return text
}

// Go's regexp has no lookaround, so the word's edges are matched as
// characters that are neither letters nor digits.
var attachmentWord = regexp.MustCompile(`(?i)(?:^|[^\pL\pN])(?:attach|attaches|attached|attaching|attachments?|enclosed|enclosures?)(?:$|[^\pL\pN])`)

// MentionsAttachment reports whether text holds an attachment word as a
// whole word.
func MentionsAttachment(text string) bool { return attachmentWord.MatchString(text) }

func imageOutsideQuotes(n *nethtml.Node) bool {
	if n.DataAtom == atom.Blockquote {
		return false
	}
	if n.DataAtom == atom.Img {
		return true
	}
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		if imageOutsideQuotes(k) {
			return true
		}
	}
	return false
}

// HasAttachment reports a chip, or an image outside every quote.
func HasAttachment(src string, attachments int) bool {
	return attachments > 0 || imageOutsideQuotes(body(src))
}

// NeedsReminder reports whether the user's own text mentions an attachment
// and none is attached.
func NeedsReminder(src, signatureHTML string, attachments int) bool {
	return MentionsAttachment(OwnText(src, signatureHTML)) && !HasAttachment(src, attachments)
}
