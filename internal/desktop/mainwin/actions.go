package mainwin

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// actionTimeout bounds one action on messages.
const actionTimeout = 60 * time.Second

// heldDelete is a delete sent for a message the list shows. Until the list
// drops the row or shows it moved, however long that takes, the row is the
// one the delete was sent for, and another Delete of it would find the
// message in Trash, and erase it.
type heldDelete struct {
	from []int64 // the mailboxes the row showed as the delete went
	// unread says the delete ended, but no read has shown what it did: maild
	// did not answer before the connection on ended, or the read after its
	// answer failed. readBack settles it; until then no view counts.
	unread   bool
	answered bool
	on       *api.Client
	// done says what the delete did is known, from maild's answer or a
	// read: the list's view then is view, and one opened since shows it.
	done bool
	view *live.View
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
	w.runWithin(what, actionTimeout, fn, done)
}

// runWithin is runThen with a deadline of timeout; with 0, the call lasts
// as long as maild takes to answer it, or the connection.
func (w *Window) runWithin(what string, timeout time.Duration, fn func(ctx context.Context, c *api.Client) error, done func(error)) {
	c := w.client
	if c == nil {
		return
	}
	go func() {
		var ctx context.Context
		var cancel context.CancelFunc
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(context.Background(), timeout)
		} else {
			ctx, cancel = untilClosed(c)
		}
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
				title := "Could not " + what
				if errors.Is(err, mail.ErrNotRemembered) {
					title = "Undo cannot take this back" // it was done
				}
				w.toasts = append(w.toasts, ui.Toast{Title: title, Description: err.Error(), Type: "error"})
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
		_, err := undo.Move(ctx, c, ids, func() error {
			for _, p := range params {
				if err := c.Message().Move(ctx, p); err != nil {
					return err
				}
			}
			return nil
		})
		return err
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
	undo, gen, client := w.undo, w.gen, w.client
	var sent, answered bool
	var after []api.MessageSummary
	// No deadline: maild goes on with a call the app stops waiting for, so
	// only its answer, or the end of the connection, which ends the call
	// in maild, tells what the delete did.
	w.runWithin("delete the messages", 0, func(ctx context.Context, c *api.Client) error {
		var err error
		after, err = undo.Move(ctx, c, ids, func() error {
			sent = true
			err := c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: ids})
			var refused *api.Error
			answered = err == nil || errors.As(err, &refused)
			return err
		})
		return err
	}, func(error) {
		if gen == w.gen {
			w.deleteEnded(ids, client, sent, answered, after)
			w.readBack()
		}
	})
}

// untilClosed returns a context that ends with the connection c, or with
// cancel, which also ends the goroutine that watches c.
func untilClosed(c *api.Client) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-c.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
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
			w.deleting[r.ID] = &heldDelete{from: slices.Clone(r.MailboxIDs)}
			out = append(out, r.ID)
		}
	}
	return out
}

// deleteEnded settles the holds of a delete that ended on the connection
// on. What was never sent lets go, and so does a message maild holds,
// after its answer, in the mailboxes its row shows: another Delete acts on
// what the list shows. One it moved or erased is done, and holds until the
// list drops or moves the row. A delete whose answer or read after it did
// not come may have moved the message to Trash: it holds, unread, however
// long, until readBack knows. A revealed message the list has no row for
// will never be dropped, so it goes now.
func (w *Window) deleteEnded(ids []int64, on *api.Client, sent, answered bool, after []api.MessageSummary) {
	now := make(map[int64][]int64, len(after))
	for _, r := range after {
		now[r.ID] = r.MailboxIDs
	}
	for _, id := range ids {
		h := w.deleting[id]
		if h == nil {
			continue
		}
		boxes, there := now[id]
		switch {
		case !sent:
			delete(w.deleting, id)
		case !answered || after == nil:
			h.unread, h.answered, h.on = true, answered, on
		case there && sameBoxes(boxes, h.from):
			delete(w.deleting, id)
		default:
			h.done, h.view = true, w.view
		}
	}
	if r := w.revealing; r != nil && w.deleting[r.ID] != nil && slices.Contains(ids, r.ID) && (w.view == nil || w.view.IndexOf(r.ID) < 0) {
		w.chosen.Remove(r.ID)
		w.revealing = nil
	}
}

// readBack reads the messages of the unread deletes on the window's
// connection, and settles them: one maild moved or erased is done from
// then, and one it holds where its row shows it was not deleted, and lets
// go. A delete maild answered is read on any connection; one it did not
// answer only on a later one, as the end of its connection ended it in
// maild. A read that fails is tried again on the same connection while it
// lasts, and on the next one (takeClient): nothing settles by time.
func (w *Window) readBack() {
	c := w.client
	if c == nil || w.readingBack {
		return
	}
	select {
	case <-c.Done():
		return // the next connection reads
	default:
	}
	var ids []int64
	for id, h := range w.deleting {
		if h.unread && (h.answered || h.on != c) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	slices.Sort(ids)
	w.readingBack = true
	gen, every := w.gen, w.readBackEvery
	go func() {
		ctx, cancel := untilClosed(c)
		defer cancel()
		got, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: ids})
		if err != nil {
			select {
			case <-time.After(every):
			case <-ctx.Done():
			}
		}
		w.o.Post(func() {
			if gen != w.gen {
				return
			}
			w.readingBack = false
			if err == nil {
				w.readBackEnded(ids, got)
			}
			w.readBack() // again, or those that came meanwhile
		})
	}()
}

// readBackEnded settles the unread deletes of ids from what a read found.
func (w *Window) readBackEnded(ids []int64, got []api.MessageSummary) {
	now := make(map[int64][]int64, len(got))
	for _, r := range got {
		now[r.ID] = r.MailboxIDs
	}
	for _, id := range ids {
		h := w.deleting[id]
		if h == nil || !h.unread {
			continue
		}
		h.unread = false
		if boxes, ok := now[id]; ok && sameBoxes(boxes, h.from) {
			delete(w.deleting, id)
			continue
		}
		h.done, h.view = true, w.view
	}
}

// releaseDeletes lets go of the finished deletes whose rows the list has
// dropped or shows in other mailboxes, and of those a view opened since
// shows: that one shows maild's state after the delete.
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
	listed := map[int64][]int64{}
	if w.view != nil {
		for _, r := range w.view.Rows(finished) {
			listed[r.ID] = r.MailboxIDs
		}
	}
	for _, id := range finished {
		h := w.deleting[id]
		boxes, ok := listed[id]
		if !ok || h.view != w.view || !sameBoxes(boxes, h.from) {
			delete(w.deleting, id)
		}
	}
}

// sameBoxes reports whether two lists name the same mailboxes.
func sameBoxes(a, b []int64) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
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
