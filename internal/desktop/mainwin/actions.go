package mainwin

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// actionTimeout bounds one action on messages.
const actionTimeout = 60 * time.Second

// selectedIDs are the chosen messages, without rows not loaded yet.
func (w *Window) selectedIDs() []int64 {
	var out []int64
	for id := range w.chosen.All() {
		if id > 0 {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

func (w *Window) rows(ids []int64) []api.MessageSummary {
	if w.view == nil {
		return nil
	}
	return w.view.Rows(ids)
}

// run does an action on maild off the UI thread; a failure shows as a
// toast.
func (w *Window) run(what string, fn func(ctx context.Context, c *api.Client) error) {
	c := w.client
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		if err := fn(ctx, c); err != nil {
			slog.Warn(what, "err", err)
			w.o.Post(func() {
				w.toasts = append(w.toasts, ui.Toast{Title: "Could not " + what, Description: err.Error(), Type: "error"})
			})
		}
	}()
}

func (w *Window) move(ids []int64, to *api.Mailbox) {
	if to == nil || len(ids) == 0 {
		return
	}
	params := mail.MoveParams(ids, *to, w.q.Source, w.mail.State().Mailboxes)
	w.run("move the messages", func(ctx context.Context, c *api.Client) error {
		return w.undo.Move(ctx, c, ids, func() error { return c.Message().Move(ctx, params) })
	})
}

func (w *Window) copy(ids []int64, to *api.Mailbox) {
	if to == nil || len(ids) == 0 {
		return
	}
	w.run("copy the messages", func(ctx context.Context, c *api.Client) error {
		return c.Message().Copy(ctx, &api.MessageCopyParams{IDs: ids, MailboxID: to.ID})
	})
}

func (w *Window) delete(ids []int64) {
	if len(ids) == 0 {
		return
	}
	w.run("delete the messages", func(ctx context.Context, c *api.Client) error {
		return w.undo.Move(ctx, c, ids, func() error { return c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: ids}) })
	})
}

func (w *Window) archive(ids []int64) {
	rows := w.rows(ids)
	if len(rows) > 0 {
		w.move(ids, mail.ArchiveOf(rows[0].AccountID, w.mail.State().Mailboxes))
	}
}

func (w *Window) toggleSpam(ids []int64) {
	_, to := mail.SpamTarget(w.rows(ids), w.mail.State().Mailboxes)
	w.move(ids, to)
}

func (w *Window) setFlags(ids []int64, ch api.FlagChanges) {
	if len(ids) == 0 {
		return
	}
	w.run("change the messages", func(ctx context.Context, c *api.Client) error { return w.undo.Flags(ctx, c, ids, ch) })
}

// toggleRead marks the messages seen, or unseen when all already are.
func (w *Window) toggleRead(ids []int64) {
	rows := w.rows(ids)
	allSeen := len(rows) > 0 && !slices.ContainsFunc(rows, func(r api.MessageSummary) bool { return !r.Flags.Seen })
	seen := !allSeen
	w.setFlags(ids, api.FlagChanges{Seen: &seen})
}

// toggleFlag clears the flag when the first message is flagged, else flags
// red.
func (w *Window) toggleFlag(ids []int64) {
	rows := w.rows(ids)
	color := int64(1)
	if len(rows) > 0 && rows[0].Flags.Flagged {
		color = 0
	}
	w.setFlags(ids, api.FlagChanges{FlagColor: &color})
}

func (w *Window) setFlagColor(ids []int64, color int64) {
	w.setFlags(ids, api.FlagChanges{FlagColor: &color})
}

func (w *Window) runUndo() {
	w.run("undo", func(ctx context.Context, c *api.Client) error {
		_, err := w.undo.Do(ctx, c)
		return err
	})
}

func (w *Window) mute(ids []int64, muted bool) {
	w.run("mute the conversation", func(ctx context.Context, c *api.Client) error {
		return c.Message().Mute(ctx, &api.MessageMuteParams{IDs: ids, Muted: muted})
	})
}

func (w *Window) remind(ids []int64, at *time.Time) {
	w.run("set the reminder", func(ctx context.Context, c *api.Client) error {
		return c.Message().Remind(ctx, &api.MessageRemindParams{IDs: ids, At: at})
	})
}

func (w *Window) applyRules(ids []int64) {
	w.run("apply the rules", func(ctx context.Context, c *api.Client) error {
		_, err := c.Rule().Apply(ctx, &api.RuleApplyParams{IDs: ids})
		return err
	})
}

// escape clears the search, else the selection.
func (w *Window) escape() {
	if w.q.Search != "" || w.searchDraft != "" {
		w.q.Search, w.searchDraft = "", ""
		w.clearSelection()
		w.syncView()
		return
	}
	w.clearSelection()
}
