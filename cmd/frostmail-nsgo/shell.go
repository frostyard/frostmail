package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The main window's shell (plan 0016, Phase 3a): the module tabs, the
// navigation's rows and foot, and the toolbar. Each is NSGo's control for
// its job, by the kit's control map.

// tier is how much of the toolbar fits beside the title.
type tier int

const (
	// tierMenu keeps Compose and one menu: a window under 720 wide.
	tierMenu tier = iota
	// tierSmall has the toolbar, with Compose and the palette as icons.
	tierSmall
	// tierAligned gives the title and Compose, with its name, the list's
	// width, so the toolbar starts over the reader.
	tierAligned
	// tierFull adds the palette's field.
	tierFull
)

// The room the toolbar's parts take beside the list segment: the message
// toolbar with the palette's and the To-Do Bar's buttons, and the same
// with the palette's field.
const (
	toolbarRoom core.Points = 410
	fieldRoom   core.Points = 620
)

// toolbarTier follows the width of the pane under the toolbar, which the
// navigation and the To-Do Bar take from, and the list's width.
func (w *workspace) toolbarTier() tier {
	window := w.cx.Env().Window().Width
	room := w.contentWidth.Get()
	if room <= 0 {
		// Before the first layout: the window less the navigation.
		room = window - 256
	}
	list := w.listWidth.Get()
	switch {
	case window < 720 || room < 520:
		return tierMenu
	case window < 800 || room < list+toolbarRoom:
		return tierSmall
	case room < list+fieldRoom:
		return tierAligned
	}
	return tierFull
}

// modules are the app's modules that the tabs at the navigation's head
// choose. Settings and Activity are modules too, reached from its foot.
var modules = []struct {
	name string
	icon func() *core.SVG
}{{"Mail", lucide.Mail}, {"Calendar", lucide.Calendar}, {"People", lucide.Users}, {"Tasks", lucide.ListChecks}}

// moduleTabs is the module bar: a segment of icons, each named by its
// tooltip and to assistive technology.
func (w *workspace) moduleTabs() core.Node {
	var options []ui.Option[string]
	for _, m := range modules {
		options = append(options, ui.Option[string]{Value: m.name, Label: m.name, Content: core.Dynamic(func(cx *core.Scope) core.Node {
			return ui.Tooltip{Text: m.name, Trigger: ui.Icon(m.icon, 16, ui.ColorsOf(cx).Text)}
		})})
	}
	return core.View{Style: core.Style{Padding: core.Symmetric(4, 8)}, Children: []core.Node{
		ui.Tabs[string]{Variant: ui.TabsSegmented, Equal: true, Label: "Module", Value: w.module.Get,
			OnChange: func(m string) { w.navigate("module:" + m) }, Options: options}}}
}

// navIcons maps the sidebar model's icon names, which are freedesktop's,
// to the icons of a mailbox's role.
var navIcons = map[string]func() *core.SVG{
	"mail-read": lucide.Inbox, "document-edit": lucide.FilePen, "mail-send": lucide.Send,
	"dialog-warning": lucide.ShieldAlert, "user-trash": lucide.Trash, "mail-archive": lucide.Archive,
	"mail-mark-important": lucide.Flag, "folder-saved-search": lucide.FolderSearch,
	"starred": lucide.Star, "avatar-default": lucide.User,
}

// navLabel is a row of the navigation: the icon of the row's role, its
// name, and its count at the row's end. The model counts unread messages,
// but all of them for Flagged and its colors, which shows muted.
func navLabel(i mail.Item) core.Node {
	icon := navIcons[i.Icon]
	if icon == nil {
		icon = lucide.Folder
	}
	all := i.Key == "flagged" || strings.HasPrefix(i.Key, "flag:")
	label := workspaceui.NavLabel{Icon: icon, Text: i.Label, Count: int(i.Unread), Unread: !all}
	if i.Depth == 0 {
		return label
	}
	return core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0),
		Padding: core.Edges{Left: core.Points(i.Depth * 12)}}, Children: []core.Node{label}}
}

// sidebarModel is the navigation's sections of mail, from what the store
// holds.
func (w *workspace) sidebarModel() []mail.Section {
	w.state.Get()
	st := w.store.State()
	extras := mail.Extras{VIPs: st.VIPs, Counts: st.Counts, Smart: st.Smart, HasSmart: true, SmartCounts: st.SmartCounts}
	if st.Settings != nil {
		extras.FlagNames = st.Settings.FlagNames
		for _, id := range st.Settings.Favorites {
			for _, box := range st.Mailboxes {
				if box.ID == id {
					extras.Favorites = append(extras.Favorites, box)
					break
				}
			}
		}
	}
	return mail.BuildSidebar(st.Accounts, st.Mailboxes, extras)
}

