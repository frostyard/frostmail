package uikit_test

import (
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

func TestDocumentCarriesThePolicyFirst(t *testing.T) {
	doc := uikit.Document(`<p>hi</p>`, "", "")
	policy := strings.Index(doc, `http-equiv="Content-Security-Policy"`)
	body := strings.Index(doc, `<body>`)
	if policy < 0 || body < policy {
		t.Fatalf("policy at %d, body at %d: %s", policy, body, doc)
	}
	for _, want := range []string{"default-src 'none'", "img-src data:", "base-uri 'none'", "form-action 'none'", `<base target="_blank">`} {
		if !strings.Contains(doc, want) {
			t.Errorf("no %q in %s", want, doc)
		}
	}
	for _, never := range []string{"script-src", "connect-src", "unsafe-eval", "http:", "https:"} {
		if strings.Contains(uikit.PageCSP, never) {
			t.Errorf("the policy allows %q", never)
		}
	}
}

func TestPrintHeaderIsPrintOnly(t *testing.T) {
	doc := uikit.Document("body", "<div>From: Ann</div>", "")
	if !strings.Contains(doc, `<body><div class="print-only"><div>From: Ann</div></div><div>body</div></body>`) {
		t.Errorf("print header missing: %s", doc)
	}
	if !strings.Contains(doc, `@media screen{.print-only{display:none}body>.print-only:first-child+div{display:contents}}`) {
		t.Errorf("the print header shows on screen, or the message's holder lays out: %s", doc)
	}
	if doc := uikit.Document("body", "", ""); !strings.Contains(doc, `<body>body</body>`) {
		t.Errorf("without a header: %s", doc)
	}
}

// M46: with a print header, the body holds the header and one element
// that the app writes, which holds all of the message. A rule of the
// print guard on that element reaches the whole message, and no style
// attribute of the message reaches it, as one on an element of the
// message's own would beat the guard; nor does a message's display:
// contents take its own elements out of it.
func TestDocumentHoldsTheMessageInOneElement(t *testing.T) {
	const message = `<div style="display:contents"><div style="position:fixed!important;z-index:2147483647!important;top:0;background:#fff">From: Bank</div></div>` +
		`<div class="print-only">From: Bank</div><table><tr><td>x</td></tr></table><p>y`
	doc, err := html.Parse(strings.NewReader(uikit.Document(message, "<div>From: Ann</div>", "")))
	if err != nil {
		t.Fatal(err)
	}
	var body *html.Node
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.DataAtom == atom.Body {
			body = n
		}
	}
	if body == nil {
		t.Fatal("no body")
	}
	var children []*html.Node
	for c := range body.ChildNodes() {
		children = append(children, c)
	}
	if len(children) != 2 {
		t.Fatalf("the body has %d children, want the header and the message's holder", len(children))
	}
	header, holder := children[0], children[1]
	if header.DataAtom != atom.Div || len(header.Attr) != 1 || header.Attr[0] != (html.Attribute{Key: "class", Val: "print-only"}) {
		t.Errorf("first child %s %v, want the header", header.Data, header.Attr)
	}
	if holder.DataAtom != atom.Div || len(holder.Attr) != 0 {
		t.Errorf("second child %s %v, want a plain div", holder.Data, holder.Attr)
	}
	var inner strings.Builder
	for c := range holder.ChildNodes() {
		if err := html.Render(&inner, c); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(inner.String(), "From: Bank</div></div>") || !strings.HasSuffix(inner.String(), "<p>y</p>") {
		t.Errorf("the holder does not hold the whole message: %s", inner.String())
	}
}

func TestPageRulesBlockAllButDataAndAbout(t *testing.T) {
	var rules []struct {
		Trigger struct {
			URLFilter string `json:"url-filter"`
		} `json:"trigger"`
		Action struct {
			Type string `json:"type"`
		} `json:"action"`
	}
	if err := json.Unmarshal([]byte(uikit.PageRules), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 || rules[0].Action.Type != "block" || rules[0].Trigger.URLFilter != ".*" {
		t.Fatalf("rules = %+v", rules)
	}
	for _, r := range rules[1:] {
		if r.Action.Type != "ignore-previous-rules" || (r.Trigger.URLFilter != "^data:" && r.Trigger.URLFilter != "^about:") {
			t.Errorf("rule allows more than data: and about: %+v", r)
		}
	}
}
