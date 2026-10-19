package notify

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// Broadcast announces new mail to the connected clients instead of the
// desktop, for a maild with no session bus (docs/specs/lodge-app.md, The
// web build): one transient message.announced event carrying the notes
// Notes makes, which each client shows itself. It returns an announcer
// for mailsync.Config.Announce.
func Broadcast(publish func([]api.EventEnvelope)) func(ctx context.Context, accountID int64, mail []Mail) error {
	return func(_ context.Context, accountID int64, mail []Mail) error {
		notes := Notes(mail)
		if len(notes) == 0 {
			return nil
		}
		ev := api.MessageAnnounced{AccountID: accountID, Notes: make([]api.Announcement, 0, len(notes))}
		for _, n := range notes {
			a := api.Announcement{Title: n.Summary, Body: n.Body}
			if n.MessageID != 0 {
				a.MessageID = &n.MessageID
			}
			ev.Notes = append(ev.Notes, a)
		}
		env, err := api.NewEnvelope(0, ev)
		if err != nil {
			return err
		}
		publish([]api.EventEnvelope{env})
		return nil
	}
}
