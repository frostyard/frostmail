package compose

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"

	"github.com/frostyard/frostmail/internal/mimex"
)

// ErrInvalid marks a message Build refuses to write: a header value with a
// line break, an address that does not parse, a bad message identifier.
var ErrInvalid = errors.New("invalid message")

// Message is what Build writes: a draft's content with its headers.
type Message struct {
	From       Address
	ReplyTo    []Address
	To         []Address
	Cc         []Address
	Bcc        []Address
	Subject    string
	MessageID  string   // without angle brackets
	InReplyTo  string   // without angle brackets; may be empty
	References []string // without angle brackets
	Date       time.Time
	// HTML is the body; the text alternative is made from it.
	HTML string
	// Inline holds the images HTML refers to as cid: URLs.
	Inline      []Inline
	Attachments []Attachment
	// WriteBcc writes the Bcc header field. Only the Drafts copy sets it, so
	// its recipients survive on the server; a sent message never does.
	WriteBcc bool
}

// Inline is an image the HTML shows from a cid:<CID> URL.
type Inline struct {
	CID         string // without angle brackets, in Message-ID syntax
	ContentType string // "" or unparsable means application/octet-stream
	Open        func() (io.ReadCloser, error)
}

// Attachment is one attached file. Open is called once, while Build
// writes the part.
type Attachment struct {
	Filename    string
	ContentType string // "" or unparsable means application/octet-stream
	Open        func() (io.ReadCloser, error)
}

// Build writes m to w as an RFC 5322 message with CRLF line endings
// (docs/design/send.md, Building a message). The body is
// multipart/alternative with a text and an HTML part; the HTML part is
// inside multipart/related with the inline images when there are any, and
// the alternative is inside multipart/mixed with the attachments when there
// are any. Bodies are quoted-printable or base64 and non-ASCII header text
// is RFC 2047 or RFC 2231 encoded, so the message is 7-bit. Build checks m
// before writing anything and returns an error wrapping ErrInvalid for a
// value it will not write.
func Build(w io.Writer, m Message) error {
	if err := m.validate(); err != nil {
		return err
	}
	root := m.body()
	h := m.header()
	for f := root.header.Fields(); f.Next(); {
		h.Add(f.Key(), f.Value())
	}
	mw, err := message.CreateWriter(w, h.Header)
	if err != nil {
		return fmt.Errorf("build message: %w", err)
	}
	if err := root.writeBody(mw); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("build message: %w", err)
	}
	return nil
}

// Envelope returns the SMTP recipients: the To, Cc and Bcc addresses in that
// order, each once (compared case-insensitively).
func (m Message) Envelope() []string {
	var out []string
	seen := map[string]bool{}
	for _, list := range [][]Address{m.To, m.Cc, m.Bcc} {
		for _, a := range list {
			key := strings.ToLower(a.Addr)
			if a.Addr == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, a.Addr)
		}
	}
	return out
}

// NewMessageID returns a random message identifier (without angle brackets)
// in the domain of the address from, or in frostmail.invalid when from has
// no usable domain. Content IDs use it too.
func NewMessageID(from string) string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails
	domain := "frostmail.invalid"
	if i := strings.LastIndexByte(from, '@'); i >= 0 && validMsgIDPart(from[i+1:]) {
		domain = strings.ToLower(from[i+1:])
	}
	return hex.EncodeToString(b[:]) + "@" + domain
}

// header is m's top-level header without its Content-Type; validate has
// accepted m. go-message writes the fields added last first, so they are
// added bottom up.
func (m Message) header() mail.Header {
	var h mail.Header
	h.SetMsgIDList("References", m.References)
	if m.InReplyTo != "" {
		h.SetMsgIDList("In-Reply-To", []string{m.InReplyTo})
	}
	h.SetMessageID(m.MessageID)
	h.SetSubject(m.Subject)
	if m.WriteBcc {
		h.SetAddressList("Bcc", mailAddresses(m.Bcc))
	}
	h.SetAddressList("Cc", mailAddresses(m.Cc))
	h.SetAddressList("To", mailAddresses(m.To))
	h.SetAddressList("Reply-To", mailAddresses(m.ReplyTo))
	h.SetAddressList("From", []*mail.Address{mailAddress(m.From)})
	h.SetDate(m.Date)
	return h
}

