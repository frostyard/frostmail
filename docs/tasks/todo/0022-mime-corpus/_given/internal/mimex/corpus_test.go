package mimex

// CONTRACT TEST for task card T-0022 (docs/tasks). Do not edit.
//
// The corpus is generated: every fixture is built from a known truth (the
// text it encodes and the attachments it attaches), so expectations are the
// generator's intent, not the implementation's output.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/quotedprintable"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

type sample struct{ charset, text, encoded string }

var samples = []sample{
	{"utf-8", "Grüße, naïve café — ✓ done", "\x47\x72\xc3\xbc\xc3\x9f\x65\x2c\x20\x6e\x61\xc3\xaf\x76\x65\x20\x63\x61\x66\xc3\xa9\x20\xe2\x80\x94\x20\xe2\x9c\x93\x20\x64\x6f\x6e\x65"},
	{"iso-8859-1", "Grüße, naïve café", "\x47\x72\xfc\xdf\x65\x2c\x20\x6e\x61\xef\x76\x65\x20\x63\x61\x66\xe9"},
	{"windows-1252", "“Smart quotes” – café", "\x93\x53\x6d\x61\x72\x74\x20\x71\x75\x6f\x74\x65\x73\x94\x20\x96\x20\x63\x61\x66\xe9"},
	{"koi8-r", "Привет, мир", "\xf0\xd2\xc9\xd7\xc5\xd4\x2c\x20\xcd\xc9\xd2"},
	{"shift_jis", "こんにちは世界", "\x82\xb1\x82\xf1\x82\xc9\x82\xbf\x82\xcd\x90\xa2\x8a\x45"},
	{"gb2312", "你好，世界", "\xc4\xe3\xba\xc3\xa3\xac\xca\xc0\xbd\xe7"},
}

var encodings = []string{"8bit", "quoted-printable", "base64"}

type fixture struct {
	name        string
	raw         []byte
	text        string
	hasHTML     bool
	attachments []Attachment
}

// encode applies a transfer encoding; the result ends in CRLF.
func encode(data []byte, enc string) string {
	switch strings.ToLower(enc) {
	case "quoted-printable":
		var b bytes.Buffer
		w := quotedprintable.NewWriter(&b)
		_, _ = w.Write(data)
		_ = w.Close()
		return strings.ReplaceAll(b.String(), "\n", "\r\n") + "\r\n"
	case "base64":
		s := base64.StdEncoding.EncodeToString(data)
		var b strings.Builder
		for len(s) > 76 {
			b.WriteString(s[:76] + "\r\n")
			s = s[76:]
		}
		return b.String() + s + "\r\n"
	}
	return string(data) + "\r\n"
}

// part renders one MIME part with extra headers.
func part(ctype, enc string, data []byte, extra ...string) string {
	h := "Content-Type: " + ctype + "\r\n"
	if enc != "" {
		h += "Content-Transfer-Encoding: " + enc + "\r\n"
	}
	for _, e := range extra {
		h += e + "\r\n"
	}
	return h + "\r\n" + encode(data, enc)
}

// multipart joins parts under a boundary into a Content-Type and body.
func multipart(subtype, boundary string, parts ...string) string {
	var b strings.Builder
	b.WriteString("Content-Type: multipart/" + subtype + "; boundary=\"" + boundary + "\"\r\n\r\n")
	for _, p := range parts {
		b.WriteString("--" + boundary + "\r\n" + p)
	}
	b.WriteString("--" + boundary + "--\r\n")
	return b.String()
}

func buildMessage(n int, body string) []byte {
	return []byte(fmt.Sprintf("From: Gen <gen@mailtest.test>\r\nTo: You <you@mailtest.test>\r\nSubject: Fixture %d\r\n"+
		"Date: Wed, 07 Oct 2026 10:00:00 +0000\r\nMessage-ID: <fixture-%d@mailtest.test>\r\nMIME-Version: 1.0\r\n", n, n) + body)
}

