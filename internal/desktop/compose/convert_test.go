package compose_test

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/compose"
)

func roundTrip(src string) string { return compose.ToHTML(compose.ToText(src, nil)) }

func TestNestedQuotesKeepTheirDepth(t *testing.T) {
	out := roundTrip(`<p>Mine</p><blockquote type="cite"><p>One</p><blockquote type="cite"><p>Two</p></blockquote></blockquote>`)
	want := `<p>Mine</p><blockquote type="cite"><p>One</p><blockquote type="cite"><p>Two</p></blockquote></blockquote>`
	if out != want {
		t.Errorf("got  %s\nwant %s", out, want)
	}
}

func TestMarksListsAndLinksSurvive(t *testing.T) {
	src := `<p><strong>b</strong> <em>i</em> <u>u</u> <s>s</s> <a href="https://a.test/">l</a></p><ul><li>x</li><li>y</li></ul><ol><li>z</li></ol>`
	if out := roundTrip(src); out != src {
		t.Errorf("got  %s\nwant %s", out, src)
	}
}

func TestHeadingsReadAsBody(t *testing.T) {
	if out := roundTrip(`<h1>Title</h1>`); out != `<p>Title</p>` {
		t.Errorf("got %s", out)
	}
}

func TestRemoteImagesAndBadLinksAreDropped(t *testing.T) {
	out := roundTrip(`<p>a<img src="https://remote.test/x.png">b <a href="javascript:alert(1)">c</a></p>`)
	if strings.Contains(out, "remote.test") || strings.Contains(out, "javascript") {
		t.Errorf("kept: %s", out)
	}
	if !strings.Contains(out, "c") {
		t.Errorf("lost the link's text: %s", out)
	}
}

func TestPartImagesKeepTheirSource(t *testing.T) {
	out := roundTrip(`<p><img src="mailpart://localhost/m/1/2.png" alt="pic"></p>`)
	if !strings.Contains(out, `src="mailpart://localhost/m/1/2.png"`) || !strings.Contains(out, `alt="pic"`) {
		t.Errorf("got %s", out)
	}
}

func TestTextIsEscaped(t *testing.T) {
	if out := roundTrip(`<p>a &lt;b&gt; &amp; c</p>`); !strings.Contains(out, "a &lt;b&gt; &amp; c") {
		t.Errorf("got %s", out)
	}
}

// The cases of attachmentReminder.test.ts.
func TestMentionsAttachment(t *testing.T) {
	for text, want := range map[string]bool{
		"I attached the file": true, "see attachment.": true, "Enclosed is it": true, "ATTACHMENTS": true,
		"unattached": false, "detached": false, "attachés": false, "nothing here": false,
	} {
		if got := compose.MentionsAttachment(text); got != want {
			t.Errorf("%q = %v", text, got)
		}
	}
}

func TestOwnTextLeavesOutQuotesAttributionAndSignature(t *testing.T) {
	src := `<p>Hello</p><p>Regards Ann</p><p>On Monday, Bo wrote:</p><blockquote><p>I attached it</p></blockquote>`
	if got := compose.OwnText(src, `<p>Regards Ann</p>`); got != "Hello" {
		t.Errorf("own text = %q", got)
	}
}

func TestNeedsReminder(t *testing.T) {
	if !compose.NeedsReminder(`<p>I attached it</p>`, "", 0) {
		t.Error("no reminder for a mention with nothing attached")
	}
	if compose.NeedsReminder(`<p>I attached it</p>`, "", 1) {
		t.Error("a reminder with a chip")
	}
	if compose.NeedsReminder(`<p>I attached it</p><p><img src="data:x"></p>`, "", 0) {
		t.Error("a reminder with an image in the body")
	}
	if !compose.NeedsReminder(`<p>I attached it</p><blockquote><img src="data:x"></blockquote>`, "", 0) {
		t.Error("a quoted image counted as attached")
	}
}
