// Package notify announces new mail on the desktop (docs/design/desktop.md,
// Notifications): the texts (notes.go) and, later, the D-Bus sender. Task
// T-0052 implements Notes; the stub announces nothing.
package notify

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

// Notes turns the new mail of one sync pass into notifications.
func Notes(_ []Mail) []Note { return nil }