var (
	pdfData  = []byte("%PDF-1.4 not really a pdf\n")
	pngData  = []byte("\x89PNG\r\n\x1a\nfake image bytes")
	csvData  = []byte("a,b\n1,2\n")
	noteData = []byte("attached notes")
)

func generatedFixtures() []fixture {
	var fs []fixture
	n := 0
	for _, s := range samples {
		for _, enc := range encodings {
			cs := "; charset=" + s.charset
			text := []byte(s.encoded)
			html := []byte("<p>" + s.encoded + "</p>")
			plainPart := part("text/plain"+cs, enc, text)
			htmlPart := part("text/html"+cs, enc, html)
			pdf := Attachment{Filename: "report.pdf", ContentType: "application/pdf", Size: int64(len(pdfData))}
			note := Attachment{Filename: "notes.txt", ContentType: "text/plain", Size: int64(len(noteData))}
			logo := Attachment{Filename: "logo.png", ContentType: "image/png", Size: int64(len(pngData)), ContentID: "logo@mailtest.test", Inline: true}
			csv := Attachment{Filename: "data.csv", ContentType: "text/csv", Size: int64(len(csvData))}
			pdfPart := part("application/pdf", "base64", pdfData, `Content-Disposition: attachment; filename="report.pdf"`)
			notePart := part("text/plain; charset=us-ascii", "7bit", noteData, `Content-Disposition: attachment; filename="notes.txt"`)
			logoPart := part("image/png", "base64", pngData, "Content-ID: <logo@mailtest.test>", `Content-Disposition: inline; filename="logo.png"`)
			csvPart := part("text/csv", "base64", csvData, `Content-Disposition: attachment; filename="data.csv"`)
			for _, st := range []struct {
				name    string
				body    string
				hasHTML bool
				atts    []Attachment
			}{
				{"plain", plainPart, false, nil},
				{"html", htmlPart, true, nil},
				{"alternative", multipart("alternative", "a", plainPart, htmlPart), true, nil},
				{"alternative-html-first", multipart("alternative", "a", htmlPart, plainPart), true, nil},
				{"mixed-pdf", multipart("mixed", "m", multipart("alternative", "a", plainPart, htmlPart), pdfPart), true, []Attachment{pdf}},
				{"mixed-text-attachment", multipart("mixed", "m", plainPart, notePart), false, []Attachment{note}},
				{"related", multipart("related", "r", htmlPart, logoPart), true, []Attachment{logo}},
				{"nested", multipart("mixed", "m", multipart("alternative", "a", plainPart, multipart("related", "r", htmlPart, logoPart)), csvPart), true, []Attachment{logo, csv}},
			} {
				n++
				fs = append(fs, fixture{
					name: fmt.Sprintf("%s/%s/%s", s.charset, enc, st.name), raw: buildMessage(n, st.body),
					text: s.text, hasHTML: st.hasHTML, attachments: st.atts,
				})
			}
		}
	}
	return fs
}

