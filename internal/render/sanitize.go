package render

import (
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// keptElements are rebuilt with their allowed attributes; the rest of the tree
// is dropped with its content (droppedElements) or unwrapped, keeping only
// its children (docs/design/rendering.md).
var keptElements = setOf(
	"div", "span", "p", "br", "hr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "code",
	"ul", "ol", "li", "dl", "dt", "dd", "table", "thead", "tbody", "tfoot", "tr", "td", "th", "caption",
	"col", "colgroup", "center", "font", "b", "strong", "i", "em", "u", "s", "strike", "sub", "sup",
	"small", "big", "tt", "abbr", "address", "cite", "q", "mark", "ins", "del", "a", "img", "style",
	"section", "article", "header", "footer", "main", "nav", "aside", "figure", "figcaption", "wbr",
)

var droppedElements = setOf(
	"script", "noscript", "iframe", "frame", "frameset", "noframes", "object", "embed", "applet", "param",
	"form", "input", "button", "select", "textarea", "option", "optgroup", "datalist", "svg", "math",
	"template", "link", "meta", "base", "title", "audio", "video", "source", "track", "canvas", "portal",
	"dialog", "slot", "noembed", "marquee", "picture", "map", "area",
)

var keptAttributes = setOf(
	"align", "valign", "width", "height", "border", "cellpadding", "cellspacing", "bgcolor", "color",
	"face", "size", "dir", "lang", "title", "alt", "colspan", "rowspan", "nowrap", "start", "type",
	"class", "id", "span", "scope", "headers", "reversed", "value",
)

// backgroundElements may carry a background image attribute.
var backgroundElements = setOf("table", "td", "th", "tr", "tbody", "thead", "tfoot")

var linkSchemes = setOf("http", "https", "mailto")

func setOf(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// imageResolver turns an image URL found in the message into the URL to
// show, or "" to drop it. tracker is true for images whose size or
// visibility mark them as tracking pixels.
type imageResolver func(raw string, tracker bool) string

// sanitizeHTML parses a message's HTML and returns the allowlisted fragment
// to put in the reader frame's body. The body's own presentation attributes
// survive on a wrapping div.
func sanitizeHTML(src string, resolve imageResolver) (string, error) {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return "", err
	}
	out := &html.Node{Type: html.DocumentNode}
	target := out
	if body := findElement(doc, atom.Body); body != nil {
		if attrs := cleanAttributes("body", body, resolve); len(attrs) > 0 {
			wrap := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div, Attr: attrs}
			out.AppendChild(wrap)
			target = wrap
		}
	}
	var head *html.Node
	if h := findElement(doc, atom.Head); h != nil {
		head = h
		copyChildren(head, target, resolve)
	}
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.DataAtom == atom.Html {
			for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
				if cc != head {
					clean(cc, target, resolve)
				}
			}
		}
	}
	var b strings.Builder
	for c := out.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&b, c); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

func findElement(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := findElement(c, a); f != nil {
			return f
		}
	}
	return nil
}

func copyChildren(from, to *html.Node, resolve imageResolver) {
	for c := from.FirstChild; c != nil; c = c.NextSibling {
		clean(c, to, resolve)
	}
}

// clean appends the sanitized form of n to parent.
func clean(n, parent *html.Node, resolve imageResolver) {
	switch n.Type {
	case html.TextNode:
		parent.AppendChild(&html.Node{Type: html.TextNode, Data: n.Data})
	case html.ElementNode:
		name := strings.ToLower(n.Data)
		if n.Namespace != "" || droppedElements[name] {
			return
		}
		if !keptElements[name] {
			copyChildren(n, parent, resolve)
			return
		}
		el := &html.Node{Type: html.ElementNode, Data: name, DataAtom: atom.Lookup([]byte(name))}
		if name == "style" {
			var text strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					text.WriteString(c.Data)
				}
			}
			el.AppendChild(&html.Node{Type: html.TextNode, Data: sanitizeCSS(text.String(), func(raw string) string {
				return resolve(raw, false)
			})})
			parent.AppendChild(el)
			return
		}
		el.Attr = cleanAttributes(name, n, resolve)
		parent.AppendChild(el)
		copyChildren(n, el, resolve)
	}
	// Comments, doctypes and anything else are dropped.
}

// cleanAttributes keeps an element's allowed attributes, with URLs resolved
// and inline styles sanitized. For "body" it returns only presentation
// attributes, for the wrapping div.
func cleanAttributes(name string, n *html.Node, resolve imageResolver) []html.Attribute {
	var out []html.Attribute
	add := func(key, val string) { out = append(out, html.Attribute{Key: key, Val: val}) }
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		if a.Namespace != "" {
			continue
		}
		switch {
		case key == "style":
			if s := strings.TrimSpace(sanitizeCSS(a.Val, func(raw string) string { return resolve(raw, false) })); s != "" {
				add("style", s)
			}
		case key == "href" && name == "a":
			if u := linkURL(a.Val); u != "" {
				add("href", u)
			}
		case key == "src" && name == "img":
			if u := resolve(a.Val, trackingImage(n)); u != "" {
				add("src", u)
			}
		case key == "background" && (backgroundElements[name] || name == "body"):
			if u := resolve(a.Val, false); u != "" {
				add("background", u)
			}
		case name == "body":
			if key == "bgcolor" || key == "class" || key == "id" {
				add(key, a.Val)
			}
		case keptAttributes[key]:
			add(key, a.Val)
		}
	}
	return out
}

// linkURL returns href if it is an absolute http, https or mailto URL.
func linkURL(href string) string {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil || !linkSchemes[strings.ToLower(u.Scheme)] {
		return ""
	}
	return u.String()
}

// trackingImage reports whether an img is a tracking pixel by its size (both
// dimensions at most 2px) or by being hidden. Hosts are checked by the
// resolver (trackers.go).
func trackingImage(n *html.Node) bool {
	w, h := -1, -1
	style := ""
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "width":
			w = pixels(a.Val)
		case "height":
			h = pixels(a.Val)
		case "style":
			style = strings.ToLower(strings.Join(strings.Fields(a.Val), ""))
		}
	}
	for _, decl := range strings.Split(style, ";") {
		k, v, _ := strings.Cut(decl, ":")
		v = strings.TrimSuffix(v, "!important")
		switch k {
		case "display":
			if v == "none" {
				return true
			}
		case "visibility":
			if v == "hidden" {
				return true
			}
		case "opacity":
			if f, err := strconv.ParseFloat(v, 64); err == nil && f == 0 {
				return true
			}
		case "width", "max-width":
			if p := pixels(v); p >= 0 && (w < 0 || p < w) {
				w = p
			}
		case "height", "max-height":
			if p := pixels(v); p >= 0 && (h < 0 || p < h) {
				h = p
			}
		}
	}
	return w >= 0 && h >= 0 && w <= 2 && h <= 2
}

// pixels parses "1", "1px" or "0" as a pixel count, or -1.
func pixels(v string) int {
	v = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(v)), "px")
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return -1
	}
	return n
}
