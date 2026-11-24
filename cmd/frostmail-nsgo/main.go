package main

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	"github.com/nsgo-dev/nsgo/base/dragdrop"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func main() {
	prepareLaunch()
	nsgo.Run(func(app *core.Scope) { nsgo.SetMainView(app, root) }, nsgo.InstanceKey(os.Getenv("FROSTMAIL_INSTANCE")), nsgo.WindowSize(1280, 820))
}

type workspace struct {
	calendarDate    time.Time
	deferredOpen    func()
	beforeRoute     func(func(), func())
	routeGeneration int
	trayAvailable   bool
	toaster         *ui.Toaster
	searchInput     *core.Ref
	trayChoice      *core.Signal[bool]
	listState       mail.ListState
	selecting       *core.Signal[bool]
	selection       *ui.Selection
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
	currentMessage  *core.Signal[*api.MessageSummary]
	photos          *live.Photos
	showPhotos      *core.Signal[bool]
	stopPhotos      context.CancelFunc
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
	w.currentMessage = core.NewSignal(cx, (*api.MessageSummary)(nil))
	w.watchReader()
	w.showPhotos = core.NewSignal(cx, true)
	w.toaster = ui.NewToaster(cx)
	w.searchInput = &core.Ref{}
	w.trayChoice = core.NewSignal(cx, false)
	w.selecting = core.NewSignal(cx, false)
	w.selection = ui.NewSelection(cx, true)
	w.listState = mail.ListState{Conversations: true, Filter: mail.FilterAll, Sort: api.ViewSortDate}
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
		if _, err := c.Activity().Subscribe(ctx, &api.ActivitySubscribeParams{}); err != nil && !errors.Is(err, api.ErrMethodNotFound) {
			return err
		}
		return w.store.Connect(ctx, c, w.events)
	}, OnChange: func(st conn.State) {
		cx.Post(func() {
			if w.pendingTarget != "" && st.Target.Key() == w.pendingTarget {
				w.pendingTarget = ""
				w.switching.Set(false)
			}
			if w.stopPhotos != nil {
				w.stopPhotos()
				w.stopPhotos = nil
				w.photos = nil
			}
			if c := w.session.Client(); st.Connected && c != nil && w.session.Target().Same(st.Target) {
				w.resetPhotos(c)
				w.checkReminders()
				w.status.Set("Connected · " + st.Target.String())
				w.openView()
				if f := w.deferredOpen; f != nil {
					w.deferredOpen = nil
					f()
				}
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
	w.commands()
	w.platformStart()
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
		Widths: workspaceui.PaneWidths{Inspector: 280}, Inspector: &workspaceui.Inspector{Title: "To-Do Bar", Available: func() bool { return w.module.Get() == "Mail" }, Content: w.todoBar},
		Actions:       []core.Node{ui.Button{Label: "Commands", Icon: lucide.Command, Variant: ui.ButtonGhost, OnPress: w.palette}, ui.Button{Text: "Compose", Icon: lucide.Pencil, OnPress: func() { w.newDraft(api.DraftKindNew, nil) }}, ui.Button{Text: "Drafts", Label: "Open local drafts", Variant: ui.ButtonGhost, OnPress: w.draftList}, ui.Button{Text: "Outbox", Variant: ui.ButtonGhost, OnPress: w.outbox}},
		Header:        []core.Node{core.Text{ContentFunc: w.title, Style: ui.TypeTitle}},
		SidebarFooter: []core.Node{core.Text{ContentFunc: w.status.Get, Style: ui.TypeCaption.Merge(core.Style{Padding: core.All(10)})}},
		Content: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			if m := w.problem.Get(); m != "" {
				return workspaceui.Notice{Kind: ui.ToastWarning, Title: "Mail", Detail: m, OnDismiss: func() { w.problem.Set("") }}
			}
			return nil
		}), core.Dynamic(func(cx *core.Scope) core.Node {
			switch w.module.Get() {
			case "Activity":
				return w.activity(cx)
			case "Settings":
				return w.settings(cx)
			case "People":
				return w.people(cx)
			case "Tasks":
				return w.tasks(cx)
			case "Calendar":
				return w.calendar(cx)
			default:
				compact := core.NewMemo(cx, func() bool { return cx.Env().Window().Width < 800 })
				return core.Dynamic(func(*core.Scope) core.Node {
					if !compact.Get() {
						return split
					}
					if w.opened.Get() == 0 {
						return list
					}
					return core.View{Style: ui.FillRest, Children: []core.Node{button("Back to message list", func() { w.generation++; w.opened.Set(0); w.reader.Set(nil) }), core.Dynamic(w.messageReader)}}
				})
			}
		})}}
	return core.View{Style: ui.FillRest, Children: []core.Node{dragdrop.Area{Children: []core.Node{shell}}, ui.Toasts{Toaster: w.toaster, Placement: ui.ToastBottomEnd}, core.Dynamic(func(cx *core.Scope) core.Node {
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
	out := []ui.SidebarSection{{Title: "Workspace", Items: []ui.SidebarItem{{ID: "module:Mail", Text: "Mail", Icon: lucide.Mail}, {ID: "module:Calendar", Text: "Calendar", Icon: lucide.Calendar}, {ID: "module:People", Text: "People", Icon: lucide.Users}, {ID: "module:Tasks", Text: "Tasks", Icon: lucide.ListChecks}, {ID: "module:Settings", Text: "Settings", Icon: lucide.Settings}, {ID: "module:Activity", Text: "Activity", Icon: lucide.Activity}}}}
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
	boxes := make(map[int64]api.Mailbox, len(st.Mailboxes))
	for _, b := range st.Mailboxes {
		boxes[b.ID] = b
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
			item := ui.SidebarItem{ID: i.Key, Text: label, Icon: lucide.Inbox}
			if b, ok := boxes[i.MailboxID]; ok {
				item.Content = w.mailboxTarget(i, label, s.Title, b)
				for _, a := range st.Accounts {
					if a.ID == b.AccountID {
						item.Menu = []core.Node{ui.MenuItem{Text: "Mailbox options", OnSelect: func() { w.mailboxMenu(a, b) }}}
						break
					}
				}
			}
			section.Items = append(section.Items, item)
		}
		out = append(out, section)
	}
	if follow := st.FollowUp; follow != nil && len(follow.Threads) > 0 {
		section := ui.SidebarSection{Title: "Follow up", Collapsible: true}
		for _, t := range follow.Threads {
			section.Items = append(section.Items, ui.SidebarItem{ID: fmt.Sprintf("follow:%d", t.Message.ID), Text: t.Message.Subject, Icon: lucide.Clock})
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
	if raw, ok := strings.CutPrefix(key, "follow:"); ok {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && id > 0 {
			w.openMessage(id, false)
		}
		return
	}
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
	w.selection.Clear()
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
	w.listState.Source = w.source
	w.listState.Search = w.search
	q := mail.ListQuery(w.listState)
	w.view = live.OpenView(c, w.events, q, func() { w.cx.Post(func() { invalidate(w.rows) }) })
	w.view.Ensure(0, live.Page)
}
func (w *workspace) messageList() core.Node {
	term := w.term
	search := func(s string) { w.search = strings.TrimSpace(s); w.openView() }
	return core.View{Style: ui.FillRest, Children: []core.Node{
		workspaceui.Row(ui.Input{Ref: w.searchInput, Label: "Search messages", Placeholder: "Search messages", Value: term.Get, OnChange: term.Set, OnSubmit: search}, ui.Button{Text: "Save search", Variant: ui.ButtonGhost, Disabled: func() bool { return strings.TrimSpace(term.Get()) == "" }, OnPress: func() { search(term.GetUntracked()); w.saveSearch() }}),
		w.listOptions(), w.bulkActions(), ui.List{Label: "Messages", Style: ui.FillRest, EstimatedSize: 68, TapSelects: true, Count: func() int {
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
					if mail.IsVIP(msg.From.Address, w.store.State().VIPs) {
						sender = "★ " + sender
					}
					if msg.Flags.Flagged {
						sender += " · ⚑"
						if color := msg.Flags.FlagColor; color > 0 {
							var names []string
							if st := w.store.State().Settings; st != nil {
								names = st.FlagNames
							}
							sender += " " + mail.FlagLabel(int(color), names)
						}
					}
					if msg.RemindAt != nil {
						sender += " · ◷"
					}
					if w.listState.Conversations && msg.ThreadCount > 1 {
						sender += fmt.Sprintf(" · %d messages", msg.ThreadCount)
					}
					children := []core.Node{core.Text{Content: sender, Style: ui.TypeLabel}, core.Text{Content: msg.Subject, MaxLines: 1}}
					if lines > 0 {
						children = append(children, core.Text{Content: msg.Preview, MaxLines: lines, Style: ui.TypeCaption})
					}
					var content core.Node = workspaceui.Stack(children...)
					if w.showPhotos.Get() && w.photos != nil {
						w.photos.Want([]string{msg.From.Address})
						if photo := w.photos.Get(msg.From.Address); len(photo) > 0 {
							content = workspaceui.Row(core.Image{Label: "Sender photo", Source: core.ImageSource{Data: photo}, Style: core.Style{Width: core.Pt(32), Height: core.Pt(32), CornerRadius: 16}}, content)
						}
					}
					colors := ui.ColorsOf(cx)
					rowButton := base.Button{Label: msg.Subject, Selected: func() bool { return w.opened.Get() == msg.ID }, StyleFunc: func() core.Style {
						return core.Style{Width: core.Pct(100), Padding: core.All(5), CornerRadius: 6, Selected: &core.Style{Background: ui.Alpha(colors.Accent(), 0.15)}}
					}, Children: []core.Node{content}, OnPress: func(core.PressEvent) { w.openMessage(msg.ID, false) }}
					draggable := w.dragRow(cx, msg, rowButton)
					if w.selecting.Get() {
						return workspaceui.Row(ui.Checkbox{Text: "", Label: "Select " + msg.Subject, Checked: func() bool { return w.selection.Has(msg.ID) }, OnChange: func(bool) { w.selection.Toggle(msg.ID) }}, draggable)
					}
					return draggable
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
	remote    bool
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
		p := &messagePage{remote: remote}
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
			unread := !p.message.Summary.Flags.Seen && !remote
			w.currentMessage.Set(&p.message.Summary)
			w.reader.Set(p)
			if unread && w.writable(p.message.Summary.AccountID) {
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
	children = append(children, core.Dynamic(func(*core.Scope) core.Node {
		current := w.currentMessage.Get()
		if current == nil || current.ID != id {
			return nil
		}
		nodes := []core.Node{w.triage(*current)}
		if current.RemindAt != nil {
			nodes = append(nodes, workspaceui.Row(core.Text{Content: "Reminder · " + mail.WhenText(*current.RemindAt, time.Now())}, button("Remove reminder", func() { w.remind(id, nil) })))
		}
		return workspaceui.Stack(nodes...)
	}))
	if p.message.SenderCheck != nil && *p.message.SenderCheck == api.SenderCheckFail {
		children = append(children, core.Text{Content: mail.SenderCheckText(m.From.Address), Style: ui.TypeLabel})
	}
	if len(p.thread) > 1 {
		children = append(children, button(fmt.Sprintf("View conversation (%d messages)", len(p.thread)), func() { w.conversation(p) }))
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

func (w *workspace) resetPhotos(c *api.Client) {
	if w.stopPhotos != nil {
		w.stopPhotos()
	}
	photoCtx, stop := context.WithCancel(w.ctx)
	w.stopPhotos = stop
	w.photos = live.NewPhotos(photoCtx, c, func() { w.cx.Post(func() { invalidate(w.rows) }) })
	invalidate(w.rows)
}
