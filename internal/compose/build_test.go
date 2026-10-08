package compose

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

func baseMessage() Message {
	return Message{
		From:       Address{Name: "Ann Example", Addr: "ann@x.test"},
		To:         []Address{{Name: "Bob", Addr: "bob@x.test"}},
		Cc:         []Address{{Addr: "carol@x.test"}},
		Bcc:        []Address{{Addr: "dan@x.test"}},
		Subject:    "Lunch",
		MessageID:  "abc123@x.test",
		InReplyTo:  "parent@x.test",
		References: []string{"root@x.test", "parent@x.test"},
		Date:       time.Date(2026, 10, 7, 12, 30, 0, 0, time.UTC),
		HTML:       "<p>Hello <b>Bob</b></p><p>Second line</p>",
	}
}

func build(t *testing.T, m Message) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Build(&buf, m); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return buf.Bytes()
}

// readPart is one leaf of a built message, read back.
type readPart struct {
	typ, disposition, filename string
	body                       []byte
}

func readBack(t *testing.T, raw []byte) (mail.Header, []readPart) {
	t.Helper()
	r, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("CreateReader: %v", err)
	}
	var parts []readPart
	for {
		p, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		body, err := io.ReadAll(p.Body)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		var got readPart
		got.body = body
		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			got.typ, _, _ = h.ContentType()
			got.disposition = "inline"
		case *mail.AttachmentHeader:
			got.typ, _, _ = h.ContentType()
			got.disposition = "attachment"
			got.filename, _ = h.Filename()
		}
		parts = append(parts, got)
	}
	return r.Header, parts
}

func addrs(t *testing.T, h mail.Header, key string) []string {
	t.Helper()
	list, err := h.AddressList(key)
	if err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	var out []string
	for _, a := range list {
		out = append(out, a.Name+" <"+a.Address+">")
	}
	return out
}

func TestBuildHeaders(t *testing.T) {
	raw := build(t, baseMessage())
	h, parts := readBack(t, raw)

	if got := addrs(t, h, "From"); !slices.Equal(got, []string{"Ann Example <ann@x.test>"}) {
		t.Errorf("From = %q", got)
	}
	if got := addrs(t, h, "To"); !slices.Equal(got, []string{"Bob <bob@x.test>"}) {
		t.Errorf("To = %q", got)
	}
	if got := addrs(t, h, "Cc"); !slices.Equal(got, []string{" <carol@x.test>"}) {
		t.Errorf("Cc = %q", got)
	}
	if h.Has("Bcc") {
		t.Errorf("a sent message has a Bcc field: %q", h.Get("Bcc"))
	}
	if s, _ := h.Subject(); s != "Lunch" {
		t.Errorf("Subject = %q", s)
	}
	if id, _ := h.MessageID(); id != "abc123@x.test" {
		t.Errorf("Message-ID = %q", id)
	}
	if ids, _ := h.MsgIDList("In-Reply-To"); !slices.Equal(ids, []string{"parent@x.test"}) {
		t.Errorf("In-Reply-To = %q", ids)
	}
	if ids, _ := h.MsgIDList("References"); !slices.Equal(ids, []string{"root@x.test", "parent@x.test"}) {
		t.Errorf("References = %q", ids)
	}
	if d, _ := h.Date(); !d.Equal(baseMessage().Date) {
		t.Errorf("Date = %v", d)
	}
	if h.Get("Mime-Version") != "1.0" {
		t.Errorf("MIME-Version = %q", h.Get("Mime-Version"))
	}
	if typ, _, _ := h.ContentType(); typ != "multipart/alternative" {
		t.Errorf("Content-Type = %q, want multipart/alternative", typ)
	}

	if len(parts) != 2 || parts[0].typ != "text/plain" || parts[1].typ != "text/html" {
		t.Fatalf("parts = %+v, want text/plain then text/html", parts)
	}
	if text := string(parts[0].body); !strings.Contains(text, "Hello Bob") || !strings.Contains(text, "Second line") {
		t.Errorf("text part = %q", text)
	}
	if html := string(parts[1].body); !strings.Contains(html, "<body><p>Hello <b>Bob</b></p><p>Second line</p></body>") ||
		!strings.Contains(html, `<meta charset="utf-8">`) {
		t.Errorf("html part = %q", html)
	}
}

