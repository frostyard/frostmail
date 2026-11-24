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

type panel struct {
	render func(*core.Scope) core.Node
	route  string
}

func (w *workspace) routePanel(route string) {
	if p := w.overlay.GetUntracked(); p != nil {
		w.overlay.Set(&panel{render: p.render, route: route})
	}
}

func (w *workspace) show(render func(*core.Scope) core.Node) { w.overlay.Set(&panel{render: render}) }
func (w *workspace) closePanel()                             { w.overlay.Set(nil) }
func (w *workspace) dialog(title string, children []core.Node, footer ...core.Node) ui.Dialog {
	if len(footer) == 0 {
		footer = []core.Node{ui.Button{Text: "Close", Variant: ui.ButtonSecondary, OnPress: w.closePanel}}
	}
	return ui.Dialog{Title: title, Open: func() bool { return true }, OnOpenChange: func(open bool, _ base.Reason) {
		if !open {
			w.closePanel()
		}
	}, Children: append(children, core.Dynamic(func(*core.Scope) core.Node {
		if err := w.problem.Get(); err != "" {
			return workspaceui.Notice{Kind: ui.ToastWarning, Title: "Mail", Detail: err, OnDismiss: func() { w.problem.Set("") }}
		}
		return nil
	})), Footer: footer}
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
		return action(label, func(ctx context.Context, c *api.Client) error { return undo.Flags(ctx, c, ids, ch) })
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
	var colors []core.Node
	var names []string
	if st := w.store.State().Settings; st != nil {
		names = st.FlagNames
	}
	for i := int64(1); i <= 7; i++ {
		yes := true
		colors = append(colors, flags(mail.FlagLabel(int(i), names), api.FlagChanges{Flagged: &yes, FlagColor: &i}))
	}
	items = append(items, ui.Submenu{Text: "Flag color", Children: colors})
	if mail.InRole(m, api.MailboxRoleDrafts, boxes) {
		items = append(items, ui.MenuItem{Text: "Edit draft", OnSelect: func() {
			perform(w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
				return c.Draft().Open(ctx, &api.DraftOpenParams{MessageID: m.ID})
			}, w.editDraft)
		}})
	}
	items = append(items, w.moreMessageActions(m)...)
	items = append(items, w.readerTools(m.ID)...)
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
	}), w.remindMenu(m))
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