func (w *workspace) sections() []ui.SidebarSection {
	model := w.sidebarModel()
	st := w.store.State()
	boxes := make(map[int64]api.Mailbox, len(st.Mailboxes))
	for _, b := range st.Mailboxes {
		boxes[b.ID] = b
	}
	var out []ui.SidebarSection
	for _, s := range model {
		section := ui.SidebarSection{Title: s.Title, Collapsible: true}
		for _, i := range s.Items {
			if !i.Selectable {
				continue
			}
			item := ui.SidebarItem{ID: i.Key, TestID: "mailbox-" + i.Key, Text: i.Label, Content: navLabel(i)}
			if b, ok := boxes[i.MailboxID]; ok {
				item.Content = w.mailboxTarget(navLabel(i), s.Title, b)
				for _, a := range st.Accounts {
					if a.ID == b.AccountID {
						item.Menu = []core.Node{ui.MenuItem{Text: "Mailbox options", OnSelect: func() { w.mailboxMenu(a, b) }}}
						break
					}
				}
			}
			section.Items = append(section.Items, item)
		}
		// A section with no row, as Smart Mailboxes before the first one
		// is made, would be a caption over nothing.
		if len(section.Items) > 0 {
			out = append(out, section)
		}
	}
	if follow := st.FollowUp; follow != nil && len(follow.Threads) > 0 {
		section := ui.SidebarSection{Title: "Follow up", Collapsible: true}
		for _, t := range follow.Threads {
			section.Items = append(section.Items, ui.SidebarItem{ID: fmt.Sprintf("follow:%d", t.Message.ID), Text: t.Message.Subject,
				Content: workspaceui.NavLabel{Icon: lucide.Clock, Text: t.Message.Subject}})
		}
		out = append(out, section)
	}
	// Mail on its way out is kept by maild, not in a mailbox: these rows
	// open its lists and choose no source.
	return append(out, ui.SidebarSection{Title: "Outgoing", Collapsible: true, Items: []ui.SidebarItem{
		{ID: panelDrafts, TestID: "local-drafts", Text: "Local Drafts", Content: workspaceui.NavLabel{Icon: lucide.SquarePen, Text: "Local Drafts"}},
		{ID: panelOutbox, TestID: "outbox", Text: "Outbox", Content: workspaceui.NavLabel{Icon: lucide.SendHorizontal, Text: "Outbox", Count: len(st.Outbox)}},
	}})
}

// The navigation's rows that open a panel in place of a source.
const (
	panelDrafts = "panel:drafts"
	panelOutbox = "panel:outbox"
)

// subtitle is the unread count of the source shown, under its title.
func (w *workspace) subtitle() string {
	if w.module.Get() != "Mail" {
		return ""
	}
	key := w.selected.Get()
	for _, s := range w.sidebarModel() {
		for _, i := range s.Items {
			if i.Key == key && i.Unread > 0 && i.Key != "flagged" && !strings.HasPrefix(i.Key, "flag:") {
				return fmt.Sprintf("%d unread", i.Unread)
			}
		}
	}
	return ""
}

// titleBlock is the toolbar's title: the module or the source shown, and a
// source's unread count under it.
func (w *workspace) titleBlock() core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			core.Text{ContentFunc: w.title, MaxLines: 1, Style: ui.TypeTitle},
			core.Show{When: func() bool { return w.subtitle() != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{ContentFunc: w.subtitle, MaxLines: 1, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)}
			}}}}
	})
}

// bar is a row of a toolbar's parts, which never wraps.
func bar(children ...core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: children}
}