func TestBuildWithoutReplyFields(t *testing.T) {
	m := baseMessage()
	m.InReplyTo, m.References, m.Cc = "", nil, nil
	h, _ := readBack(t, build(t, m))
	for _, k := range []string{"In-Reply-To", "References", "Cc"} {
		if h.Has(k) {
			t.Errorf("%s written for an empty value: %q", k, h.Get(k))
		}
	}
}

func TestBuildDraftCopyKeepsBcc(t *testing.T) {
	m := baseMessage()
	m.WriteBcc = true
	h, _ := readBack(t, build(t, m))
	if got := addrs(t, h, "Bcc"); !slices.Equal(got, []string{" <dan@x.test>"}) {
		t.Errorf("Bcc = %q", got)
	}
}

func TestBuildIsSevenBitWithCRLF(t *testing.T) {
	m := baseMessage()
	m.Subject = "Café — straße, déjà vu, and a subject long enough that it has to be folded across lines"
	m.From.Name = "Zoë Ångström"
	m.HTML = "<p>Grüße aus Köln 🎉 " + strings.Repeat("long words ", 40) + "</p>"
	m.References = nil
	for i := range 12 {
		m.References = append(m.References, strings.Repeat("r", 20)+string(rune('a'+i))+"@x.test")
	}
	m.Attachments = []Attachment{{Filename: "Übersicht März.pdf", ContentType: "application/pdf", Open: opener("pdf")}}
	raw := build(t, m)

	for i, c := range raw {
		if c >= 0x80 {
			t.Fatalf("byte %d is 0x%x: the message is not 7-bit", i, c)
		}
	}
	for i, line := range bytes.Split(raw, []byte("\r\n")) {
		if bytes.ContainsAny(line, "\r\n") {
			t.Fatalf("line %d has a bare CR or LF: %q", i, line)
		}
		if len(line) > 998 {
			t.Fatalf("line %d is %d bytes", i, len(line))
		}
	}

	h, parts := readBack(t, raw)
	if s, _ := h.Subject(); s != m.Subject {
		t.Errorf("Subject = %q, want %q", s, m.Subject)
	}
	if got := addrs(t, h, "From"); !slices.Equal(got, []string{"Zoë Ångström <ann@x.test>"}) {
		t.Errorf("From = %q", got)
	}
	if ids, _ := h.MsgIDList("References"); !slices.Equal(ids, m.References) {
		t.Errorf("References = %q", ids)
	}
	if !strings.Contains(string(parts[1].body), "Grüße aus Köln 🎉") {
		t.Errorf("html part lost its text: %q", parts[1].body)
	}
	if parts[2].filename != "Übersicht März.pdf" {
		t.Errorf("filename = %q", parts[2].filename)
	}
}

func opener(content string) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(content)), nil }
}

