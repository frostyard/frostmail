//go:build !windows

package main

import (
	"strconv"

	"github.com/egoist/mygo"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
)

// noteClicked brings the window forward and shows the message, as a click
// on a notification on Windows does through a second instance.
func noteClicked(post func(func()), main func() *mainwin.Window, message int64) func() {
	return func() {
		post(func() {
			w := main()
			w.Raise()
			w.OpenMessage(message)
		})
	}
}

// newAnnouncer shows maild's notes as notifications, for a maild behind
// lodge, which has no desktop to notify on; a click shows the message.
func newAnnouncer(post func(func()), main func() *mainwin.Window) func(int64, []api.Announcement) {
	return func(_ int64, notes []api.Announcement) {
		for _, note := range notes {
			id := ""
			if note.MessageID != nil {
				id = "message-" + strconv.FormatInt(*note.MessageID, 10)
			}
			n := mygo.NewNotification(mygo.NotificationOptions{ID: id, Title: note.Title, Body: note.Body})
			if note.MessageID != nil {
				n.OnClick(noteClicked(post, main, *note.MessageID))
			}
			_ = n.Show()
		}
	}
}
