package mimex

import (
	"html"
	"strings"
	"testing"
)

func TestPreviewRemovesNewsletterPadding(t *testing.T) {
	const lead = "Your order shipped"
	padding := strings.Repeat("&#8199;&#847;", 40)
	for name, body := range map[string]string{
		"plain entity text":     lead + padding,
		"decoded padding":       lead + strings.Repeat("\u2007\u034f", 40),
		"HTML escaped once":     HTMLToText("<p>" + lead + padding + "</p>"),
		"HTML escaped twice":    HTMLToText("<p>" + lead + html.EscapeString(padding) + "</p>"),
		"hexadecimal and named": lead + "&#x2007;&#x034f;&nbsp;&shy;&ZeroWidthSpace;",
	} {
		t.Run(name, func(t *testing.T) {
			if got := Preview(body, 100); got != lead {
				t.Errorf("preview=%q", got)
			}
		})
	}
}

func TestPreviewCleansBeforeQuoteAndLengthRules(t *testing.T) {
	for _, tc := range []struct {
		in   string
		max  int
		want string
	}{
		{"A\u200b\u200c\u200d\u2060\ufeff\u00ad\u034fB", 20, "AB"},
		{"Milk &amp; honey &unknown;", 50, "Milk & honey &unknown;"},
		{"Caf&#233; &lt;3", 50, "Café <3"},
		{"Hello\n&#62; old message", 50, "Hello"},
		{"Hi\n\u200b--\nsignature", 50, "Hi"},
		{"a\u200bb\u200bc\u200bd\u200be\u200bf", 4, "abc…"},
		{"&amp;amp;", 50, "&amp;"},
	} {
		if got := Preview(tc.in, tc.max); got != tc.want {
			t.Errorf("Preview(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}
