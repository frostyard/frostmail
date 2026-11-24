package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func main() { nsgo.Run(root, nsgo.WindowSize(1280, 820)) }

type workspace struct {
	cx              *core.Scope
	ctx             context.Context
	session         *conn.Session
	store           *live.Mail
	events          *live.Events
	view            *live.View
	module          *core.Signal[string]
	source          mail.Source
	search          string
	term            *core.Signal[string]
	switching       *core.Signal[bool]
	pendingTarget   string
	selected        *core.Signal[string]
	state, rows     *core.Signal[int]
	status, problem *core.Signal[string]
	opened          *core.Signal[int64]
	reader          *core.Signal[*messagePage]
	generation      int
	undo            *mail.Undo
	overlay         *core.Signal[*panel]
	acting          *core.Signal[bool]
}

func invalidate(s *core.Signal[int]) { s.Set(s.GetUntracked() + 1) }

func root(cx *core.Scope) core.Node {
	cx.SetTitle("FrostMail")
	target, err := platformTarget()
	if err != nil {
		return workspaceui.Page(core.Text{Content: err.Error()})
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &workspace{cx: cx, ctx: ctx, events: live.NewEvents(), source: mail.Source{Kind: mail.AllInboxes}, selected: core.NewSignal(cx, "all-inboxes"), state: core.NewSignal(cx, 0), rows: core.NewSignal(cx, 0), status: core.NewSignal(cx, "Connecting to maild…"), problem: core.NewSignal(cx, ""), opened: core.NewSignal(cx, int64(0)), reader: core.NewSignal(cx, (*messagePage)(nil))}
	w.term = core.NewSignal(cx, "")
	w.switching = core.NewSignal(cx, false)
	w.module = core.NewSignal(cx, "Mail")
	w.overlay = core.NewSignal(cx, (*panel)(nil))
	w.acting = core.NewSignal(cx, false)
	w.undo = &mail.Undo{}
	w.store = live.NewMail(func() { cx.Post(func() { invalidate(w.state) }) })
	w.session = conn.NewSession(target, conn.Options{Client: "FrostMail NSGo", OnConnect: func(ctx context.Context, c *api.Client, _ *api.Hello) error {
		go w.events.Run(c)
		if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
			return err
		}
		return w.store.Connect(ctx, c, w.events)
	}, OnChange: func(st conn.State) {
		cx.Post(func() {
			if w.pendingTarget != "" && st.Target.Key() == w.pendingTarget {
				w.pendingTarget = ""
				w.switching.Set(false)
			}
			if st.Connected {
				w.status.Set("Connected · " + st.Target.String())
				w.openView()
				return
			}
			w.clearView()
			if st.Err != nil {
				w.status.Set("Reconnecting · " + st.Err.Error())
			} else {
				w.status.Set("Connecting to maild…")
			}
		})
	}})
	go w.session.Run(ctx)
	cx.Cleanup(func() {
		cancel()
		if w.view != nil {
			w.view.Close()
		}
		w.store.Disconnect()
	})
	list := w.messageList()
	split := ui.ResizablePanelGroup{Style: ui.FillRest, HandleLabel: "Resize message list", Panels: []base.ResizablePanel{
		{Size: func() core.Points { return 390 }, MinSize: 180, Children: []core.Node{list}},
		{MinSize: 220, Children: []core.Node{core.Dynamic(w.messageReader)}},
	}}
	shell := workspaceui.Shell{Brand: "FrostMail", Sections: w.sections, Selected: func() string {
		if mode := w.module.Get(); mode != "Mail" {
			return "module:" + mode
		}
		return w.selected.Get()
	}, Navigate: w.navigate,
		Actions:       []core.Node{ui.Button{Text: "Compose", Icon: lucide.Pencil, OnPress: func() { w.newDraft(api.DraftKindNew, nil) }}, ui.Button{Text: "Drafts", Label: "Open local drafts", Variant: ui.ButtonGhost, OnPress: w.draftList}, ui.Button{Text: "Outbox", Variant: ui.ButtonGhost, OnPress: w.outbox}},
		Header:        []core.Node{core.Text{ContentFunc: w.title, Style: ui.TypeTitle}},
		SidebarFooter: []core.Node{core.Text{ContentFunc: w.status.Get, Style: ui.TypeCaption.Merge(core.Style{Padding: core.All(10)})}},
		Content: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			if m := w.problem.Get(); m != "" {
				return workspaceui.Notice{Kind: ui.ToastWarning, Title: "Mail", Detail: m, OnDismiss: func() { w.problem.Set("") }}
			}
			return nil
		}), core.Dynamic(func(cx *core.Scope) core.Node {
			switch w.module.Get() {
			case "Settings":
				return w.settings(cx)
			case "People":
				return w.people(cx)
			case "Tasks":
				return w.tasks(cx)
			case "Calendar":
				return w.calendar(cx)
			default:
				return split
			}
		})}}
	return core.View{Style: ui.FillRest, Children: []core.Node{shell, core.Dynamic(func(cx *core.Scope) core.Node {
		if w.switching.Get() {
			dlg := w.dialog("Changing mail server…", nil)
			dlg.Footer = nil
			dlg.OnOpenChange = nil
			return dlg
		}
		if p := w.overlay.Get(); p != nil {
			return p.render(cx)
		}
		return nil
	})}}
}

