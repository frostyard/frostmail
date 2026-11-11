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

// deleteHold is how long a finished delete stays held while the view goes
// on listing its row, as one that also shows Trash does.
const deleteHold = 10 * time.Second

// heldDelete is a delete sent for a message the view still lists.
type heldDelete struct {
	done bool
	at   time.Time // when it finished
}

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
	if r := w.revealing; r != nil && len(ids) == 1 && ids[0] == r.ID {
		return []api.MessageSummary{*r}
	}
	if w.view == nil {
		return nil
	}
	return w.view.Rows(ids)
}

// run does an action on maild off the UI thread; a failure shows as a
// toast.
func (w *Window) run(what string, fn func(ctx context.Context, c *api.Client) error) {
	w.runThen(what, fn, nil)
}

// runThen is run, and tells done how it ended, on the UI thread.
func (w *Window) runThen(what string, fn func(ctx context.Context, c *api.Client) error, done func(error)) {
	c := w.client
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		err := fn(ctx, c)
		if err != nil {
			slog.Warn(what, "err", err)
		}
		if err == nil && done == nil {
			return
		}
		w.o.Post(func() {
			if err != nil {
				w.toasts = append(w.toasts, ui.Toast{Title: "Could not " + what, Description: err.Error(), Type: "error"})
			}
			if done != nil {
				done(err)
			}
		})
	}()
}

// bound returns fn to run only while the window is on the maild it is on
// now: what was asked of one maild must not reach another.
func (w *Window) bound(fn func()) func() {
	gen := w.gen
	return func() {
		if gen == w.gen {
			fn()
		}
	}
}

func (w *Window) move(ids []int64, to *api.Mailbox) {
	if to == nil || len(ids) == 0 {
		return
	}
	w.moveGroups([]mail.MoveGroup{{IDs: ids, To: *to}})
}

// moveGroups moves each group to its mailbox, as one action to undo.
func (w *Window) moveGroups(groups []mail.MoveGroup) {
	if len(groups) == 0 {
		return
	}
	mailboxes := w.mail.State().Mailboxes
	var ids []int64
	params := make([]*api.MessageMoveParams, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.IDs...)
		params = append(params, mail.MoveParams(g.IDs, g.To, w.q.Source, mailboxes))
	}
	undo := w.undo
	w.run("move the messages", func(ctx context.Context, c *api.Client) error {
		return undo.Move(ctx, c, ids, func() error {
			for _, p := range params {
				if err := c.Message().Move(ctx, p); err != nil {
					return err
				}
			}
			return nil
		})
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

// delete sends the messages to Trash, and erases those already there, so
// it acts only on what the list shows now, once.
func (w *Window) delete(ids []int64) {
	ids = w.holdDeletes(ids)
	if len(ids) == 0 {
		return
	}
	undo, gen := w.undo, w.gen
	w.runThen("delete the messages", func(ctx context.Context, c *api.Client) error {
		return undo.Move(ctx, c, ids, func() error { return c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: ids}) })
	}, func(err error) {
		if gen == w.gen {
			w.deleteEnded(ids, err)
		}
	})
}

// deleteKey is Delete and Backspace, which act in the list alone
// (docs/specs/ui.md, Keyboard map). The pane is the last to hold the
// focus, so the palette, which takes it for a moment, acts as the key does.
func (w *Window) deleteKey() {
	if w.pane == paneList {
		w.delete(w.selectedIDs())
	}
}

// holdDeletes returns the ids to delete now, and holds them: those the list
// has a row for, and no delete sent yet. A second Delete before the first
// shows in the list would find the message in Trash, and erase it.
func (w *Window) holdDeletes(ids []int64) []int64 {
	if w.client == nil {
		return nil
	}
	rows := w.rows(ids)
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		if w.deleting[r.ID] == nil {
			w.deleting[r.ID] = &heldDelete{}
			out = append(out, r.ID)
		}
	}
	return out
}

// deleteEnded lets go of what failed, which can be tried again; the rest
// stays held until the list drops it. A revealed message the list has no
// row for will never be dropped, so it goes now.
func (w *Window) deleteEnded(ids []int64, err error) {
	now := w.o.Now()
	for _, id := range ids {
		if err != nil {
			delete(w.deleting, id)
		} else if h := w.deleting[id]; h != nil {
			h.done, h.at = true, now
		}
	}
	if r := w.revealing; err == nil && r != nil && slices.Contains(ids, r.ID) && (w.view == nil || w.view.IndexOf(r.ID) < 0) {
		w.chosen.Remove(r.ID)
		w.revealing = nil
	}
}

// releaseDeletes lets go of the finished deletes whose rows the list has
// dropped, or that have waited for it long enough.
func (w *Window) releaseDeletes() {
	var finished []int64
	for id, h := range w.deleting {
		if h.done {
			finished = append(finished, id)
		}
	}
	if len(finished) == 0 {
		return
	}
	listed := map[int64]bool{}
	if w.view != nil {
		for _, r := range w.view.Rows(finished) {
			listed[r.ID] = true
		}
	}
	now := w.o.Now()
	for _, id := range finished {
		if !listed[id] || now.Sub(w.deleting[id].at) >= deleteHold {
			delete(w.deleting, id)
		}
	}
}

// writable leaves out the rows of read-only accounts.
func (w *Window) writable(rows []api.MessageSummary) []api.MessageSummary {
	return slices.DeleteFunc(slices.Clone(rows), func(r api.MessageSummary) bool { return w.readOnly(r.AccountID) })
}

func (w *Window) archive(ids []int64) {
	w.moveGroups(mail.ArchiveGroups(w.writable(w.rows(ids)), w.mail.State().Mailboxes))
}

func (w *Window) toggleSpam(ids []int64) {
	_, groups := mail.SpamGroups(w.writable(w.rows(ids)), w.mail.State().Mailboxes)
	w.moveGroups(groups)
}

func (w *Window) setFlags(ids []int64, ch api.FlagChanges) {
	if len(ids) == 0 {
		return
	}
	undo := w.undo
	w.run("change the messages", func(ctx context.Context, c *api.Client) error { return undo.Flags(ctx, c, ids, ch) })
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
	undo := w.undo
	w.run("undo", func(ctx context.Context, c *api.Client) error {
		_, err := undo.Do(ctx, c)
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
