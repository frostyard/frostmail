// Package compose builds outgoing mail: the rules for replies and forwards
// (this file) and the message builder (docs/design/send.md).
package compose

import (
	"html"
	"slices"
	"strings"
	"time"
)

// Address is one mailbox address.
type Address struct {
	Name string
	Addr string
}

// Source is the message a reply or forward starts from.
type Source struct {
	From       Address
	ReplyTo    []Address
	To         []Address
	Cc         []Address
	Subject    string
	MessageID  string   // without angle brackets
	References []string // without angle brackets
	Date       time.Time
	HTML       string // the sanitized rendering; "" for a text-only message
	Text       string
}

// ReplyRecipients returns a reply's To and Cc. To is the source's Reply-To,
// else its From, without the account's own addresses (self); if that leaves
// To empty (a reply to one's own message), To is the source's To without
// self. With all, Cc is the source's To followed by its Cc, without self,
// without addresses already in To and without repeats (the first occurrence
// wins, name included); without all, Cc is nil. Empty results are nil.
// Address comparisons ignore case.
func ReplyRecipients(src Source, self []string, all bool) (to, cc []Address) {
	own := lowerSet(self)
	base := src.ReplyTo
	if len(base) == 0 {
		base = []Address{src.From}
	}
	to = withoutAddrs(base, own)
	if len(to) == 0 {
		to = withoutAddrs(src.To, own)
	}
	if !all {
		return to, nil
	}
	seen := lowerAddrs(to)
	for _, a := range slices.Concat(src.To, src.Cc) {
		key := strings.ToLower(a.Addr)
		if own[key] || seen[key] {
			continue
		}
		seen[key] = true
		cc = append(cc, a)
	}
	return to, cc
}

// ReplySubject is a reply's subject: trimmed, unchanged when it already
// starts (case-insensitively) with "re:", "aw:" or "sv:", else prefixed
// with "Re: ".
func ReplySubject(s string) string {
	s = strings.TrimSpace(s)
	if hasAnyPrefixFold(s, "re:", "aw:", "sv:") {
		return s
	}
	return "Re: " + s
}

// ForwardSubject is a forward's subject: trimmed, unchanged when it already
// starts (case-insensitively) with "fwd:" or "fw:", else prefixed with
// "Fwd: ".
func ForwardSubject(s string) string {
	s = strings.TrimSpace(s)
	if hasAnyPrefixFold(s, "fwd:", "fw:") {
		return s
	}
	return "Fwd: " + s
}

// ReplyReferences is a reply's References: the source's References then its
// Message-ID, skipping empty strings and repeats; when longer than 20, the
// first entry and the last 19. Nil when empty.
func ReplyReferences(src Source) []string {
	var refs []string
	seen := make(map[string]bool, len(src.References)+1)
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		refs = append(refs, id)
	}
	for _, r := range src.References {
		add(r)
	}
	add(src.MessageID)
	if len(refs) > 20 {
		refs = append([]string{refs[0]}, refs[len(refs)-19:]...)
	}
	return refs
}

// Attribution is the line above a quoted message: "On <date> at <time>,
// <name> wrote:", with name "someone" when empty, the date rendered in loc.
func Attribution(date time.Time, name string, loc *time.Location) string {
	if name == "" {
		name = "someone"
	}
	return "On " + date.In(loc).Format("Mon, Jan 2, 2006 at 3:04 PM") + ", " + name + " wrote:"
}

// QuoteHTML is the body of a reply: the attribution line above the quoted
// source inside a cite blockquote. The attribution name is the source's
// From name, else its address.
func QuoteHTML(src Source, loc *time.Location) string {
	name := src.From.Name
	if name == "" {
		name = src.From.Addr
	}
	attr := html.EscapeString(Attribution(src.Date, name, loc))
	return `<p><br></p><p>` + attr + `</p><blockquote type="cite">` + quotedBody(src) + `</blockquote>`
}

// ForwardHTML is the body of a forward: a "Begin forwarded message" line
// above a cite blockquote holding the source's From, Subject, Date and To
// headers, then the quoted body. Every inserted value is HTML-escaped.
func ForwardHTML(src Source, loc *time.Location) string {
	var b strings.Builder
	b.WriteString(`<p><br></p><p>Begin forwarded message:</p><blockquote type="cite"><p>`)
	b.WriteString(`<b>From:</b> ` + html.EscapeString(formatAddress(src.From)))
	b.WriteString(`<br><b>Subject:</b> ` + html.EscapeString(src.Subject))
	b.WriteString(`<br><b>Date:</b> ` + src.Date.In(loc).Format("January 2, 2006 at 3:04 PM"))
	b.WriteString(`<br><b>To:</b> ` + joinEscaped(src.To))
	b.WriteString(`</p>` + quotedBody(src) + `</blockquote>`)
	return b.String()
}

// quotedBody is the quoted source shared by replies and forwards: the
// sanitized HTML as it is when present, else the escaped text with line
// breaks turned into <br>, wrapped in a paragraph.
func quotedBody(src Source) string {
	if src.HTML != "" {
		return src.HTML
	}
	return "<p>" + strings.ReplaceAll(html.EscapeString(src.Text), "\n", "<br>") + "</p>"
}

// formatAddress renders one address as "Name <addr>", or just the address
// when it has no name.
func formatAddress(a Address) string {
	if a.Name == "" {
		return a.Addr
	}
	return a.Name + " <" + a.Addr + ">"
}

// joinEscaped renders the addresses and joins them with ", ", each escaped.
func joinEscaped(addrs []Address) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		parts = append(parts, html.EscapeString(formatAddress(a)))
	}
	return strings.Join(parts, ", ")
}

// withoutAddrs returns the addresses whose (lower-cased) address is not in
// the given set.
func withoutAddrs(addrs []Address, skip map[string]bool) []Address {
	var out []Address
	for _, a := range addrs {
		if skip[strings.ToLower(a.Addr)] {
			continue
		}
		out = append(out, a)
	}
	return out
}

// lowerSet is the lower-cased set of the given addresses.
func lowerSet(addrs []string) map[string]bool {
	set := make(map[string]bool, len(addrs))
	for _, a := range addrs {
		set[strings.ToLower(a)] = true
	}
	return set
}

// lowerAddrs is the lower-cased set of the addresses' address fields.
func lowerAddrs(addrs []Address) map[string]bool {
	set := make(map[string]bool, len(addrs))
	for _, a := range addrs {
		set[strings.ToLower(a.Addr)] = true
	}
	return set
}

// hasAnyPrefixFold reports whether s starts with any prefix, ignoring case.
func hasAnyPrefixFold(s string, prefixes ...string) bool {
	low := strings.ToLower(s)
	for _, p := range prefixes {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	return false
}