func (w *workspace) sections() []ui.SidebarSection {
	w.state.Get()
	st := w.store.State()
	out := []ui.SidebarSection{{Title: "Workspace", Items: []ui.SidebarItem{{ID: "module:Mail", Text: "Mail", Icon: lucide.Mail}, {ID: "module:Calendar", Text: "Calendar", Icon: lucide.Calendar}, {ID: "module:People", Text: "People", Icon: lucide.Users}, {ID: "module:Tasks", Text: "Tasks", Icon: lucide.ListChecks}, {ID: "module:Settings", Text: "Settings", Icon: lucide.Settings}}}}
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
	for _, s := range mail.BuildSidebar(st.Accounts, st.Mailboxes, extras) {
		section := ui.SidebarSection{Title: s.Title, Collapsible: true}
		for _, i := range s.Items {
			if !i.Selectable {
				continue
			}
			label := i.Label
			if i.Unread > 0 {
				label += fmt.Sprintf("  ·  %d", i.Unread)
			}
			section.Items = append(section.Items, ui.SidebarItem{ID: i.Key, Text: label, Icon: lucide.Inbox})
		}
		out = append(out, section)
	}
	return out
}
func (w *workspace) title() string {
	if m := w.module.Get(); m != "Mail" {
		return m
	}
	w.selected.Get()
	w.state.Get()
	st := w.store.State()
	return mail.SourceTitle(w.source, mail.Names{Mailboxes: st.Mailboxes, Smart: st.Smart})
}
func (w *workspace) navigate(key string) {
	if m, ok := strings.CutPrefix(key, "module:"); ok {
		w.module.Set(m)
		return
	}
	w.module.Set("Mail")
	if s, ok := mail.SourceFromKey(key, w.store.State().VIPs); ok {
		w.source = s
		w.selected.Set(key)
		w.openView()
	}
}
func (w *workspace) clearView() {
	if w.view != nil {
		w.view.Close()
		w.view = nil
	}
	w.generation++
	w.opened.Set(0)
	w.reader.Set(nil)
	invalidate(w.rows)
}
func (w *workspace) openView() {
	w.clearView()
	c := w.session.Client()
	if c == nil {
		return
	}
	q := mail.SourceQuery(w.source, true)
	if w.search != "" {
		term := w.search
		q.Text = &term
	}
	w.view = live.OpenView(c, w.events, q, func() { w.cx.Post(func() { invalidate(w.rows) }) })
	w.view.Ensure(0, live.Page)
}
func (w *workspace) messageList() core.Node {
	term := w.term
	search := func(s string) { w.search = strings.TrimSpace(s); w.openView() }
	return core.View{Style: ui.FillRest, Children: []core.Node{
		workspaceui.Row(ui.Input{Label: "Search messages", Placeholder: "Search messages", Value: term.Get, OnChange: term.Set, OnSubmit: search}, ui.Button{Text: "Save search", Variant: ui.ButtonGhost, Disabled: func() bool { return strings.TrimSpace(term.Get()) == "" }, OnPress: func() { search(term.GetUntracked()); w.saveSearch() }}),
		ui.List{Label: "Messages", Style: ui.FillRest, EstimatedSize: 68, TapSelects: true, Count: func() int {
			w.rows.Get()
			if w.view == nil {
				return 0
			}
			return int(w.view.Count())
		},
			Row: func(cx *core.Scope, row core.ListRow) core.Node {
				return core.Dynamic(func(*core.Scope) core.Node {
					idx := int64(row.Index())
					w.rows.Get()
					v := w.view
					if v == nil {
						return nil
					}
					msg, ok := v.Row(idx)
					if !ok {
						v.Ensure(idx, idx+1)
						return core.Text{Content: "Loading…"}
					}
					sender := msg.From.Name
					if sender == "" {
						sender = msg.From.Address
					}
					unread := !msg.Flags.Seen
					if msg.ThreadUnread != nil {
						unread = *msg.ThreadUnread > 0
					}
					if unread {
						sender = "●  " + sender
					}
					w.state.Get()
					lines := mail.PreviewLines(w.store.State().Settings)
					children := []core.Node{core.Text{Content: sender, Style: ui.TypeLabel}, core.Text{Content: msg.Subject, MaxLines: 1}}
					if lines > 0 {
						children = append(children, core.Text{Content: msg.Preview, MaxLines: lines, Style: ui.TypeCaption})
					}
					return base.Button{Label: msg.Subject, Children: []core.Node{workspaceui.Stack(children...)}, OnPress: func(core.PressEvent) { w.openMessage(msg.ID, false) }}
				})
			}, OnActivate: func(i int) {
				if w.view != nil {
					if m, ok := w.view.Row(int64(i)); ok {
						w.openMessage(m.ID, false)
					}
				}
			},
			Empty: core.Text{ContentFunc: func() string {
				w.rows.Get()
				if w.view == nil {
					return ""
				}
				if err := w.view.Err(); err != nil {
					return err.Error()
				}
				if !w.view.Ready() {
					return "Loading messages…"
				}
				return "No messages"
			}}},
	}}
}