// header is the toolbar's list segment: the title and, in Mail, Compose,
// the window's one filled button. With room for it the segment is as wide
// as the list under it.
func (w *workspace) header() core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		fit := w.tier.Get()
		if w.module.Get() != "Mail" || fit == tierMenu {
			return w.titleBlock()
		}
		row := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, FlexShrink: 1, MinWidth: core.Pt(0)}
		if fit == tierSmall {
			return core.View{Style: row.Merge(core.Style{FlexGrow: 1}), Children: []core.Node{w.titleBlock(),
				ui.IconButton{Label: "Compose", Icon: lucide.SquarePen, Variant: ui.IconButtonPrimary, OnPress: w.newMessage}}}
		}
		// The header's own inset is taken from the list's width.
		return core.View{Style: row, StyleFunc: func() core.Style { return core.Style{Width: core.Pt(w.listWidth.Get() - 12)} }, Children: []core.Node{
			w.titleBlock(), ui.Button{Text: "Compose", Icon: lucide.SquarePen, OnPress: w.newMessage}}}
	})
}

func (w *workspace) newMessage() { w.newDraft(api.DraftKindNew, nil) }

// actions is the toolbar's end: the open message's actions in Mail, and
// the palette. A window under 720 wide keeps Compose and one menu.
func (w *workspace) actions() core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		fit := w.tier.Get()
		inMail := w.module.Get() == "Mail"
		if fit == tierMenu {
			return bar(ui.IconButton{Label: "Compose", Icon: lucide.SquarePen, Variant: ui.IconButtonPrimary, OnPress: w.newMessage}, w.overflowMenu(inMail))
		}
		var parts []core.Node
		if inMail {
			parts = append(parts, w.messageToolbar())
		} else {
			parts = append(parts, ui.IconButton{Label: "Compose", Icon: lucide.SquarePen, OnPress: w.newMessage})
		}
		if inMail && fit < tierFull {
			return bar(append(parts, ui.IconButton{Label: "Search or run a command", Icon: lucide.Search, OnPress: w.palette})...)
		}
		return bar(append(parts, ui.SidebarButton{Text: "Search or run a command", Icon: lucide.Search, Shortcut: "mod+k", Variant: ui.SidebarButtonField, OnPress: w.palette})...)
	})
}

// sidebarFoot is the navigation's foot: where the app is connected, which
// opens Activity, and Settings.
func (w *workspace) sidebarFoot() core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		tone := workspaceui.Warning
		if w.online.Get() {
			tone = workspaceui.Success
		}
		link := base.Button{Label: "Activity", OnPress: func(core.PressEvent) { w.navigate("module:Activity") },
			Style: core.Style{FlexShrink: 1, MinWidth: core.Pt(0), CornerRadius: ui.RadiusControl},
			StyleFunc: func() core.Style {
				return core.Style{Hover: &core.Style{Background: colors.Hover()}, FocusVisible: ui.FocusRing(cx, -2)}
			},
			Children: []core.Node{workspaceui.ConnectionItem{Tone: tone, Text: w.status.Get}}}
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Width: core.Pct(100), Padding: core.Edges{Right: 6}}, Children: []core.Node{
			link, ui.Spacer{}, ui.IconButton{Label: "Settings", Icon: lucide.Settings, OnPress: func() { w.navigate("module:Settings") }}}}
	})
}

// tool is an action on the open message: a button of the toolbar, and an
// item of a narrow window's menu.
type tool struct {
	label string
	icon  func() *core.SVG
	// run is nil while there is nothing to act on, which dims the button.
	run func()
	// on draws the button as chosen: a flagged message's Flag.
	on bool
	// menu, when set, opens in place of run.
	menu []core.Node
}

type toolGroup struct {
	label string
	tools []tool
}

// openSummary is the message the reader shows, as maild last told it, or
// nil with none open.
func (w *workspace) openSummary() *api.MessageSummary {
	m := w.currentMessage.Get()
	if m == nil || m.ID != w.opened.Get() {
		return nil
	}
	return m
}