// part is one MIME entity to write: a multipart with children, or a leaf
// whose content comes from body.
type part struct {
	header   message.Header
	children []part
	body     func(io.Writer) error
}

// body is the MIME tree of m's content.
func (m Message) body() part {
	html := textPart("text/html", htmlDocument(m.HTML))
	if len(m.Inline) > 0 {
		children := []part{html}
		for _, in := range m.Inline {
			children = append(children, inlinePart(in))
		}
		html = multipart("related", map[string]string{"type": "text/html"}, children)
	}
	alt := multipart("alternative", nil, []part{textPart("text/plain", mimex.HTMLToText(m.HTML)+"\n"), html})
	if len(m.Attachments) == 0 {
		return alt
	}
	children := []part{alt}
	for _, a := range m.Attachments {
		children = append(children, attachmentPart(a))
	}
	return multipart("mixed", nil, children)
}

// writeBody writes p's content into w, whose header is already written.
func (p part) writeBody(w *message.Writer) error {
	if p.body != nil {
		return p.body(w)
	}
	for _, c := range p.children {
		cw, err := w.CreatePart(c.header)
		if err != nil {
			return fmt.Errorf("build message: %w", err)
		}
		if err := c.writeBody(cw); err != nil {
			return err
		}
		if err := cw.Close(); err != nil {
			return fmt.Errorf("build message: %w", err)
		}
	}
	return nil
}

func multipart(subtype string, params map[string]string, children []part) part {
	var h message.Header
	h.SetContentType("multipart/"+subtype, params)
	return part{header: h, children: children}
}

// textPart is a UTF-8 quoted-printable text part.
func textPart(typ, body string) part {
	var h message.Header
	h.Set("Content-Transfer-Encoding", "quoted-printable")
	h.SetContentType(typ, map[string]string{"charset": "utf-8"})
	return part{header: h, body: func(w io.Writer) error {
		if _, err := io.WriteString(w, body); err != nil {
			return fmt.Errorf("build %s part: %w", typ, err)
		}
		return nil
	}}
}

// inlinePart is a base64 image part with its Content-ID.
func inlinePart(in Inline) part {
	typ, params := leafType(in.ContentType)
	var h message.Header
	h.Set("Content-Transfer-Encoding", "base64")
	h.Set("Content-Disposition", "inline")
	h.Set("Content-Id", "<"+in.CID+">")
	h.SetContentType(typ, params)
	return part{header: h, body: copyFrom(in.Open, "inline "+in.CID)}
}

// attachmentPart is a base64 attachment part. The file name goes in
// Content-Disposition (RFC 2231 when not ASCII) and, for older readers, in
// Content-Type's name parameter (RFC 2047 when not ASCII).
func attachmentPart(a Attachment) part {
	name := a.Filename
	if name == "" {
		name = "attachment"
	}
	typ, params := leafType(a.ContentType)
	params["name"] = name
	var h message.Header
	h.Set("Content-Transfer-Encoding", "base64")
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	h.SetContentType(typ, params)
	return part{header: h, body: copyFrom(a.Open, "attachment "+name)}
}

// leafType parses a part's content type, falling back to
// application/octet-stream when it does not parse or names a multipart or
// message type (which may not be base64 encoded). The params are never nil.
func leafType(contentType string) (string, map[string]string) {
	typ, params, err := mime.ParseMediaType(contentType)
	if err != nil || strings.HasPrefix(typ, "multipart/") || strings.HasPrefix(typ, "message/") {
		return "application/octet-stream", map[string]string{}
	}
	if params == nil {
		params = map[string]string{}
	}
	return typ, params
}

