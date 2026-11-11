// Package compose is the desktop app's compose window (docs/specs/compose-ui.md;
// plan 0015, Phase 5): the draft's HTML through MyGo's rich text editor,
// the recipients, the attachments, and sending.
package compose

import (
	"bytes"
	"encoding/base64"
	"html"
	"image"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// PartLoader reads a mailpart: image's bytes; nil, or an error, leaves the
// image without a picture but keeps it in the draft.
type PartLoader func(src string) ([]byte, error)

// converter walks a draft's HTML into paragraphs. A quotation's depth is
// kept separately from list indentation, including while the editor splits it.
type converter struct {
	load  PartLoader
	paras []richtext.Paragraph
	cur   *richtext.Paragraph
	attrs richtext.Attributes
	quote int
	list  []richtext.ListKind
	pre   bool
}

// ToText reads a draft's HTML as the editor's text. Headings read as body
// paragraphs, as TipTap's schema has none, and images that are neither
// data: nor mailpart: are dropped, since the app never loads remote
// content.
func ToText(src string, load PartLoader) richtext.Text {
	root, err := nethtml.Parse(strings.NewReader(src))
	if err != nil {
		return richtext.NewText("")
	}
	c := &converter{load: load}
	c.walk(root)
	c.end()
	if len(c.paras) == 0 {
		return richtext.NewText("")
	}
	return richtext.TextOf(c.paras...)
}

func (c *converter) style() richtext.ParagraphStyle {
	var s richtext.ParagraphStyle
	if c.quote > 0 {
		s.Quote, s.QuoteDepth = true, c.quote
	}
	if n := len(c.list); n > 0 {
		s.List, s.Indent = c.list[n-1], n-1
	}
	s.CodeBlock = c.pre
	return s
}

func (c *converter) begin() {
	if c.cur == nil {
		c.cur = &richtext.Paragraph{Style: c.style()}
	}
}

func (c *converter) end() {
	if c.cur != nil {
		c.paras = append(c.paras, *c.cur)
		c.cur = nil
	}
}

func (c *converter) text(s string) {
	if !c.pre {
		s = collapse(s)
		if c.cur == nil && strings.TrimSpace(s) == "" {
			return
		}
	}
	c.begin()
	c.cur.Runs = append(c.cur.Runs, richtext.Run{Text: s, Attributes: c.attrs})
}

// collapse turns runs of white space into one space, as a page does.
func collapse(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !space {
				b.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

func attr(n *nethtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// allowedLink reports whether the editor keeps a link.
func allowedLink(href string) bool {
	h := strings.ToLower(strings.TrimSpace(href))
	return strings.HasPrefix(h, "http:") || strings.HasPrefix(h, "https:") || strings.HasPrefix(h, "mailto:")
}

func (c *converter) children(n *nethtml.Node) {
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		c.walk(k)
	}
}

func (c *converter) inline(n *nethtml.Node, set func(a *richtext.Attributes)) {
	saved := c.attrs
	set(&c.attrs)
	c.children(n)
	c.attrs = saved
}

func (c *converter) block(n *nethtml.Node) {
	c.end()
	c.children(n)
	c.end()
}

func (c *converter) walk(n *nethtml.Node) {
	switch n.Type {
	case nethtml.TextNode:
		c.text(n.Data)
		return
	case nethtml.ElementNode:
	default:
		c.children(n)
		return
	}
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Head, atom.Title:
	case atom.P, atom.Div, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Li:
		c.block(n)
	case atom.Pre:
		c.end()
		c.pre = true
		c.children(n)
		c.end()
		c.pre = false
	case atom.Blockquote:
		c.end()
		c.quote++
		c.children(n)
		c.end()
		c.quote--
	case atom.Ul, atom.Ol:
		c.end()
		kind := richtext.Bullets
		if n.DataAtom == atom.Ol {
			kind = richtext.Numbers
		}
		c.list = append(c.list, kind)
		c.children(n)
		c.end()
		c.list = c.list[:len(c.list)-1]
	case atom.Br:
		c.begin()
		c.end()
	case atom.B, atom.Strong:
		c.inline(n, func(a *richtext.Attributes) { a.Weight = 700 })
	case atom.I, atom.Em:
		c.inline(n, func(a *richtext.Attributes) { a.Italic = true })
	case atom.U:
		c.inline(n, func(a *richtext.Attributes) { a.Underline = true })
	case atom.S, atom.Strike, atom.Del:
		c.inline(n, func(a *richtext.Attributes) { a.Strikethrough = true })
	case atom.A:
		href := attr(n, "href")
		if !allowedLink(href) {
			c.children(n)
			return
		}
		c.inline(n, func(a *richtext.Attributes) { a.Link = href })
	case atom.Img:
		c.image(n)
	default:
		c.children(n)
	}
}

const objectChar = "￼"

func (c *converter) image(n *nethtml.Node) {
	src := attr(n, "src")
	lower := strings.ToLower(src)
	if !strings.HasPrefix(lower, "data:") && !strings.HasPrefix(lower, "mailpart:") {
		return // the app never loads remote content
	}
	a := &richtext.Attachment{Name: attr(n, "alt"), URL: src}
	if bmp := c.bitmap(src, lower); bmp != nil {
		a.Image = bmp
	}
	c.begin()
	attrs := c.attrs
	attrs.Attachment = a
	c.cur.Runs = append(c.cur.Runs, richtext.Run{Text: objectChar, Attributes: attrs})
}

// bitmap decodes an image at its real size; nil when it cannot.
func (c *converter) bitmap(src, lower string) *ui.Bitmap {
	var data []byte
	switch {
	case strings.HasPrefix(lower, "data:"):
		_, payload, ok := strings.Cut(src, ",")
		if !ok || !strings.Contains(lower[:strings.Index(lower, ",")], "base64") {
			return nil
		}
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil
		}
		data = decoded
	case c.load != nil:
		loaded, err := c.load(src)
		if err != nil {
			return nil
		}
		data = loaded
	default:
		return nil
	}
	bmp, err := uikit.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	return bmp
}

// listTag is the element of a list of kind.
func listTag(kind richtext.ListKind) string {
	if kind == richtext.Numbers {
		return "ol"
	}
	return "ul"
}

// ToHTML writes the editor's text as the draft's HTML: a quotation's depth
// as nested <blockquote type="cite">, lists as <ul> and <ol>, nested by
// their items' Indent. QuoteDepth is independent of the list nesting.
func ToHTML(t richtext.Text) string {
	var b strings.Builder
	depth := 0
	var lists []richtext.ListKind // the open lists, each with its <li> open
	closeList := func() {
		b.WriteString("</li></" + listTag(lists[len(lists)-1]) + ">")
		lists = lists[:len(lists)-1]
	}
	closeLists := func(keep int) {
		for len(lists) > keep {
			closeList()
		}
	}
	for _, p := range t.Paragraphs() {
		want := 0
		if p.Style.Quote {
			want = max(1, p.Style.QuoteDepth)
		}
		if want != depth {
			closeLists(0)
		}
		for ; depth > want; depth-- {
			b.WriteString("</blockquote>")
		}
		for ; depth < want; depth++ {
			b.WriteString(`<blockquote type="cite">`)
		}
		if p.Style.List == richtext.NoList {
			closeLists(0)
			b.WriteString("<p>")
			writeRuns(&b, p.Runs)
			b.WriteString("</p>")
			continue
		}
		level := p.Style.Indent + 1
		closeLists(level)
		if len(lists) == level && lists[level-1] != p.Style.List {
			closeList()
		}
		if len(lists) == level {
			b.WriteString("</li>")
		}
		for len(lists) < level {
			b.WriteString("<" + listTag(p.Style.List) + ">")
			lists = append(lists, p.Style.List)
			if len(lists) < level {
				b.WriteString("<li>")
			}
		}
		b.WriteString("<li>")
		writeRuns(&b, p.Runs)
	}
	closeLists(0)
	for ; depth > 0; depth-- {
		b.WriteString("</blockquote>")
	}
	return b.String()
}

func writeRuns(b *strings.Builder, runs []richtext.Run) {
	for _, r := range runs {
		writeRun(b, r)
	}
}

// hasImage reports an image anywhere in t.
func hasImage(t richtext.Text) bool {
	for _, p := range t.Paragraphs() {
		for _, r := range p.Runs {
			if r.Attributes.Attachment != nil {
				return true
			}
		}
	}
	return false
}

// unsaved reports an attachment shown from a bitmap with no source to save.
func unsaved(a *richtext.Attachment) bool {
	return a != nil && a.Image != nil && a.URL == ""
}

// hasUnsavedImage reports a picture in t that has no source yet.
func hasUnsavedImage(t richtext.Text) bool {
	for _, p := range t.Paragraphs() {
		for _, r := range p.Runs {
			if unsaved(r.Attributes.Attachment) {
				return true
			}
		}
	}
	return false
}

// maxDraftHTML bounds a draft's HTML, its pictures' data: URLs and all,
// when a paste adds pictures: a draft goes to maild in one message of at
// most 16 MiB (jsonrpc.MaxMessageSize), with its recipients and the
// message's own framing beside it.
const maxDraftHTML = 8 << 20

// maxPastedPixels is richtext's own limit on a pasted picture.
const maxPastedPixels = 1 << 25

// pastedImages lists the sources of the pictures in a clipboard's HTML
// that richtext's paste decodes to bitmaps, in order: its own test.
func pastedImages(src string) []string {
	root, err := nethtml.Parse(strings.NewReader(src))
	if err != nil {
		return nil
	}
	var out []string
	var walk func(n *nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n.Type == nethtml.ElementNode && n.DataAtom == atom.Img && decodesAsBitmap(attr(n, "src")) {
			out = append(out, attr(n, "src"))
		}
		for k := n.FirstChild; k != nil; k = k.NextSibling {
			walk(k)
		}
	}
	walk(root)
	return out
}

func decodesAsBitmap(src string) bool {
	data, ok := strings.CutPrefix(src, "data:")
	if !ok {
		return false
	}
	_, enc, ok := strings.Cut(data, ";base64,")
	if !ok {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxPastedPixels {
		return false
	}
	_, err = ui.DecodeBitmap(raw)
	return err == nil
}

// KeepPastedImages gives each picture in t that has only a bitmap the
// source it had in html, the clipboard's HTML it was pasted from: the
// editor drops that source, and a bitmap's bytes cannot be read back. A
// paste repeated in one frame brings the same pictures again. It returns
// how many have no usable source, or one that would take the draft's HTML
// past maxDraftHTML; those lose their bitmap, so that they show empty and
// are not asked about again.
func KeepPastedImages(t richtext.Text, html string) (lost int) {
	sources := pastedImages(html)
	size := len(ToHTML(t)) // without the pictures that have no source yet
	next := 0
	for _, p := range t.Paragraphs() {
		for _, r := range p.Runs {
			a := r.Attributes.Attachment
			if !unsaved(a) {
				continue
			}
			src := ""
			if len(sources) > 0 {
				src = sources[next%len(sources)]
				next++
			}
			if !strings.HasPrefix(strings.ToLower(src), "data:image/") || size+len(src) > maxDraftHTML {
				a.Image = nil
				lost++
				continue
			}
			a.URL = src
			size += len(src)
		}
	}
	return lost
}

func writeRun(b *strings.Builder, r richtext.Run) {
	a := r.Attributes
	if a.Attachment != nil {
		if keptImageURL(a.Attachment.URL) { // a picture with no source, or a remote one, is not saved
			b.WriteString(`<img src="` + html.EscapeString(a.Attachment.URL) + `" alt="` + html.EscapeString(a.Attachment.Name) + `">`)
		}
		return
	}
	var closing []string
	open := func(tag, end string) {
		b.WriteString(tag)
		closing = append(closing, end)
	}
	if a.Link != "" && allowedLink(a.Link) {
		open(`<a href="`+html.EscapeString(a.Link)+`">`, "</a>")
	}
	if a.Bold() {
		open("<strong>", "</strong>")
	}
	if a.Italic {
		open("<em>", "</em>")
	}
	if a.Underline {
		open("<u>", "</u>")
	}
	if a.Strikethrough {
		open("<s>", "</s>")
	}
	b.WriteString(html.EscapeString(r.Text))
	for i := len(closing) - 1; i >= 0; i-- {
		b.WriteString(closing[i])
	}
}

// keptImageURL reports an image source a draft keeps: a data: picture or a
// part of a message. A pasted remote image is dropped (docs/specs/compose-ui.md),
// as the app never loads, nor sends a reference to, remote content.
func keptImageURL(src string) bool {
	lower := strings.ToLower(src)
	return strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "mailpart:")
}