func brokenFixtures() []fixture {
	// The CRLF before a boundary belongs to the delimiter (RFC 2046), so
	// the forwarded part is fwd without a final line break.
	fwd := "From: a@x.test\r\nSubject: inner\r\n\r\ninner body"
	return []fixture{
		{name: "broken/no-content-type", raw: []byte("Subject: old\r\n\r\nPlain old text\r\n"), text: "Plain old text"},
		{name: "broken/lf-line-endings", raw: []byte("Subject: lf\nMIME-Version: 1.0\nContent-Type: multipart/alternative; boundary=\"x\"\n\n--x\nContent-Type: text/plain\n\nUnix lines\n--x\nContent-Type: text/html\n\n<p>Unix lines</p>\n--x--\n"),
			text: "Unix lines", hasHTML: true},
		{name: "broken/missing-final-boundary", raw: []byte("Subject: cut\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"m\"\r\n\r\n--m\r\nContent-Type: text/plain\r\n\r\nKept text\r\n--m\r\nContent-Type: application/pdf\r\nContent-Transfer-Encoding: base64\r\nContent-Disposition: attachment; filename=\"cut.pdf\"\r\n\r\n" + base64.StdEncoding.EncodeToString(pdfData) + "\r\n"),
			text: "Kept text", attachments: []Attachment{{Filename: "cut.pdf", ContentType: "application/pdf", Size: int64(len(pdfData))}}},
		{name: "broken/charset-utf8-label", raw: []byte("Content-Type: text/plain; charset=utf8\r\n\r\nnaïve label\r\n"), text: "naïve label"},
		{name: "broken/unknown-charset", raw: []byte("Content-Type: text/plain; charset=x-unknown\r\n\r\ncafé anyway\r\n"), text: "café anyway"},
		{name: "broken/rfc2231-filename", raw: []byte("MIME-Version: 1.0\r\n" + multipart("mixed", "m", part("text/plain", "", []byte("See CV")),
			part("application/pdf", "base64", pdfData, "Content-Disposition: attachment; filename*=utf-8''r%C3%A9sum%C3%A9.pdf"))),
			text: "See CV", attachments: []Attachment{{Filename: "résumé.pdf", ContentType: "application/pdf", Size: int64(len(pdfData))}}},
		{name: "broken/encoded-word-filename", raw: []byte("MIME-Version: 1.0\r\n" + multipart("mixed", "m", part("text/plain", "", []byte("See CV")),
			part("application/pdf", "base64", pdfData, `Content-Disposition: attachment; filename="=?UTF-8?Q?r=C3=A9sum=C3=A9.pdf?="`))),
			text: "See CV", attachments: []Attachment{{Filename: "résumé.pdf", ContentType: "application/pdf", Size: int64(len(pdfData))}}},
		{name: "broken/uppercase-encodings", raw: []byte("MIME-Version: 1.0\r\n" + multipart("alternative", "a", part("text/plain; charset=utf-8", "BASE64", []byte("Shouted encoding")),
			part("text/html; charset=utf-8", "Quoted-Printable", []byte("<p>Shouted encoding</p>")))), text: "Shouted encoding", hasHTML: true},
		{name: "broken/forwarded-message", raw: []byte("MIME-Version: 1.0\r\n" + multipart("mixed", "m", part("text/plain", "", []byte("FYI")),
			"Content-Type: message/rfc822\r\nContent-Disposition: attachment; filename=\"fwd.eml\"\r\n\r\n"+fwd+"\r\n")),
			text: "FYI", attachments: []Attachment{{Filename: "fwd.eml", ContentType: "message/rfc822", Size: int64(len(fwd))}}},
		{name: "broken/name-in-content-type-only", raw: []byte("MIME-Version: 1.0\r\n" + multipart("mixed", "m", part("text/plain", "", []byte("Old style")),
			part(`application/pdf; name="old.pdf"`, "base64", pdfData))),
			text: "Old style", attachments: []Attachment{{Filename: "old.pdf", ContentType: "application/pdf", Size: int64(len(pdfData))}}},
	}
}

func TestCorpus(t *testing.T) {
	fixtures := append(generatedFixtures(), brokenFixtures()...)
	if len(fixtures) < 150 {
		t.Fatalf("corpus has %d fixtures; M1 needs at least 150", len(fixtures))
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			text, hasHTML, err := BodyText(f.raw)
			if err != nil {
				t.Fatalf("BodyText: %v", err)
			}
			if text != f.text || hasHTML != f.hasHTML {
				t.Errorf("BodyText = %q, html %v; want %q, html %v", text, hasHTML, f.text, f.hasHTML)
			}
			if !utf8.ValidString(text) {
				t.Error("text is not valid UTF-8")
			}
			atts, err := Attachments(f.raw)
			if err != nil {
				t.Fatalf("Attachments: %v", err)
			}
			if !reflect.DeepEqual(atts, f.attachments) {
				t.Errorf("Attachments = %+v\nwant %+v", atts, f.attachments)
			}
		})
	}
}
