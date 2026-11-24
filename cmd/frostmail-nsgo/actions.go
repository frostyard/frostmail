package main

import (
	"context"
	"fmt"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type panel struct{ render func(*core.Scope) core.Node }

func (w *workspace) show(render func(*core.Scope) core.Node) { w.overlay.Set(&panel{render}) }
func (w *workspace) closePanel()                             { w.overlay.Set(nil) }
func (w *workspace) dialog(title string, children []core.Node, footer ...core.Node) ui.Dialog {
	return ui.Dialog{Title: title, Open: func() bool { return true }, OnOpenChange: func(open bool, _ base.Reason) {
		if !open {
			w.closePanel()
		}
	}, Children: children, Footer: footer}
}

func (w *workspace) confirm(title, detail, label string, do func()) {
	w.show(func(*core.Scope) core.Node {
		d := w.dialog(title, []core.Node{core.Text{Content: detail}}, ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: w.closePanel}, ui.Button{Text: label, OnPress: func() { w.closePanel(); do() }})
		return d
	})
}

// perform delivers a result only to the session that issued the operation.
func perform[T any](w *workspace, job func(context.Context, *api.Client) (T, error), done func(T, error)) {
	c := w.session.Client()
	if c == nil {
		var zero T
		done(zero, fmt.Errorf("maild is disconnected"))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(w.ctx, 60*time.Second)
		defer cancel()
		v, err := job(ctx, c)
		w.cx.Post(func() {
			if w.session.Client() != c && err == nil {
				err = fmt.Errorf("maild reconnected during the operation")
			}
			done(v, err)
		})
	}()
}

func (w *workspace) change(job func(context.Context, *api.Client) error, done func()) {
	if w.acting.GetUntracked() {
		return
	}
	w.acting.Set(true)
	perform(w, func(ctx context.Context, c *api.Client) (struct{}, error) { return struct{}{}, job(ctx, c) }, func(_ struct{}, err error) {
		w.acting.Set(false)
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		if done != nil {
			done()
		}
	})
}

func (w *workspace) triage(m api.MessageSummary) core.Node {
	ids := []int64{m.ID}
	undo := w.undo
	action := func(title string, fn func(context.Context, *api.Client) error) ui.MenuItem {
		return ui.MenuItem{Text: title, Disabled: w.acting.Get, OnSelect: func() { w.change(fn, nil) }}
	}
	flags := func(label string, ch api.FlagChanges) ui.MenuItem {
		return ui.MenuItem{Text: label, OnSelect: func() {
			w.change(func(ctx context.Context, c *api.Client) error { return undo.Flags(ctx, c, ids, ch) }, func() {
				p := w.reader.GetUntracked()
				if p == nil || p.message.Summary.ID != m.ID {
					return
				}
				next := *p
				message := *p.message
				next.message = &message
				if ch.Seen != nil {
					message.Summary.Flags.Seen = *ch.Seen
				}
				if ch.Flagged != nil {
					message.Summary.Flags.Flagged = *ch.Flagged
				}
				w.reader.Set(&next)
			})
		}}
	}
	boxes := w.store.State().Mailboxes
	source := w.source
	move := func(box api.Mailbox) func(context.Context, *api.Client) error {
		return func(ctx context.Context, c *api.Client) error {
			_, err := undo.Move(ctx, c, ids, func() error { return c.Message().Move(ctx, mail.MoveParams(ids, box, source, boxes)) })
			return err
		}
	}
	seen, flag := !m.Flags.Seen, !m.Flags.Flagged
	readLabel := "Mark as unread"
	if seen {
		readLabel = "Mark as read"
	}
	flagLabel := "Flag"
	if !flag {
		flagLabel = "Unflag"
	}
	items := []core.Node{flags(readLabel, api.FlagChanges{Seen: &seen}), flags(flagLabel, api.FlagChanges{Flagged: &flag})}
	if box := mail.ArchiveOf(m.AccountID, boxes); box != nil {
		items = append(items, action("Archive", move(*box)))
	}
	if unjunk, box := mail.SpamTarget([]api.MessageSummary{m}, boxes); box != nil {
		label := "Move to junk"
		if unjunk {
			label = "Move to inbox"
		}
		items = append(items, action(label, move(*box)))
	}
	var moves, copies []core.Node
	for _, box := range mail.MoveTargets(&m, boxes) {
		moves = append(moves, action(box.Name, move(box)))
	}
	for _, box := range mail.CopyTargets(&m, boxes) {
		copies = append(copies, action(box.Name, func(ctx context.Context, c *api.Client) error {
			return c.Message().Copy(ctx, &api.MessageCopyParams{IDs: ids, MailboxID: box.ID})
		}))
	}
	if len(moves) > 0 {
		items = append(items, ui.Submenu{Text: "Move to", Children: moves})
	}
	if len(copies) > 0 {
		items = append(items, ui.Submenu{Text: "Copy to", Children: copies})
	}
	muted := m.Muted == nil || !*m.Muted
	muteLabel := "Mute conversation"
	if !muted {
		muteLabel = "Unmute conversation"
	}
	items = append(items, action(muteLabel, func(ctx context.Context, c *api.Client) error {
		return c.Message().Mute(ctx, &api.MessageMuteParams{IDs: ids, Muted: muted})
	}), action("Remind me tomorrow", func(ctx context.Context, c *api.Client) error {
		at := time.Now().AddDate(0, 0, 1)
		return c.Message().Remind(ctx, &api.MessageRemindParams{IDs: ids, At: &at})
	}))
	items = append(items, ui.MenuItem{Text: "Delete message", OnSelect: func() {
		w.confirm("Delete message?", m.Subject, "Delete", func() {
			w.change(func(ctx context.Context, c *api.Client) error {
				_, err := undo.Move(ctx, c, ids, func() error { return c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: ids}) })
				return err
			}, func() {
				if w.opened.GetUntracked() == m.ID {
					w.generation++
					w.opened.Set(0)
					w.reader.Set(nil)
				}
			})
		})
	}})
	return workspaceui.Row(ui.Button{Text: "Reply", Variant: ui.ButtonSecondary, OnPress: func() { w.newDraft(api.DraftKindReply, &m.ID) }}, ui.Menu{Trigger: ui.Button{Text: "Forward or reply all", Variant: ui.ButtonGhost}, Children: []core.Node{ui.MenuItem{Text: "Reply all", OnSelect: func() { w.newDraft(api.DraftKindReplyall, &m.ID) }}, ui.MenuItem{Text: "Forward", OnSelect: func() { w.newDraft(api.DraftKindForward, &m.ID) }}, ui.MenuItem{Text: "Forward as attachment", OnSelect: func() { w.newDraft(api.DraftKindAttached, &m.ID) }}}}, ui.Menu{Trigger: ui.Button{Text: "Message actions", Variant: ui.ButtonSecondary}, Children: items}, ui.Button{Text: "Undo", Variant: ui.ButtonGhost, OnPress: func() {
		w.change(func(ctx context.Context, c *api.Client) error { _, err := undo.Do(ctx, c); return err }, nil)
	}})
}
