package mail_test

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/mail"
)

func TestTextHTMLEscapesEverything(t *testing.T) {
	out := mail.TextHTML(`<script>alert(1)</script> & <img src=x onerror=y>`)
	for _, never := range []string{"<script", "<img"} {
		if strings.Contains(out, never) {
			t.Errorf("%q in %s", never, out)
		}
	}
}

func TestTextHTMLLinks(t *testing.T) {
	out := mail.TextHTML(`See https://a.test/x, www.b.test. Mail me@c.test!`)
	for _, want := range []string{
		`<a href="https://a.test/x">https://a.test/x</a>,`,
		`<a href="https://www.b.test">www.b.test</a>.`,
		`<a href="mailto:me@c.test">me@c.test</a>!`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in %s", want, out)
		}
	}
}

func TestTextHTMLLinkCannotBreakOutOfItsAttribute(t *testing.T) {
	out := mail.TextHTML(`http://a.test/'onclick='x`)
	if strings.Contains(out, `'onclick`) {
		t.Errorf("unescaped quote in %s", out)
	}
}

func TestTextHTMLQuotesAndSignature(t *testing.T) {
	out := mail.TextHTML("Hi\n> one\n>> two\n-- \nAnn")
	if strings.Contains(out, "&gt;") {
		t.Errorf("the prefixes show: %s", out)
	}
	if got := strings.Count(out, `class="quote"`); got != 3 {
		t.Errorf("%d quote bars, want 3 (one level 1, then levels 1 and 2): %s", got, out)
	}
	if !strings.Contains(out, `<div class="sig">Ann</div>`) {
		t.Errorf("no signature: %s", out)
	}
}

// M36: the count of ">" comes from the message; the page's nesting does not.
func TestTextHTMLCapsQuoteDepth(t *testing.T) {
	deep := strings.Repeat(">", 100_000)
	out := mail.TextHTML(deep + " bottom")
	if got := strings.Count(out, `class="quote"`); got != 8 {
		t.Errorf("%d quote bars, want 8", got)
	}
	if len(out) > 1000 {
		t.Errorf("%d bytes for one line", len(out))
	}
	if !strings.Contains(out, "bottom") {
		t.Errorf("the text went: %s", out)
	}
	if opened, closed := strings.Count(out, "<div"), strings.Count(out, "</div>"); opened != closed {
		t.Errorf("%d opened, %d closed: %s", opened, closed, out)
	}
	// Lines past the cap are one block, however deep each is.
	merged := mail.TextHTML(strings.Repeat(">", 9) + " a\n" + strings.Repeat(">", 100) + " b")
	if got := strings.Count(merged, `class="quote"`); got != 8 {
		t.Errorf("%d quote bars for 2 lines past the cap: %s", got, merged)
	}
}

func TestLongQuoteFoldsAfterTwoLines(t *testing.T) {
	out := mail.TextHTML("> a\n> b\n> c\n> d\n> e")
	if !strings.Contains(out, "a\nb</div><details><summary>See More</summary><div>c\nd\ne</div></details>") {
		t.Errorf("fold wrong: %s", out)
	}
	if short := mail.TextHTML("> a\n> b\n> c\n> d"); strings.Contains(short, "details") {
		t.Errorf("a 4-line quote folded: %s", short)
	}
}

func TestPrintHeaderEscapesTheWarning(t *testing.T) {
	out := mail.PrintHeader("Ann", "", "", "", "", "from <x>")
	if !strings.HasSuffix(out, "<div>from &lt;x&gt;</div>") {
		t.Errorf("header: %s", out)
	}
}

func TestPrintHeaderEscapes(t *testing.T) {
	out := mail.PrintHeader("<Ann>", "", "", "", "a&b", "")
	if !strings.Contains(out, "&lt;Ann&gt;") || !strings.Contains(out, "a&amp;b") || strings.Contains(out, "To:") {
		t.Errorf("header: %s", out)
	}
}