// copyFrom writes a part's content from open.
func copyFrom(open func() (io.ReadCloser, error), what string) func(io.Writer) error {
	return func(w io.Writer) error {
		r, err := open()
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		defer r.Close()
		if _, err := io.Copy(w, r); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		return nil
	}
}

// htmlDocument wraps the body in the minimal document of the text/html part.
func htmlDocument(body string) string {
	return "<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"></head><body>" + body + "</body></html>\n"
}

// validate checks every value Build writes into a header.
func (m Message) validate() error {
	if m.Date.IsZero() {
		return fmt.Errorf("%w: no date", ErrInvalid)
	}
	if err := checkAddress("From", m.From); err != nil {
		return err
	}
	for _, f := range []struct {
		name string
		list []Address
	}{{"Reply-To", m.ReplyTo}, {"To", m.To}, {"Cc", m.Cc}, {"Bcc", m.Bcc}} {
		for _, a := range f.list {
			if err := checkAddress(f.name, a); err != nil {
				return err
			}
		}
	}
	if err := checkText("Subject", m.Subject); err != nil {
		return err
	}
	if !validMsgID(m.MessageID) {
		return fmt.Errorf("%w: Message-ID %q", ErrInvalid, m.MessageID)
	}
	if m.InReplyTo != "" && !validMsgID(m.InReplyTo) {
		return fmt.Errorf("%w: In-Reply-To %q", ErrInvalid, m.InReplyTo)
	}
	for _, id := range m.References {
		if !validMsgID(id) {
			return fmt.Errorf("%w: References %q", ErrInvalid, id)
		}
	}
	for _, in := range m.Inline {
		if !validMsgID(in.CID) {
			return fmt.Errorf("%w: Content-ID %q", ErrInvalid, in.CID)
		}
		if err := checkText("inline type", in.ContentType); err != nil {
			return err
		}
		if in.Open == nil {
			return fmt.Errorf("%w: inline %q has no content", ErrInvalid, in.CID)
		}
	}
	for _, a := range m.Attachments {
		if err := checkText("attachment name", a.Filename); err != nil {
			return err
		}
		if err := checkText("attachment type", a.ContentType); err != nil {
			return err
		}
		if a.Open == nil {
			return fmt.Errorf("%w: attachment %q has no content", ErrInvalid, a.Filename)
		}
	}
	return nil
}

// ValidAddress reports whether Build accepts a as a recipient.
func ValidAddress(a Address) bool { return checkAddress("", a) == nil }

// checkAddress accepts an address whose Addr is a bare addr-spec and whose
// Name has no line break.
func checkAddress(field string, a Address) error {
	if err := checkText(field+" name", a.Name); err != nil {
		return err
	}
	p, err := netmail.ParseAddress(a.Addr)
	if err != nil || p.Name != "" || p.Address != a.Addr {
		return fmt.Errorf("%w: %s address %q", ErrInvalid, field, a.Addr)
	}
	return nil
}

// checkText refuses CR, LF and NUL, which could end a header field early
// (header injection).
func checkText(field, s string) error {
	if strings.ContainsAny(s, "\r\n\x00") {
		return fmt.Errorf("%w: %s contains a line break", ErrInvalid, field)
	}
	return nil
}

// validMsgID accepts left@right with each side a validMsgIDPart.
func validMsgID(id string) bool {
	left, right, ok := strings.Cut(id, "@")
	return ok && validMsgIDPart(left) && validMsgIDPart(right)
}

// validMsgIDPart accepts one or more printable ASCII characters other than
// space, '@', '<', '>' and the RFC 5322 specials that would need quoting.
func validMsgIDPart(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if c <= ' ' || c >= 0x7f || strings.IndexByte(`@<>()[]\,;:"`, c) >= 0 {
			return false
		}
	}
	return true
}

func mailAddress(a Address) *mail.Address {
	return &mail.Address{Name: a.Name, Address: a.Addr}
}

func mailAddresses(list []Address) []*mail.Address {
	out := make([]*mail.Address, 0, len(list))
	for _, a := range list {
		out = append(out, mailAddress(a))
	}
	return out
}
