// Package notify announces new mail on the desktop (docs/design/desktop.md,
// Notifications): the texts (notes.go) and, later, the D-Bus sender.
package notify

import (
	"fmt"
	"strings"
)

// Mail is a newly arrived message to announce.
type Mail struct {
	ID       int64
	FromName string
	FromAddr string
	Subject  string
	Preview  string
}

// Note is one desktop notification.
type Note struct {
	Summary   string
	Body      string
	MessageID int64 // the message it opens; 0 for a group
}

// Notes turns the new mail of one sync pass into notifications: one note per
// message for up to three, or a single group note when four or more arrive.
func Notes(mail []Mail) []Note {
	switch n := len(mail); {
	case n == 0:
		return nil
	case n <= 3:
		notes := make([]Note, 0, n)
		for _, m := range mail {
			notes = append(notes, Note{Summary: sender(m), Body: body(m), MessageID: m.ID})
		}
		return notes
	default:
		return []Note{{
			Summary: fmt.Sprintf("%d new messages", n),
			Body:    "From " + joinSenders(mail),
		}}
	}
}

func sender(m Mail) string {
	if name := strings.TrimSpace(m.FromName); name != "" {
		return name
	}
	if addr := strings.TrimSpace(m.FromAddr); addr != "" {
		return addr
	}
	return "Unknown Sender"
}

func body(m Mail) string {
	subject := strings.TrimSpace(m.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	preview := strings.Join(strings.Fields(m.Preview), " ")
	if preview == "" {
		return subject
	}
	if runes := []rune(preview); len(runes) > 120 {
		preview = string(runes[:120]) + "…"
	}
	return subject + "\n" + preview
}

func joinSenders(mail []Mail) string {
	seen := make(map[string]bool, len(mail))
	senders := make([]string, 0, len(mail))
	for _, m := range mail {
		if s := sender(m); !seen[s] {
			seen[s] = true
			senders = append(senders, s)
		}
	}
	switch len(senders) {
	case 1:
		return senders[0]
	case 2:
		return senders[0] + " and " + senders[1]
	case 3:
		return senders[0] + ", " + senders[1] + " and " + senders[2]
	default:
		return fmt.Sprintf("%s, %s, %s and %d others",
			senders[0], senders[1], senders[2], len(senders)-3)
	}
}
