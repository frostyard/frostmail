package main

import (
	"context"
	"log/slog"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/notify"
)

// announcement is one message.announced for the notifier.
type announcement struct {
	account int64
	notes   []notify.Note
}

// announceQueue is how many announcements may wait for the notifier.
const announceQueue = 64

// newAnnouncer shows maild's notes through frostmail-notify.exe beside the
// app, for a maild behind lodge, which has no desktop to notify on
// (docs/design/desktop.md, Notifications). A click runs the app with
// --open-message, which the running app takes as a second instance, so
// post and main go unused. One goroutine runs the notifier, as a run can
// take seconds and a group note must replace the one before it.
func newAnnouncer(_ func(func()), _ func() *mainwin.Window) func(int64, []api.Announcement) {
	ctx := context.Background()
	log := slog.Default()
	queue := make(chan announcement, announceQueue)
	go func() {
		desktop, err := notify.OpenDesktop(ctx, log, nil)
		if err != nil {
			log.Warn("no notifications", "err", err)
		}
		for a := range queue {
			if desktop == nil {
				continue
			}
			if err := desktop.Show(ctx, a.account, a.notes); err != nil {
				log.Warn("showing a notification", "err", err)
			}
		}
	}()
	return func(account int64, notes []api.Announcement) {
		a := announcement{account: account, notes: make([]notify.Note, 0, len(notes))}
		for _, n := range notes {
			note := notify.Note{Summary: n.Title, Body: n.Body}
			if n.MessageID != nil {
				note.MessageID = *n.MessageID
			}
			a.notes = append(a.notes, note)
		}
		select {
		case queue <- a:
		default:
			log.Warn("dropping a notification: the notifier is behind")
		}
	}
}