func TestBuildAttachments(t *testing.T) {
	m := baseMessage()
	bin := string([]byte{0, 1, 2, 0xff, '\r', '\n', 'x'})
	m.Attachments = []Attachment{
		{Filename: "notes.txt", ContentType: "text/plain; charset=utf-8", Open: opener("hello\n")},
		{Filename: "data.bin", ContentType: "", Open: opener(bin)},
		{Filename: "", ContentType: "not a type", Open: opener("x")},
		{Filename: "fwd.eml", ContentType: "message/rfc822", Open: opener("Subject: x\r\n\r\nbody")},
	}
	raw := build(t, m)
	h, parts := readBack(t, raw)
	if typ, _, _ := h.ContentType(); typ != "multipart/mixed" {
		t.Errorf("Content-Type = %q, want multipart/mixed", typ)
	}
	want := []readPart{
		{typ: "text/plain", disposition: "inline"},
		{typ: "text/html", disposition: "inline"},
		{typ: "text/plain", disposition: "attachment", filename: "notes.txt", body: []byte("hello\n")},
		{typ: "application/octet-stream", disposition: "attachment", filename: "data.bin", body: []byte(bin)},
		{typ: "application/octet-stream", disposition: "attachment", filename: "attachment", body: []byte("x")},
		{typ: "application/octet-stream", disposition: "attachment", filename: "fwd.eml", body: []byte("Subject: x\r\n\r\nbody")},
	}
	if len(parts) != len(want) {
		t.Fatalf("got %d parts, want %d: %+v", len(parts), len(want), parts)
	}
	for i, w := range want {
		g := parts[i]
		if g.typ != w.typ || g.disposition != w.disposition || g.filename != w.filename {
			t.Errorf("part %d = %s %s %q, want %s %s %q", i, g.typ, g.disposition, g.filename, w.typ, w.disposition, w.filename)
		}
		if w.body != nil && !bytes.Equal(g.body, w.body) {
			t.Errorf("part %d body = %q, want %q", i, g.body, w.body)
		}
	}
	if !regexp.MustCompile(`(?i)Content-Transfer-Encoding: base64`).Match(raw) {
		t.Error("attachments are not base64")
	}
}

func TestBuildAttachmentOpenError(t *testing.T) {
	m := baseMessage()
	boom := errors.New("boom")
	m.Attachments = []Attachment{{Filename: "a", Open: func() (io.ReadCloser, error) { return nil, boom }}}
	if err := Build(io.Discard, m); !errors.Is(err, boom) {
		t.Fatalf("Build = %v, want the open error", err)
	}
}

func TestBuildRejectsInjection(t *testing.T) {
	cases := map[string]func(*Message){
		"subject CRLF":       func(m *Message) { m.Subject = "hi\r\nBcc: evil@x.test" },
		"subject LF":         func(m *Message) { m.Subject = "hi\nX: y" },
		"name LF":            func(m *Message) { m.To[0].Name = "Bob\nBcc: evil@x.test" },
		"address with name":  func(m *Message) { m.To[0].Addr = "Bob <bob@x.test>" },
		"address CRLF":       func(m *Message) { m.Cc[0].Addr = "carol@x.test\r\nBcc: e@x.test" },
		"not an address":     func(m *Message) { m.Bcc[0].Addr = "dan" },
		"empty from":         func(m *Message) { m.From.Addr = "" },
		"message-id space":   func(m *Message) { m.MessageID = "a b@x.test" },
		"message-id bracket": func(m *Message) { m.MessageID = "a>@x.test" },
		"message-id no at":   func(m *Message) { m.MessageID = "abc" },
		"in-reply-to CRLF":   func(m *Message) { m.InReplyTo = "a@x\r\nX: y" },
		"reference":          func(m *Message) { m.References = []string{"ok@x.test", ""} },
		"filename CR":        func(m *Message) { m.Attachments = []Attachment{{Filename: "a\r.txt", Open: opener("")}} },
		"type LF":            func(m *Message) { m.Attachments = []Attachment{{ContentType: "text/plain\nX: y", Open: opener("")}} },
		"no content":         func(m *Message) { m.Attachments = []Attachment{{Filename: "a"}} },
		"no date":            func(m *Message) { m.Date = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := baseMessage()
			mutate(&m)
			var buf bytes.Buffer
			err := Build(&buf, m)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Build = %v, want ErrInvalid", err)
			}
			if buf.Len() != 0 {
				t.Fatalf("Build wrote %d bytes before refusing", buf.Len())
			}
		})
	}
}

func TestEnvelope(t *testing.T) {
	m := baseMessage()
	m.To = append(m.To, Address{Addr: "CAROL@x.test"})
	m.Bcc = append(m.Bcc, Address{Addr: "bob@X.test"})
	got := m.Envelope()
	want := []string{"bob@x.test", "CAROL@x.test", "dan@x.test"}
	if !slices.Equal(got, want) {
		t.Fatalf("Envelope = %q, want %q", got, want)
	}
}