// messageTools are the toolbar's groups for a message, and the items of
// its More menu. With no message every tool is there with nothing to run,
// so the toolbar keeps its place.
func (w *workspace) messageTools(m *api.MessageSummary) ([]toolGroup, []core.Node) {
	file := []tool{{label: "Archive", icon: lucide.Archive}, {label: "Delete", icon: lucide.Trash}, {label: "Junk", icon: lucide.ShieldAlert}}
	answer := []tool{{label: "Reply", icon: lucide.Reply}, {label: "Reply all", icon: lucide.ReplyAll}, {label: "Forward", icon: lucide.Forward}}
	mark := []tool{{label: "Flag", icon: lucide.Flag}, {label: "Move", icon: lucide.FolderInput}}
	groups := []toolGroup{{"File", file}, {"Answer", answer}, {"Mark", mark}}
	undo := w.undo
	undoItem := ui.MenuItem{Text: "Undo", Icon: lucide.Undo2, Shortcut: "mod+z", Disabled: w.acting.Get, OnSelect: func() {
		w.change(func(ctx context.Context, c *api.Client) error { _, err := undo.Do(ctx, c); return err }, nil)
	}}
	if m == nil {
		return groups, []core.Node{undoItem}
	}
	msg := *m
	ids := []int64{msg.ID}
	boxes := w.store.State().Mailboxes
	writable := w.writable(msg.AccountID)
	action := func(title string, fn func(context.Context, *api.Client) error) ui.MenuItem {
		return ui.MenuItem{Text: title, Disabled: func() bool { return !writable || w.acting.Get() }, OnSelect: func() { w.change(fn, nil) }}
	}
	flags := func(label string, ch api.FlagChanges) ui.MenuItem {
		return action(label, func(ctx context.Context, c *api.Client) error { return undo.Flags(ctx, c, ids, ch) })
	}
	draft := func(kind api.DraftKind) func() { return func() { w.newDraft(kind, &msg.ID) } }
	answer[0].run, answer[1].run, answer[2].run = draft(api.DraftKindReply), draft(api.DraftKindReplyall), draft(api.DraftKindForward)
	if writable {
		if box := mail.ArchiveOf(msg.AccountID, boxes); box != nil {
			file[0].run = func() { w.change(w.moveMessage(msg, *box), w.moved("Message archived")) }
		}
		file[1].run = func() { w.deleteMessage(msg) }
		if unjunk, box := mail.SpamTarget([]api.MessageSummary{msg}, boxes); box != nil {
			if unjunk {
				file[2].label, file[2].icon = "Not junk", lucide.ShieldCheck
			}
			file[2].run = func() { w.change(w.moveMessage(msg, *box), w.moved("Message moved to "+box.Name)) }
		}
		flag := !msg.Flags.Flagged
		if !flag {
			mark[0].label, mark[0].on = "Unflag", true
		}
		mark[0].run = func() {
			w.change(func(ctx context.Context, c *api.Client) error {
				return undo.Flags(ctx, c, ids, api.FlagChanges{Flagged: &flag})
			}, nil)
		}
		for _, box := range mail.MoveTargets(&msg, boxes) {
			mark[1].menu = append(mark[1].menu, ui.MenuItem{Text: box.Name, Disabled: w.acting.Get, OnSelect: func() {
				w.change(w.moveMessage(msg, box), w.moved("Message moved to "+box.Name))
			}})
		}
	}

	seen := !msg.Flags.Seen
	readLabel := "Mark as unread"
	if seen {
		readLabel = "Mark as read"
	}
	var names []string
	if st := w.store.State().Settings; st != nil {
		names = st.FlagNames
	}
	var colors []core.Node
	for i := int64(1); i <= 7; i++ {
		yes := true
		colors = append(colors, flags(mail.FlagLabel(int(i), names), api.FlagChanges{Flagged: &yes, FlagColor: &i}))
	}
	more := []core.Node{flags(readLabel, api.FlagChanges{Seen: &seen}), ui.Submenu{Text: "Flag color", Children: colors}}
	if mail.InRole(msg, api.MailboxRoleDrafts, boxes) {
		more = append(more, ui.MenuItem{Text: "Edit draft", OnSelect: func() {
			perform(w, func(ctx context.Context, c *api.Client) (*api.Draft, error) {
				return c.Draft().Open(ctx, &api.DraftOpenParams{MessageID: msg.ID})
			}, w.editDraft)
		}})
	}
	more = append(more, ui.MenuSeparator{}, ui.MenuItem{Text: "Forward as attachment", OnSelect: draft(api.DraftKindAttached)})
	more = append(more, w.moreMessageActions(msg)...)
	more = append(more, w.readerTools(msg.ID)...)
	more = append(more, ui.MenuSeparator{})
	var copies []core.Node
	for _, box := range mail.CopyTargets(&msg, boxes) {
		copies = append(copies, action(box.Name, func(ctx context.Context, c *api.Client) error {
			return c.Message().Copy(ctx, &api.MessageCopyParams{IDs: ids, MailboxID: box.ID})
		}))
	}
	if len(copies) > 0 {
		more = append(more, ui.Submenu{Text: "Copy to", Children: copies})
	}
	muted := msg.Muted == nil || !*msg.Muted
	muteLabel := "Mute conversation"
	if !muted {
		muteLabel = "Unmute conversation"
	}
	more = append(more, action(muteLabel, func(ctx context.Context, c *api.Client) error {
		return c.Message().Mute(ctx, &api.MessageMuteParams{IDs: ids, Muted: muted})
	}), w.remindMenu(msg), ui.MenuSeparator{}, undoItem)
	return groups, more
}

