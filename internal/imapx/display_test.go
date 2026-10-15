package imapx_test

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mimex"
)

// leaf is a part of a parsed message: its type, the omitted mark, and its
// content as written (still transfer-encoded).
type leaf struct {
	typ, omitted, body string
}

// leaves parses a message's MIME tree into its single parts, in order.
func leaves(t *testing.T, raw []byte) []leaf {
	t.Helper()
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("the copy does not parse: %v\n%s", err, raw)
	}
	var out []leaf
	var walk func(h interface{ Get(string) string }, body io.Reader)
	walk = func(h interface{ Get(string) string }, body io.Reader) {
		typ, params, err := mime.ParseMediaType(h.Get("Content-Type"))
		if err != nil {
			typ = "text/plain"
		}
		if strings.HasPrefix(typ, "multipart/") {
			r := multipart.NewReader(body, params["boundary"])
			for {
				p, err := r.NextRawPart()
				if err == io.EOF {
					return
				}
				if err != nil {
					t.Fatalf("multipart: %v\n%s", err, raw)
				}
				walk(p.Header, p)
			}
		}
		data, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, leaf{typ: typ, omitted: h.Get(mimex.OmittedHeader), body: string(data)})
	}
	walk(msg.Header, msg.Body)
	return out
}

func crlf(lines ...string) string { return strings.Join(lines, "\r\n") + "\r\n" }

var pdf = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("%PDF-1.7 0123456789abcdef"), 4000))

// A newsletter-shaped message: text and HTML, an image the HTML shows,
// a PDF attached, and an invitation.
var mixed = crlf(
	"From: Ann <ann@example.com>", "To: me@example.com", "Subject: Invoice", "MIME-Version: 1.0",
	`Content-Type: multipart/mixed; boundary="mixed"`, "",
	"--mixed",
	`Content-Type: multipart/related; boundary="related"`, "",
	"--related",
	`Content-Type: multipart/alternative; boundary="alt"`, "",
	"--alt",
	"Content-Type: text/plain; charset=utf-8", "",
	"Your invoice is attached.",
	"--alt",
	"Content-Type: text/html; charset=utf-8", "",
	`<p>Your invoice is attached.<img src="cid:logo@example.com"></p>`,
	"--alt--",
	"--related",
	"Content-Type: image/png", "Content-ID: <logo@example.com>", "Content-Disposition: inline", "Content-Transfer-Encoding: base64", "",
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=",
	"--related--",
	"--mixed",
	`Content-Type: application/pdf; name="invoice.pdf"`, `Content-Disposition: attachment; filename="invoice.pdf"`,
	"Content-Transfer-Encoding: base64", "",
	pdf,
	"--mixed",
	`Content-Type: text/calendar; method=REQUEST; name="invite.ics"`, `Content-Disposition: attachment; filename="invite.ics"`, "",
	"BEGIN:VCALENDAR", "METHOD:REQUEST", "END:VCALENDAR",
	"--mixed",
	`Content-Type: text/csv; name="rows.csv"`, `Content-Disposition: attachment; filename="rows.csv"`, "",
	"a,b,c",
	"--mixed--",
)

func appendAndOpen(t *testing.T, messages ...string) *imapx.Session {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	for _, m := range messages {
		if _, err := mem.User.Append("INBOX", strings.NewReader(m), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	s, err := imapx.Open(t.Context(), mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.Select(t.Context(), "INBOX"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFetchDisplayLeavesAttachmentsOut(t *testing.T) {
	s := appendAndOpen(t, mixed)
	raw, err := s.FetchDisplay(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > len(mixed)/10 {
		t.Errorf("the copy is %d bytes of the message's %d", len(raw), len(mixed))
	}
	got := leaves(t, raw)
	want := []leaf{
		{typ: "text/plain", body: "Your invoice is attached."},
		{typ: "text/html", body: `<p>Your invoice is attached.<img src="cid:logo@example.com"></p>`},
		{typ: "image/png", body: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="},
		{typ: "application/pdf", omitted: "<size>"},
		{typ: "text/calendar", body: "BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nEND:VCALENDAR"},
		{typ: "text/csv", omitted: "5"},
	}
	if len(got) != len(want) {
		t.Fatalf("parts = %+v", got)
	}
	for i, w := range want {
		g := got[i]
		if w.omitted == "<size>" && g.omitted != "" {
			w.omitted = g.omitted // the server's size of the encoded PDF
		}
		if g.typ != w.typ || g.omitted != w.omitted || strings.TrimRight(g.body, "\r\n") != w.body {
			t.Errorf("part %d = %+v, want %+v", i+1, g, w)
		}
	}
	if got[3].omitted == "" || got[3].omitted == "0" {
		t.Errorf("the PDF's mark = %q", got[3].omitted)
	}
	// The header and every kept part's MIME header are as sent.
	for _, line := range []string{"Subject: Invoice", "Content-ID: <logo@example.com>", `filename="invoice.pdf"`} {
		if !strings.Contains(string(raw), line) {
			t.Errorf("no %q in the copy:\n%s", line, raw)
		}
	}
}

func TestFetchDisplayOfSinglePartMessages(t *testing.T) {
	plain := crlf("From: a@example.com", "Subject: Hi", "Content-Type: text/plain", "", "Hello there.")
	pdfOnly := crlf("From: a@example.com", "Subject: Scan", "Content-Type: application/pdf; name=scan.pdf",
		"Content-Disposition: attachment; filename=scan.pdf", "Content-Transfer-Encoding: base64", "", pdf)
	s := appendAndOpen(t, plain, pdfOnly)
	raw, err := s.FetchDisplay(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := leaves(t, raw); len(got) != 1 || got[0].typ != "text/plain" || strings.TrimSpace(got[0].body) != "Hello there." || got[0].omitted != "" {
		t.Errorf("plain = %+v\n%s", got, raw)
	}
	raw, err = s.FetchDisplay(t.Context(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := leaves(t, raw); len(got) != 1 || got[0].typ != "application/pdf" || got[0].omitted == "" || got[0].body != "" {
		t.Errorf("pdf = %+v\n%s", got, raw)
	}
	if _, err := s.FetchDisplay(t.Context(), 9); err == nil {
		t.Error("a missing UID was fetched")
	}
}