type messagePage struct {
	message   *api.Message
	rendering *api.Rendering
	thread    []api.MessageSummary
}

func (w *workspace) openMessage(id int64, remote bool) {
	c := w.session.Client()
	if c == nil {
		return
	}
	w.generation++
	gen := w.generation
	w.opened.Set(id)
	w.reader.Set(nil)
	go func() {
		ctx, cancel := context.WithTimeout(w.ctx, 45*time.Second)
		defer cancel()
		p := &messagePage{}
		var err error
		p.message, err = c.Message().Get(ctx, &api.MessageGetParams{ID: id})
		if err == nil && p.message.Summary.ThreadID != 0 {
			p.thread, err = c.Thread().Messages(ctx, &api.ThreadMessagesParams{ID: p.message.Summary.ThreadID})
		}
		content := w.session.Target().Remote()
		if err == nil {
			p.rendering, err = c.Message().Render(ctx, &api.MessageRenderParams{ID: id, Remote: &remote, Content: &content})
		}
		w.cx.Post(func() {
			if gen != w.generation || c != w.session.Client() {
				return
			}
			if err != nil {
				w.problem.Set(err.Error())
				w.opened.Set(0)
				return
			}
			unread := !p.message.Summary.Flags.Seen
			p.message.Summary.Flags.Seen = true
			w.reader.Set(p)
			if unread {
				w.call(func(ctx context.Context, c *api.Client) error {
					yes := true
					return c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{id}, Changes: api.FlagChanges{Seen: &yes}})
				})
			}
		})
	}()
}
func (w *workspace) call(f func(context.Context, *api.Client) error) {
	c := w.session.Client()
	if c == nil {
		w.problem.Set("Reconnect to maild to continue.")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(w.ctx, 45*time.Second)
		defer cancel()
		if err := f(ctx, c); err != nil {
			w.cx.Post(func() {
				if w.session.Client() == c {
					w.problem.Set(err.Error())
				}
			})
		}
	}()
}
func (w *workspace) messageReader(cx *core.Scope) core.Node {
	id := w.opened.Get()
	p := w.reader.Get()
	if p == nil {
		label := "Select a message"
		if id != 0 {
			label = "Loading message…"
		}
		return workspaceui.Page(core.Text{Content: label, Style: ui.TypeTitle})
	}
	m := p.message.Summary
	children := []core.Node{core.Text{Content: m.Subject, Style: ui.TypeTitle}, core.Text{Content: m.From.Address + " · " + m.Date.Local().Format("Jan 2, 2006 15:04"), Style: ui.TypeCaption}}
	children = append(children, w.triage(p.message.Summary))
	if p.message.SenderCheck != nil && *p.message.SenderCheck == api.SenderCheckFail {
		children = append(children, core.Text{Content: mail.SenderCheckText(m.From.Address), Style: ui.TypeLabel})
	}
	if len(p.thread) > 1 {
		items := []core.Node{}
		for _, item := range p.thread {
			if item.ID != id {
				items = append(items, ui.Button{Text: item.From.Address + " · " + item.Date.Local().Format("Jan 2"), OnPress: func() { w.openMessage(item.ID, false) }})
			}
		}
		children = append(children, workspaceui.Section{Title: "Conversation", Count: func() int { return len(p.thread) }, Content: func(*core.Scope) core.Node { return workspaceui.Stack(items...) }})
	}
	if p.rendering.Remote > 0 {
		children = append(children, ui.Button{Text: "Load remote images (" + strconv.FormatInt(p.rendering.Remote, 10) + ")", OnPress: func() { w.openMessage(id, true) }})
	}
	children = append(children, w.readerExtras(p.message)...)
	children = append(children, bodyView(w, p.rendering))
	return core.View{Style: ui.FillRest.Merge(core.Style{Padding: core.All(12), Gap: 8}), Children: children}
}
func plainBody(s string) core.Node {
	return core.Scroll{Style: ui.FillRest, Children: []core.Node{core.Text{Content: s, Selectable: core.SelectableOn}}}
}
