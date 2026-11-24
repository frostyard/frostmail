package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/nsgo-dev/nsgo/base"
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

func (w *workspace) dragRow(cx *core.Scope, m api.MessageSummary, button base.Button) core.Node {
	var copy bool
	button.OnPressDown = func(e core.PressEvent) { copy = e.Modifiers.Alt || e.Modifiers.Control }
	source := dragdrop.Source{View: core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), Width: core.Pct(100)}, Label: "Drag " + m.Subject, Children: []core.Node{button}}, Data: func() any {
		ids := []int64{m.ID}
		if w.selecting.GetUntracked() && w.selection.Has(m.ID) {
			ids = nil
			for _, key := range w.selection.Keys() {
				if id, ok := key.(int64); ok {
					ids = append(ids, id)
				}
			}
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

func (w *workspace) mailboxTarget(i mail.Item, label, account string, b api.Mailbox) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		over := core.NewSignal(cx, false)
		colors := ui.ColorsOf(cx)
		return dragdrop.Target{View: core.View{Label: "File messages in " + b.Path + " · " + account, StyleFunc: func() core.Style {
			s := core.Style{Width: core.Pct(100), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 7, Padding: core.Edges{Left: core.Points(i.Depth * 12)}, CornerRadius: 5}
			if over.Get() {
				s.Background = ui.Alpha(colors.Accent(), .25)
			}
			return s
		}, Children: []core.Node{ui.Icon(lucide.Folder, 16, colors.Accent), core.Text{Content: label, MaxLines: 1, Style: ui.TypeLabel.Merge(core.Style{FlexShrink: 1})}}},
			Accept: func(v any) bool {
				d, ok := v.(draggedMail)
				return ok && d.client != nil && w.session.Client() == d.client && d.account == b.AccountID && w.writable(b.AccountID)
			},
			OnEnter: func(dragdrop.Event) { over.Set(true) }, OnLeave: func(dragdrop.Event) { over.Set(false) }, OnDrop: func(e dragdrop.Event) { over.Set(false); w.dropMail(e.Data.(draggedMail), b) }}
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
	}, func() { w.selection.Clear() })
}
