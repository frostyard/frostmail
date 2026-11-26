//go:build !js

package main

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/nsgo-dev/nsgo/core"
)

// TestMessagePageRunsNoScript holds the reader's page to its rules
// (AGENTS.md, Conventions; ADR-0005): script off, no stored state, the
// page's policy first in its head, and a window's link opened only on the
// user's press and only when the opener allows it.
func TestMessagePageRunsNoScript(t *testing.T) {
	var opened []string
	doc := mail.ScrollingDocument(`<p>hi</p><script>alert(1)</script>`, "")
	v := messageWebView(doc, func(link string) { opened = append(opened, link) })
	if !v.NoJavaScript {
		t.Error("the message's page runs script")
	}
	if !v.Incognito {
		t.Error("the message's page keeps state")
	}
	head, _, ok := strings.Cut(v.Source.HTML, "</head>")
	if !ok || !strings.Contains(head, `http-equiv="Content-Security-Policy" content="`+mail.PageCSP+`"`) {
		t.Errorf("the page's head has no policy: %q", head)
	}
	if !strings.Contains(mail.PageCSP, "default-src 'none'") || strings.Contains(mail.PageCSP, "script-src") {
		t.Errorf("the policy allows more than it should: %q", mail.PageCSP)
	}
	if v.Source.BaseURL != "" {
		t.Errorf("the page has a base URL %q", v.Source.BaseURL)
	}
	v.OnOpenWindow(core.WebWindowRequest{URL: "https://example.com/a", UserInitiated: false})
	v.OnOpenWindow(core.WebWindowRequest{URL: "javascript:alert(1)", UserInitiated: true})
	v.OnOpenWindow(core.WebWindowRequest{URL: "file:///etc/passwd", UserInitiated: true})
	v.OnOpenWindow(core.WebWindowRequest{URL: "https://example.com/b", UserInitiated: true})
	if len(opened) != 1 || opened[0] != "https://example.com/b" {
		t.Errorf("opened %q; want only the pressed https link", opened)
	}
}
