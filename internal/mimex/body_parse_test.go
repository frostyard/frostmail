package mimex

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// knownText is what a reader must see of these messages: the seed messages
// and the inline shapes below. TestBodyGivesWhatTheWrappersGive also checks
// that Body agrees with BodyText and BodyHTML on every shape.
var knownText = map[string]string{
	"../../dev/incus/seed/01-welcome.eml":         "Hello! This mailbox is reset from the `clean` snapshot before integration runs.\nUnicode check: naïve café, 日本語, emoji 📬.",
	"../../dev/incus/seed/04-html-newsletter.eml": "October deals: up to 40% off. View in a browser: https://shop.mailtest.test/oct",
	"../../dev/incus/seed/05-attachment.eml":      "Numbers attached.",
	"plain":                                       "Hello there.\nSecond line.",
	"html only":                                   "Hello world",
}

// TestBodyGivesWhatTheWrappersGive: the single parse Body gives the text,
// HTML and HTML flag the wrappers give, and the text a reader expects.
func TestBodyGivesWhatTheWrappersGive(t *testing.T) {
	files, err := filepath.Glob("../../dev/incus/seed/*.eml")
	if err != nil || len(files) == 0 {
		t.Fatalf("seed messages = %v, %d found", err, len(files))
	}
	raws := map[string][]byte{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		raws[f] = raw
	}
	raws["plain"] = crlf("Subject: hi\nContent-Type: text/plain; charset=utf-8\n\nHello there.\nSecond line.\n")
	raws["html only"] = crlf("Content-Type: text/html; charset=utf-8\n\n<p>Hello <b>world</b></p>\n")
	raws["alternative"] = crlf(`Content-Type: multipart/alternative; boundary="b"

--b
Content-Type: text/plain

Plain version.
--b
Content-Type: text/html

<p>HTML version.</p>
--b--
`)
	raws["html with a cid image"] = crlf(`Content-Type: multipart/related; boundary="r"

--r
Content-Type: text/html

<p><img src="cid:logo@x"></p>
--r
Content-Type: image/png
Content-Transfer-Encoding: base64
Content-ID: <logo@x>

iVBORw0KGgo=
--r--
`)
	for name, raw := range raws {
		text, html, hasHTML, err := Body(raw)
		if err != nil {
			t.Errorf("%s: Body = %v", name, err)
			continue
		}
		wantText, wantHasHTML, err := BodyText(raw)
		if err != nil {
			t.Errorf("%s: BodyText = %v", name, err)
			continue
		}
		wantHTML, err := BodyHTML(raw)
		if err != nil {
			t.Errorf("%s: BodyHTML = %v", name, err)
			continue
		}
		if text != wantText || hasHTML != wantHasHTML || html != wantHTML {
			t.Errorf("%s: Body = (%q, %q, %v); BodyText = (%q, %v), BodyHTML = (%q)",
				name, text, html, hasHTML, wantText, wantHasHTML, wantHTML)
		}
		if want, ok := knownText[name]; ok && text != want {
			t.Errorf("%s: Body text = %q, want %q", name, text, want)
		}
	}
	if _, _, hasHTML, err := Body(raws["html only"]); err != nil || !hasHTML {
		t.Errorf("an HTML-only message: hasHTML = %v, %v; want true", hasHTML, err)
	}
	if _, _, hasHTML, err := Body(raws["plain"]); err != nil || hasHTML {
		t.Errorf("a plain message: hasHTML = %v, %v; want false", hasHTML, err)
	}
}

// TestBodyParseErrorMatches: a message Body cannot parse is the parse error
// the wrappers report too (mimex.ErrParse).
func TestBodyParseErrorMatches(t *testing.T) {
	raw := []byte("this is not a header\x00\n")
	_, _, _, bodyErr := Body(raw)
	_, _, textErr := BodyText(raw)
	_, htmlErr := BodyHTML(raw)
	for name, err := range map[string]error{"Body": bodyErr, "BodyText": textErr, "BodyHTML": htmlErr} {
		if !errors.Is(err, ErrParse) {
			t.Errorf("%s = %v, want ErrParse", name, err)
		}
	}
}
