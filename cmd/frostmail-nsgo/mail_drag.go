package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base/dragdrop"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type draggedMail struct {
	ids     []int64
	account int64
	copy    bool
	source  mail.Source
	client  *api.Client
}

func (w *workspace) dragRow(cx *core.Scope, m api.MessageSummary, content core.Node) core.Node {
	var copy bool
	source := dragdrop.Source{View: core.View{Style: fillRow, Label: "Drag " + m.Subject, OnPressDown: func(e core.PressEvent) { copy = e.Modifiers.Alt }, Children: []core.Node{content}}, Data: func() any {
		// A chosen row carries every chosen message; another row, itself.
		ids := []int64{m.ID}
		if w.selection.Has(m.ID) {
			ids = w.selectedIDs()
		}
		return draggedMail{ids: ids, account: m.AccountID, copy: copy, source: w.source, client: w.session.Client()}
	}, Preview: func(value any) core.Node {
		d := value.(draggedMail)
		action := "Move"
		if d.copy {
			action = "Copy"
		}
		return ui.Button{Text: fmt.Sprintf("%s · %d messages", action, len(d.ids)), Variant: ui.ButtonSecondary}
	}}
	if ui.Touch(cx) {
		source.Activation = dragdrop.AfterLongPress
	}
	return source
}

func (w *workspace) mailboxTarget(label core.Node, account string, b api.Mailbox) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		over := core.NewSignal(cx, false)
		colors := ui.ColorsOf(cx)
		return dragdrop.Target{View: core.View{Label: "File messages in " + b.Path + " · " + account, StyleFunc: func() core.Style {
			s := core.Style{Width: core.Pct(100), Direction: core.Row, AlignItems: core.AlignCenter, CornerRadius: ui.RadiusSmall}
			if over.Get() {
				s.Background = ui.Alpha(colors.Accent(), .25)
			}
			return s
		}, Children: []core.Node{label}},
			Accept: func(v any) bool {
				d, ok := v.(draggedMail)
				return ok && d.client != nil && w.session.Client() == d.client && d.account == b.AccountID && w.writable(b.AccountID)
			},
			OnEnter: func(dragdrop.Event) { over.Set(true) }, OnLeave: func(dragdrop.Event) { over.Set(false) }, OnDrop: func(e dragdrop.Event) {
				over.Set(false)
				// The release that drops also presses this row in a GTK
				// window; navigate lets that one press go by.
				w.dropping = true
				w.cx.Post(func() { w.dropping = false })
				w.dropMail(e.Data.(draggedMail), b)
			}}
	})
}

func (w *workspace) dropMail(d draggedMail, box api.Mailbox) {
	if len(d.ids) == 0 || d.client != w.session.Client() {
		return
	}
	boxes, undo := w.store.State().Mailboxes, w.undo
	w.change(func(ctx context.Context, c *api.Client) error {
		rows, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: d.ids})
		if err != nil {
			return err
		}
		if len(rows) != len(d.ids) {
			return fmt.Errorf("The messages changed during the drag")
		}
		for _, r := range rows {
			targets := mail.MoveTargets(&r, boxes)
			if d.copy {
				targets = mail.CopyTargets(&r, boxes)
			}
			if !slices.ContainsFunc(targets, func(b api.Mailbox) bool { return b.ID == box.ID }) {
				return fmt.Errorf("This mailbox cannot take all the selected messages")
			}
		}
		if d.copy {
			return c.Message().Copy(ctx, &api.MessageCopyParams{IDs: d.ids, MailboxID: box.ID})
		}
		_, err = undo.Move(ctx, c, d.ids, func() error { return c.Message().Move(ctx, mail.MoveParams(d.ids, box, d.source, boxes)) })
		return err
	}, func() {
		w.selection.Clear()
		if !d.copy {
			w.moved(fmt.Sprintf("%d moved to %s", len(d.ids), box.Name))()
		}
	})
}

// rowWithActions adds a row's quick actions, over the row's end while the
// pointer or the focus is on the row, and the same commands as its context
// menu. The row's own hover state shows them, as NSGo's sidebar shows a
// title's arrow, so no pointer event is needed. They stand outside the drag
// source, so a press on one never starts a move. Touch users have the same
// commands in the toolbar.
func (w *workspace) rowWithActions(cx *core.Scope, m api.MessageSummary, row core.Node) core.Node {
	if ui.Touch(cx) || !w.writable(m.AccountID) {
		return row
	}
	flag := !m.Flags.Flagged
	flagLabel := "Flag"
	if !flag {
		flagLabel = "Unflag"
	}
	undo := w.undo
	var actions []ui.IconButton
	actions = append(actions, ui.IconButton{Label: flagLabel, Icon: lucide.Flag, Selected: func() bool { return m.Flags.Flagged }, OnPress: func() {
		w.change(func(ctx context.Context, c *api.Client) error {
			return undo.Flags(ctx, c, []int64{m.ID}, api.FlagChanges{Flagged: &flag})
		}, nil)
	}})
	if box := mail.ArchiveOf(m.AccountID, w.store.State().Mailboxes); box != nil {
		actions = append(actions, ui.IconButton{Label: "Archive", Icon: lucide.Archive, OnPress: func() { w.change(w.moveMessage(m, *box), w.moved("Message archived")) }})
	}
	actions = append(actions, ui.IconButton{Label: "Delete message", Icon: lucide.Trash, OnPress: func() { w.deleteMessage(m) }})
	menu := []core.Node{ui.MenuItem{Text: "Reply", OnSelect: func() { w.newDraft(api.DraftKindReply, &m.ID) }}}
	var buttons []core.Node
	for _, a := range actions {
		menu = append(menu, ui.MenuItem{Text: a.Label, Disabled: w.acting.Get, OnSelect: a.OnPress})
		a.Disabled = w.acting.Get
		a.Size = 24
		buttons = append(buttons, a)
	}
	kit := workspaceui.ColorsOf(cx)
	shown := &core.Style{Opacity: core.Alpha(1)}
	quick := core.View{TestID: "row-actions", Style: core.Style{Position: core.Absolute, Top: core.Pt(4), Right: core.Pt(4), Direction: core.Row, Gap: 2,
		CornerRadius: ui.RadiusControl, Opacity: core.Alpha(0),
		GroupStyle: &core.GroupStyle{Name: rowGroup, Hover: shown, FocusWithin: shown}},
		StyleFunc: func() core.Style { return core.Style{Background: kit.Panel()} }, Children: buttons}
	// The menu's trigger is a column in the list's row, so what it holds
	// takes its width and its own height: with a flex basis of 0 there, as
	// it had, it was 0 high and its content stood outside it, where no
	// pointer was over it (frostyard/frostmail#391).
	content := core.View{Style: core.Style{Width: core.Pct(100), Direction: core.Row}, Children: []core.Node{row, quick}}
	return ui.ContextMenu{Style: fillRow, Trigger: content, Children: menu}
}

// fillRow has a view take the room of the row it is in.
var fillRow = core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0)}

// rowGroup names a row of the list to its quick actions' styles. The group
// is the list's own row, which has the row's hover fill too.
const rowGroup = "message-row"
