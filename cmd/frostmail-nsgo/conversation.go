package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
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
		return w.dialog("Conversation", []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(525)}, Children: []core.Node{workspaceui.Stack(articles, core.Dynamic(func(*core.Scope) core.Node {
			n := limit.Get()
			if n >= len(messages) {
				return nil
			}
			return button(fmt.Sprintf("Show %d earlier messages", min(batch, len(messages)-n)), func() { limit.Set(n + batch) })
		}))}}})
	})
}

// conversationArticle is one message of a conversation: a header that
// opens and closes it, and under it the subject, the page in its frame, and
// a way to the message itself.
func (w *workspace) conversationArticle(cx *core.Scope, m api.MessageSummary, initial bool) core.Node {
	open := core.NewSignal(cx, initial)
	colors := ui.ColorsOf(cx)
	name := mail.DisplayName(m.From)
	when := m.Date.Local().Format("Jan 2, 15:04")
	weight := ui.Regular
	if !m.Flags.Seen {
		weight = ui.Semibold
	}
	sender := core.Text{Content: name, MaxLines: 1, Style: ui.TypeBody.Merge(core.Style{FontWeight: weight, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)})}
	header := base.Button{Label: name + " · " + when, Expanded: open.Get, OnPress: func(core.PressEvent) { open.Set(!open.GetUntracked()) },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, CornerRadius: ui.RadiusControl},
		StyleFunc: func() core.Style {
			return core.Style{Hover: &core.Style{Background: colors.Hover()}, FocusVisible: ui.FocusRing(cx, -2)}
		},
		Children: []core.Node{ui.Avatar{Name: name, Size: 28}, sender, figures(cx, when),
			ui.IconOf(func() *core.SVG {
				if open.Get() {
					return lucide.ChevronDown()
				}
				return lucide.ChevronRight()
			}, 14, colors.Muted)}}
	return workspaceui.Card{Children: []core.Node{
		header,
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
					return workspaceui.Banner{Tone: workspaceui.Danger, Icon: lucide.CircleAlert, Text: err}
				}
				body := rendered.Get()
				if body == nil {
					return ui.Skeleton{Lines: 3, Label: "Loading message"}
				}
				var nodes []core.Node
				if body.Remote > 0 {
					nodes = append(nodes, workspaceui.Banner{Icon: lucide.ImageOff, Text: "Remote images are not shown.", Action: "Load images for this message", OnAction: func() { load(true) }})
				}
				nodes = append(nodes, workspaceui.PageFrame{Page: bodyView(w, body), Size: core.Style{Height: core.Pt(270)}})
				return workspaceui.Stack(nodes...)
			}), workspaceui.Row(ui.Button{Text: "Open this message", Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: func() { w.closePanel(); w.openMessage(m.ID, false) }}))
		}},
	}}
}