// toolset is what the toolbar offers for the message that is open.
type toolset struct {
	groups []toolGroup
	more   []core.Node
}

// tools follows the open message. The toolbar's and the menus' nodes read
// it and stay in place when it changes, so a menu that is open stays open
// while maild reports a change of its message.
func (w *workspace) tools(cx *core.Scope) func() toolset {
	return core.NewMemoFunc(cx, func() toolset {
		if ids := w.selectedIDs(); len(ids) > 1 {
			groups, more := w.bulkTools(ids)
			return toolset{groups, more}
		}
		groups, more := w.messageTools(w.openSummary())
		return toolset{groups, more}
	}, func(toolset, toolset) bool { return false }).Get
}

// messageToolbar is the toolbar's reader segment: icon buttons in groups,
// named by their tooltips, and the More menu.
func (w *workspace) messageToolbar() core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		tools := w.tools(cx)
		// The groups and their places never change; only what a place
		// holds does, which the buttons read as they draw.
		shape := core.Untracked(cx, tools).groups
		var children []core.Node
		for g, group := range shape {
			if g > 0 {
				children = append(children, ui.ToolbarSeparator{})
			}
			var items []core.Node
			for i := range group.tools {
				at := func() tool { return tools().groups[g].tools[i] }
				button := ui.IconButton{LabelFunc: func() string { return at().label }, Icon: func() *core.SVG { return at().icon() },
					Selected: func() bool { return at().on }, Disabled: func() bool { return at().run == nil || w.acting.Get() },
					Tint: func() core.Color {
						if at().on {
							return colors.Warn()
						}
						return colors.Text()
					}, OnPress: func() {
						if run := at().run; run != nil {
							run()
						}
					}}
				// A tool with a menu is a menu's trigger while it has
				// items, and a dimmed button while it has none.
				has := core.NewMemo(cx, func() bool { return len(at().menu) > 0 })
				items = append(items, core.Dynamic(func(*core.Scope) core.Node {
					if !has.Get() {
						return button
					}
					label, icon := core.Untracked(cx, func() string { return at().label }), core.Untracked(cx, func() func() *core.SVG { return at().icon })
					return ui.Menu{Label: label, Trigger: ui.Button{Label: label, Icon: icon, Variant: ui.ButtonGhost},
						Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node { return core.Group(at().menu) })}}
				}))
			}
			if g == len(shape)-1 {
				items = append(items, ui.Menu{Label: "More", Trigger: ui.Button{Label: "More", Icon: lucide.Ellipsis, Variant: ui.ButtonGhost},
					Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node { return core.Group(tools().more) })}})
			}
			children = append(children, ui.ToolbarGroup{Label: group.label, Children: items})
		}
		return ui.Toolbar{Label: "Message", Children: children}
	})
}

// overflowMenu is a narrow window's one menu: the open message's actions
// in Mail, and the palette.
func (w *workspace) overflowMenu(inMail bool) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		palette := ui.MenuItem{Text: "Search or run a command", Icon: lucide.Search, OnSelect: w.palette}
		items := []core.Node{palette}
		if inMail {
			tools := w.tools(cx)
			items = []core.Node{core.Dynamic(func(*core.Scope) core.Node {
				var items core.Group
				for _, g := range tools().groups {
					for _, t := range g.tools {
						if len(t.menu) > 0 {
							items = append(items, ui.Submenu{Text: t.label, Children: t.menu})
							continue
						}
						items = append(items, ui.MenuItem{Text: t.label, Icon: t.icon, Disabled: func() bool { return t.run == nil || w.acting.Get() }, OnSelect: func() {
							if t.run != nil {
								t.run()
							}
						}})
					}
					items = append(items, ui.MenuSeparator{})
				}
				return append(append(items, tools().more...), ui.MenuSeparator{})
			}), palette}
		}
		return ui.Menu{Label: "More", Trigger: ui.Button{Label: "More", Icon: lucide.Ellipsis, Variant: ui.ButtonGhost}, Children: items}
	})
}