func TestNewMessageID(t *testing.T) {
	a, b := NewMessageID("Ann@Mail.X.Test"), NewMessageID("ann@mail.x.test")
	if a == b {
		t.Fatalf("two identifiers are equal: %s", a)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}@mail\.x\.test$`).MatchString(a) {
		t.Fatalf("NewMessageID = %q", a)
	}
	if !validMsgID(a) {
		t.Fatalf("%q does not pass validMsgID", a)
	}
	for _, from := range []string{"", "nobody", "x@", "x@bad domain"} {
		if id := NewMessageID(from); !strings.HasSuffix(id, "@frostmail.invalid") {
			t.Errorf("NewMessageID(%q) = %q, want the fallback domain", from, id)
		}
	}
}

// structure renders a message's MIME tree as type[children,...].
func structure(t *testing.T, raw []byte) string {
	t.Helper()
	e, err := message.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("message.Read: %v", err)
	}
	var render func(*message.Entity) string
	render = func(e *message.Entity) string {
		typ, _, _ := e.Header.ContentType()
		mr := e.MultipartReader()
		if mr == nil {
			return typ
		}
		var kids []string
		for {
			p, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("NextPart: %v", err)
			}
			kids = append(kids, render(p))
		}
		return typ + "[" + strings.Join(kids, ",") + "]"
	}
	return render(e)
}

func TestBuildStructure(t *testing.T) {
	png := Inline{CID: "img1@x.test", ContentType: "image/png", Open: opener("\x89PNG")}
	pdf := Attachment{Filename: "a.pdf", ContentType: "application/pdf", Open: opener("%PDF")}
	cases := []struct {
		name   string
		inline []Inline
		attach []Attachment
		want   string
	}{
		{"plain", nil, nil, "multipart/alternative[text/plain,text/html]"},
		{"inline", []Inline{png}, nil, "multipart/alternative[text/plain,multipart/related[text/html,image/png]]"},
		{"attachment", nil, []Attachment{pdf}, "multipart/mixed[multipart/alternative[text/plain,text/html],application/pdf]"},
		{"both", []Inline{png}, []Attachment{pdf},
			"multipart/mixed[multipart/alternative[text/plain,multipart/related[text/html,image/png]],application/pdf]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := baseMessage()
			m.Inline, m.Attachments = c.inline, c.attach
			if got := structure(t, build(t, m)); got != c.want {
				t.Fatalf("structure = %s, want %s", got, c.want)
			}
		})
	}
}

func TestBuildInlineImage(t *testing.T) {
	m := baseMessage()
	m.HTML = `<p>See <img src="cid:img1@x.test"></p>`
	m.Inline = []Inline{{CID: "img1@x.test", ContentType: "image/png", Open: opener("\x89PNG\r\n")}}
	raw := build(t, m)
	if !bytes.Contains(raw, []byte("Content-Id: <img1@x.test>")) {
		t.Fatalf("no Content-ID header in:\n%s", raw)
	}
	if !bytes.Contains(raw, []byte(`type="text/html"`)) && !bytes.Contains(raw, []byte(`type=text/html`)) {
		t.Errorf("multipart/related has no type parameter")
	}
	_, parts := readBack(t, raw)
	if len(parts) != 3 || parts[2].typ != "image/png" || string(parts[2].body) != "\x89PNG\r\n" {
		t.Fatalf("parts = %+v, want the image third with its bytes", parts)
	}
}

func TestBuildRejectsBadInline(t *testing.T) {
	for name, in := range map[string]Inline{
		"cid":     {CID: "no-at", Open: opener("")},
		"cid CR":  {CID: "a@b\r", Open: opener("")},
		"type LF": {CID: "a@b", ContentType: "image/png\nX: y", Open: opener("")},
		"no open": {CID: "a@b"},
	} {
		t.Run(name, func(t *testing.T) {
			m := baseMessage()
			m.Inline = []Inline{in}
			if err := Build(io.Discard, m); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Build = %v, want ErrInvalid", err)
			}
		})
	}
}
