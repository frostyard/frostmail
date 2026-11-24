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
	// The kit's panel is opaque: NSGo's own shows the window through it
	// where no blur is drawn, as on Linux.
	return ui.Dialog{Title: title, StyleFunc: workspaceui.DialogPanel(w.cx), Open: func() bool { return true }, OnOpenChange: func(open bool, _ base.Reason) {
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
