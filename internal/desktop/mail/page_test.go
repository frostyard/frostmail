package mail_test

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/mail"
)

func TestDocumentCarriesThePolicyFirst(t *testing.T) {
	doc := mail.ScrollingDocument(`<p>hi</p>`, "")
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
		if strings.Contains(mail.PageCSP, never) {
			t.Errorf("the policy allows %q", never)
		}
	}
}

// The page's height rule is in a layer of the page's head, after the
// style the reader gives and before the body, where every style of the
// message is: so its layer comes before any layer of the message's, and an
// important rule of the message cannot give the root the viewport's
// height.
func TestDocumentGivesTheRootItsContentHeight(t *testing.T) {
	const rule = `@layer page-height{html,body{height:auto!important}}`
	doc := mail.ScrollingDocument(`<style>@layer message-root{html,body{height:100%!important}}</style>`, `@layer text-size{}`)
	given, at, head := strings.Index(doc, `@layer text-size`), strings.Index(doc, rule), strings.Index(doc, `</head>`)
	if given < 0 || at < given || head < at {
		t.Fatalf("the given style at %d, the height rule at %d, the head's end at %d: %s", given, at, head, doc)
	}
}

// The reader's page scrolls itself: the message is the body, and no rule
// hides the page's own scrollbar.
func TestScrollingDocumentHoldsTheMessageAsItsBody(t *testing.T) {
	doc := mail.ScrollingDocument("<p>body</p>", "")
	if !strings.HasSuffix(doc, `</style></head><body><p>body</p></body></html>`) {
		t.Errorf("the body is not the message: %s", doc)
	}
	if strings.Contains(doc, "overflow-y:hidden") {
		t.Errorf("the page cannot scroll: %s", doc)
	}
}
