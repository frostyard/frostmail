package main

import (
	"context"
	"fmt"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
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
	return w.ownDialog(title, children, withWayOut(footer, w.closePanel)...)
}

// ownDialog is a dialog whose buttons are its own, with none added: the
// compose window's, which leaves by Discard or by Save and close, and
// must not be left by a button that does neither.
func (w *workspace) ownDialog(title string, children []core.Node, footer ...core.Node) ui.Dialog {
	// The kit's panel is opaque: NSGo's own shows the window through it
	// where no blur is drawn, as on Linux.
	return ui.Dialog{Title: title, StyleFunc: workspaceui.DialogPanel(w.cx), Open: func() bool { return true }, OnOpenChange: func(open bool, _ base.Reason) {
		if !open {
			w.closePanel()
		}
	}, Children: append(children, core.Dynamic(func(*core.Scope) core.Node {
		if err := w.problem.Get(); err != "" {
			return workspaceui.Banner{Tone: workspaceui.Danger, Icon: lucide.CircleAlert, Text: err, Action: "Dismiss", OnAction: func() { w.problem.Set("") }}
		}
		return nil
	})), Footer: footer}
}

// withWayOut gives a dialog's buttons a quiet one that leaves it, first
// (plan 0016, rule 3): Cancel before a button that confirms or destroys,
// and Close where the dialog only shows something. Buttons that already
// hold a Cancel or a Close are left as they are.
func withWayOut(footer []core.Node, leave func()) []core.Node {
	label := "Close"
	for _, node := range footer {
		b, ok := node.(ui.Button)
		if !ok {
			continue
		}
		if b.Text == "Cancel" || b.Text == "Close" {
			return footer
		}
		if b.Variant == ui.ButtonPrimary || b.Variant == ui.ButtonDanger {
			label = "Cancel"
		}
	}
	return append([]core.Node{ui.Button{Text: label, Variant: ui.ButtonSecondary, OnPress: leave}}, footer...)
}

// confirm asks before an action that sends or changes something, and
// destroy before one that removes it: NSGo's alert dialog on the kit's
// panel, Cancel first and quiet, and a destroying action in the danger
// variant (plan 0016, rules 3 and 4). Either takes the place of the panel
// that asked.
func (w *workspace) confirm(title, detail, label string, do func()) {
	w.ask(title, detail, label, ui.ButtonPrimary, do)
}

func (w *workspace) destroy(title, detail, label string, do func()) {
	w.ask(title, detail, label, ui.ButtonDanger, do)
}

func (w *workspace) ask(title, detail, label string, variant ui.ButtonVariant, do func()) {
	w.show(func(cx *core.Scope) core.Node {
		// The alert reports that it closed after its action ran, which
		// may have shown a panel of its own: only a cancelled alert
		// closes what is shown.
		answered := false
		return ui.AlertDialog{Title: title, Description: detail, StyleFunc: workspaceui.DialogPanel(cx), Open: func() bool { return true },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open && !answered {
					w.closePanel()
				}
			},
			Actions: []ui.AlertAction{
				{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
				{Text: label, Variant: variant, Default: true, OnPress: func() { answered = true; w.closePanel(); do() }}}}
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

// moved answers a move or a deletion, which can be undone, with a toast
// whose action undoes it (plan 0016, rule 5).
func (w *workspace) moved(title string) func() {
	return func() {
		undo := w.undo
		w.toaster.Show(ui.ToastContent{ID: "undo", Title: title, Action: "Undo", OnAction: func() {
			w.change(func(ctx context.Context, c *api.Client) error { _, err := undo.Do(ctx, c); return err }, nil)
		}})
	}
}

func (w *workspace) deleteMessage(m api.MessageSummary) {
	undo := w.undo
	ids := []int64{m.ID}
	w.destroy("Delete message?", m.Subject, "Delete", func() {
		w.change(func(ctx context.Context, c *api.Client) error {
			_, err := undo.Move(ctx, c, ids, func() error { return c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: ids}) })
			return err
		}, func() {
			if w.opened.GetUntracked() == m.ID {
				w.generation++
				w.opened.Set(0)
				w.reader.Set(nil)
			}
			w.moved("Message deleted")()
		})
	})
}

func (w *workspace) moveMessage(m api.MessageSummary, b api.Mailbox) func(context.Context, *api.Client) error {
	ids, undo := []int64{m.ID}, w.undo
	p := mail.MoveParams(ids, b, w.source, w.store.State().Mailboxes)
	return func(ctx context.Context, c *api.Client) error {
		_, err := undo.Move(ctx, c, ids, func() error { return c.Message().Move(ctx, p) })
		return err
	}
}
