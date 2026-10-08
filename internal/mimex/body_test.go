package mimex

// CONTRACT TEST for task card T-0018 (docs/tasks). Do not edit.

import (
	"os"
	"strings"
	"testing"
)

func crlf(s string) []byte { return []byte(strings.ReplaceAll(s, "\n", "\r\n")) }

func TestBodyText(t *testing.T) {
	cases := []struct {
		name     string
		raw      []byte
		want     string
		wantHTML bool
	}{
		{"plain", crlf("Subject: hi\nContent-Type: text/plain; charset=utf-8\n\nHello there.\nSecond line.\n"),
			"Hello there.\nSecond line.", false},
		{"no content-type", crlf("Subject: hi\n\nJust text.\n"), "Just text.", false},
		{"quoted-printable latin1", crlf("Content-Type: text/plain; charset=iso-8859-1\nContent-Transfer-Encoding: quoted-printable\n\ncaf=E9 au lait=\n continued\n"),
			"café au lait continued", false},
		{"base64 html only", crlf("Content-Type: text/html; charset=utf-8\nContent-Transfer-Encoding: base64\n\nPHA+SGVsbG8gPGI+d29ybGQ8L2I+PC9wPjxwPlNlY29uZDwvcD4=\n"),
			"Hello world\n\nSecond", true},
		{"alternative prefers plain", crlf(`Content-Type: multipart/alternative; boundary="b"

--b
Content-Type: text/plain

Plain version.
--b
Content-Type: text/html

<p>HTML version.</p>
--b--
`), "Plain version.", true},
		{"empty plain falls back to html", crlf(`Content-Type: multipart/alternative; boundary="b"

--b
Content-Type: text/plain

   
--b
Content-Type: text/html

<p>Only real content.</p>
--b--
`), "Only real content.", true},
		{"mixed with attachments", crlf(`Content-Type: multipart/mixed; boundary="m"

--m
Content-Type: multipart/alternative; boundary="a"

--a
Content-Type: text/plain; charset=utf-8

See attached.
--a
Content-Type: text/html; charset=utf-8

<p>See <b>attached</b>.</p>
--a--
--m
Content-Type: text/plain; name="notes.txt"
Content-Disposition: attachment; filename="notes.txt"

attachment text must not be the body
--m
Content-Type: application/pdf
Content-Transfer-Encoding: base64
Content-Disposition: attachment; filename="q3.pdf"

JVBERi0xLjQK
--m--
`), "See attached.", true},
		{"named part without disposition is an attachment", crlf(`Content-Type: multipart/mixed; boundary="m"

--m
Content-Type: text/plain; name="log.txt"

not the body
--m
Content-Type: text/plain

The body.
--m--
`), "The body.", false},
		{"forwarded message is not the body", crlf(`Content-Type: multipart/mixed; boundary="m"

--m
Content-Type: text/plain

FYI below.
--m
Content-Type: message/rfc822

Subject: inner

inner body
--m--
`), "FYI below.", false},
		{"unknown charset keeps the bytes", crlf("Content-Type: text/plain; charset=x-made-up\n\nhello anyway\n"), "hello anyway", false},
		{"empty body", crlf("Subject: nothing\n\n"), "", false},
	}
	for _, tc := range cases {
		got, html, err := BodyText(tc.raw)
		if err != nil {
			t.Errorf("%s: error %v", tc.name, err)
			continue
		}
		if got != tc.want || html != tc.wantHTML {
			t.Errorf("%s: BodyText = %q, html %v; want %q, html %v", tc.name, got, html, tc.want, tc.wantHTML)
		}
	}
}

func TestBodyTextSeedMessages(t *testing.T) {
	for file, want := range map[string]string{
		"01-welcome.eml":         "Hello! This mailbox is reset from the `clean` snapshot before integration runs.\nUnicode check: naïve café, 日本語, emoji 📬.",
		"04-html-newsletter.eml": "October deals: up to 40% off. View in a browser: https://shop.mailtest.test/oct",
		"05-attachment.eml":      "Numbers attached.",
	} {
		raw, err := os.ReadFile("../../dev/incus/seed/" + file)
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := BodyText(raw)
		if err != nil || got != want {
			t.Errorf("%s: BodyText = %q, %v; want %q", file, got, err, want)
		}
	}
}

func TestBodyTextMalformed(t *testing.T) {
	if _, _, err := BodyText([]byte("this is not a header\x00\n")); err == nil {
		t.Fatal("BodyText of garbage succeeded; want an error")
	}
}

func TestBodyTextKeepsSignatureSeparator(t *testing.T) {
	raw := "Content-Type: text/plain\r\n\r\nHi   \r\n-- \r\nAnn  \r\n"
	text, _, err := BodyText([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hi\n-- \nAnn" {
		t.Fatalf("BodyText = %q, want the trailing space of the signature separator kept", text)
	}
}
