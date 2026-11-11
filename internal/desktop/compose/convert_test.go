package compose_test

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/richtext"

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

// M29: lists keep their nesting, also inside a quotation.
func TestNestedListsKeepTheirLevels(t *testing.T) {
	for _, src := range []string{
		`<ul><li>a<ul><li>b</li></ul></li><li>c</li></ul>`,
		`<ol><li>a<ul><li>b<ol><li>c</li></ol></li><li>d</li></ul></li></ol><p>after</p>`,
		`<blockquote type="cite"><ul><li>a<ul><li>b</li></ul></li></ul></blockquote>`,
		`<ul><li>x</li></ul><ol><li>y</li></ol>`,
	} {
		if out := roundTrip(src); out != src {
			t.Errorf("got  %s\nwant %s", out, src)
		}
	}
}

// M28: a picture with no source to find says so and writes no empty image.
func TestPastedPictureWithNoSourceIsTold(t *testing.T) {
	bitmap := compose.ToText(`<p><img src="`+tinyPNG+`"></p>`, nil)
	var a *richtext.Attachment
	for _, p := range bitmap.Paragraphs() {
		a = p.Runs[0].Attributes.Attachment
	}
	if a == nil || a.Image == nil {
		t.Fatalf("no bitmap: %+v", a)
	}
	a.URL = ""
	if lost := compose.KeepPastedImages(bitmap, "<p>no picture</p>"); lost != 1 {
		t.Errorf("lost = %d", lost)
	}
	if out := compose.ToHTML(bitmap); strings.Contains(out, "<img") {
		t.Errorf("wrote %s", out)
	}
}

// M28: only an image's data: URL is kept for a pasted picture.
func TestPastedPictureMustBeAnImageURL(t *testing.T) {
	bitmap := compose.ToText(`<p><img src="`+tinyPNG+`"></p>`, nil)
	for _, p := range bitmap.Paragraphs() {
		p.Runs[0].Attributes.Attachment.URL = ""
	}
	labeled := "data:text/html;base64," + strings.SplitN(tinyPNG, ",", 2)[1]
	if lost := compose.KeepPastedImages(bitmap, `<img src="`+labeled+`">`); lost != 1 {
		t.Errorf("lost = %d", lost)
	}
}

const tinyPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// A pasted remote image is dropped when the draft is written.
func TestARemoteImageNeverReachesTheDraft(t *testing.T) {
	text := richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: "\uFFFC", Attributes: richtext.Attributes{
		Attachment: &richtext.Attachment{URL: "https://tracker.example/x.png"}}}}})
	if out := compose.ToHTML(text); strings.Contains(out, "tracker.example") {
		t.Errorf("wrote %s", out)
	}
	kept := richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: "\uFFFC", Attributes: richtext.Attributes{
		Attachment: &richtext.Attachment{URL: "mailpart://localhost/m/1/2.png"}}}}})
	if out := compose.ToHTML(kept); !strings.Contains(out, "mailpart://localhost/m/1/2.png") {
		t.Errorf("a part was dropped: %s", out)
	}
}
