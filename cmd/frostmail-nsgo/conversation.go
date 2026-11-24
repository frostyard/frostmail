package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workspace) conversation(page *messagePage) {
	selected := page.message.Summary
	boxes := w.store.State().Mailboxes
	inTrash := mail.InRole(selected, api.MailboxRoleTrash, boxes)
	messages := slices.DeleteFunc(slices.Clone(page.thread), func(m api.MessageSummary) bool {
		return m.ID != selected.ID && !inTrash && mail.InRole(m, api.MailboxRoleTrash, boxes)
	})
	slices.Reverse(messages)
	w.show(func(cx *core.Scope) core.Node {
		const batch = 20
		limit := core.NewSignal(cx, batch)
		articles := core.Each(func() []api.MessageSummary { return messages[:min(limit.Get(), len(messages))] }, func(m api.MessageSummary) int64 { return m.ID }, func(owner *core.Scope, m func() api.MessageSummary, _ func() int) core.Node {
			message := m()
			return w.conversationArticle(owner, message, message.ID == selected.ID || !message.Flags.Seen)
		})
		return w.dialog("Conversation", []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(525)}, Children: []core.Node{articles, core.Dynamic(func(*core.Scope) core.Node {
			n := limit.Get()
			if n >= len(messages) {
				return nil
			}
			return button(fmt.Sprintf("Show %d earlier messages", min(batch, len(messages)-n)), func() { limit.Set(n + batch) })
		})}}})
	})
}

func (w *workspace) conversationArticle(cx *core.Scope, m api.MessageSummary, initial bool) core.Node {
	open := core.NewSignal(cx, initial)
	label := m.From.Name
	if label == "" {
		label = m.From.Address
	}
	return workspaceui.Card{Children: []core.Node{
		ui.Button{Text: label + " · " + m.Date.Local().Format("Jan 2, 15:04"), Variant: ui.ButtonGhost, OnPress: func() { open.Set(!open.GetUntracked()) }},
		core.Show{When: open.Get, Then: func(scope *core.Scope) core.Node {
			rendered := core.NewSignal(scope, (*api.Rendering)(nil))
			failure := core.NewSignal(scope, "")
			remote := w.session.Target().Remote()
			load := func(images bool) {
				perform(w, func(ctx context.Context, c *api.Client) (*api.Rendering, error) {
					return c.Message().Render(ctx, &api.MessageRenderParams{ID: m.ID, Remote: &images, Content: &remote})
				}, func(body *api.Rendering, err error) {
					scope.Post(func() {
						if err != nil {
							failure.Set(err.Error())
							return
						}
						rendered.Set(body)
						if !m.Flags.Seen && w.writable(m.AccountID) {
							m.Flags.Seen = true
							w.call(func(ctx context.Context, c *api.Client) error {
								seen := true
								return c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{m.ID}, Changes: api.FlagChanges{Seen: &seen}})
							})
						}
					})
				})
			}
			load(false)
			return workspaceui.Stack(core.Text{Content: m.Subject, Style: ui.TypeLabel}, core.Dynamic(func(*core.Scope) core.Node {
				if err := failure.Get(); err != "" {
					return core.Text{Content: err}
				}
				body := rendered.Get()
				if body == nil {
					return core.Text{Content: "Loading message…"}
				}
				nodes := []core.Node{core.View{Style: core.Style{Height: core.Pt(270)}, Children: []core.Node{bodyView(w, body)}}}
				if body.Remote > 0 {
					nodes = append(nodes, button("Load images for this message", func() { load(true) }))
				}
				return workspaceui.Stack(nodes...)
			}), button("Open this message", func() { w.closePanel(); w.openMessage(m.ID, false) }))
		}},
	}}
}
