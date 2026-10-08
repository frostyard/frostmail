// Package compose builds outgoing mail: the rules for replies and forwards
// (this file) and the message builder (docs/design/send.md).
package compose

import "time"

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

// ReplyRecipients returns a reply's To and Cc. Task T-0040 implements the
// functions in this file; the stubs return zero values.
func ReplyRecipients(_ Source, _ []string, _ bool) (to, cc []Address) {
	return nil, nil
}

// ReplySubject is a reply's subject.
func ReplySubject(_ string) string { return "" }

// ForwardSubject is a forward's subject.
func ForwardSubject(_ string) string { return "" }

// ReplyReferences is a reply's References.
func ReplyReferences(_ Source) []string { return nil }

// Attribution is the line above a quoted message.
func Attribution(_ time.Time, _ string, _ *time.Location) string { return "" }

// QuoteHTML is the body of a reply.
func QuoteHTML(_ Source, _ *time.Location) string { return "" }

// ForwardHTML is the body of a forward.
func ForwardHTML(_ Source, _ *time.Location) string { return "" }
