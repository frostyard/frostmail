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
	"github.com/frostyard/frostui/workspaceui"
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
	backHistory     func() bool
	deferredOpen    func()
	beforeRoute     func(func(), func())
	routeGeneration int
	trayAvailable   bool
	toaster         *ui.Toaster
	searchInput     *core.Ref
	trayChoice      *core.Signal[bool]
	listState       mail.ListState
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
	textScale       *core.Signal[float32]
	stopPhotos      context.CancelFunc
	generation      int
	undo            *mail.Undo
	overlay         *core.Signal[*panel]
	acting          *core.Signal[bool]
	appearance      *core.Signal[core.Appearance]
	online          *core.Signal[bool]
	dropping        bool
	listWidth       *core.Signal[core.Points]
	contentWidth    *core.Signal[core.Points]
	tier            *core.Memo[tier]
}

func invalidate(s *core.Signal[int]) { s.Set(s.GetUntracked() + 1) }

func root(cx *core.Scope) core.Node {
	cx.SetTitle("FrostMail")
	// The Frostyard look and the bundled Inter (plan 0016, rule 1). A font
	// the platform refuses leaves the text in the system's face.
	_ = workspaceui.Apply(cx)
	appearance := core.AppearanceSystem
	if desktopServer {
		// Lodge sets the web build's appearance; a desktop keeps its own.
		appearance = savedAppearance(cx)
		cx.SetAppearance(appearance)
	}
	target, err := platformTarget()
	if err != nil {
		return workspaceui.Page(core.Text{Content: err.Error()})
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &workspace{cx: cx, ctx: ctx, events: live.NewEvents(), source: mail.Source{Kind: mail.AllInboxes}, selected: core.NewSignal(cx, "all-inboxes"), state: core.NewSignal(cx, 0), rows: core.NewSignal(cx, 0), status: core.NewSignal(cx, "Connecting to maild…"), problem: core.NewSignal(cx, ""), opened: core.NewSignal(cx, int64(0)), reader: core.NewSignal(cx, (*messagePage)(nil))}
	w.currentMessage = core.NewSignal(cx, (*api.MessageSummary)(nil))
	w.appearance = core.NewSignal(cx, appearance)
	w.online = core.NewSignal(cx, false)
	w.watchReader()
	w.showPhotos = core.NewSignal(cx, true)
	w.textScale = core.NewSignal(cx, float32(1))
	w.toaster = ui.NewToaster(cx)
	w.searchInput = &core.Ref{}
	w.trayChoice = core.NewSignal(cx, false)
	w.selection = ui.NewSelection(cx, true)
	// The list's choice follows the reader: when a message closes by any
	// way but the list (Back, a removal, a route), the row lets go too, so
	// a press on it opens the message again.
	cx.Effect(func() {
		if w.opened.Get() == 0 && core.Untracked(cx, w.selection.Len) == 1 {
			w.selection.Clear()
		}
	})
	w.listState = mail.ListState{Conversations: true, Filter: mail.FilterAll, Sort: api.ViewSortDate}
	w.term = core.NewSignal(cx, "")
	w.switching = core.NewSignal(cx, false)
	w.module = core.NewSignal(cx, "Mail")
	w.overlay = core.NewSignal(cx, (*panel)(nil))
	w.acting = core.NewSignal(cx, false)
	w.undo = &mail.Undo{}
	w.store = live.NewMail(func() { cx.Post(func() { invalidate(w.state) }) })
	if desktopServer {
		cx.Effect(func() { w.state.Get(); w.textScale.Set(mail.TextScale(w.store.State().Settings)) })
	}
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
				w.status.Set(st.Target.String())
				w.online.Set(true)
				w.openView()
				if f := w.deferredOpen; f != nil {
					w.deferredOpen = nil
					f()
				}
				return
			}
			w.clearView()
			w.online.Set(false)
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
	listWidth := core.NewSignal(cx, core.Points(390))
	w.listWidth, w.contentWidth = listWidth, core.NewSignal(cx, core.Points(0))
	w.tier = core.NewMemo(cx, w.toolbarTier)
	split := ui.ResizablePanelGroup{Style: ui.FillRest, HandleLabel: "Resize message list", Panels: []base.ResizablePanel{
		{Size: listWidth.Get, OnResize: listWidth.Set, MinSize: 180, Children: []core.Node{list}},
		{MinSize: 220, Children: []core.Node{core.Dynamic(w.messageReader)}},
	}}
	shell := workspaceui.Shell{Brand: "FrostMail", Sections: w.sections, Selected: func() string {
		if mode := w.module.Get(); mode != "Mail" {
			return "module:" + mode
		}
		return w.selected.Get()
	}, Navigate: w.navigate,
		Widths: workspaceui.PaneWidths{Inspector: 280}, Inspector: &workspaceui.Inspector{Title: "To-Do Bar", Available: func() bool { return w.module.Get() == "Mail" }, Content: w.todoBar},
		Actions:       []core.Node{w.actions()},
		Header:        []core.Node{w.header()},
		SidebarHeader: []core.Node{w.moduleTabs()},
		SidebarFooter: []core.Node{w.sidebarFoot()},
		// The pane reports its width, which the toolbar over it fits to.
		Content: []core.Node{core.View{Style: ui.FillRest, OnLayout: func(b core.Bounds) { w.contentWidth.Set(b.Size.Width) }, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
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
					return core.View{Style: ui.FillRest, Children: []core.Node{button("Back to message list", func() {
						if w.backHistory != nil && w.backHistory() {
							return
						}
						w.generation++
						w.opened.Set(0)
						w.reader.Set(nil)
					}), core.Dynamic(w.messageReader)}}
				})
			}
		})}}}}
	return workspaceui.Frame(dragdrop.Area{Children: []core.Node{shell}}, ui.Toasts{Toaster: w.toaster, Placement: ui.ToastBottomEnd}, core.Dynamic(func(cx *core.Scope) core.Node {
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
	}))
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
	// In a GTK window the release that drops a message on a mailbox also
	// presses that mailbox's row, in the same turn of the window's loop
	// (frostyard/frostmail#392): a drop files the message and leaves the
	// list where it is.
	if w.dropping {
		return
	}
	switch key {
	case panelDrafts:
		w.draftList()
		return
	case panelOutbox:
		w.outbox()
		return
	}
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
	// A message opened from elsewhere (Follow up, a link) is the list's choice too.
	if !w.selection.Has(id) || w.selection.Len() != 1 {
		w.selection.Set(id)
	}
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
		switch chosen := w.selection.Len(); {
		case id != 0:
			return workspaceui.Page(ui.Skeleton{Avatar: true, Lines: 3, Label: "Loading message"})
		case chosen > 1:
			return workspaceui.EmptyState{Icon: lucide.Mails, Title: strconv.Itoa(chosen) + " messages selected", Hint: "Delete, flag or move them from the toolbar."}
		}
		return workspaceui.EmptyState{Icon: lucide.MailOpen, Title: "No message selected", Hint: "Choose a message to read it here."}
	}
	m := p.message.Summary
	children := []core.Node{core.Text{Content: m.Subject, Style: ui.TypeTitle}, core.Text{Content: m.From.Address + " · " + m.Date.Local().Format("Jan 2, 2006 15:04"), Style: ui.TypeCaption}}
	children = append(children, core.Dynamic(func(*core.Scope) core.Node {
		current := w.currentMessage.Get()
		if current == nil || current.ID != id {
			return nil
		}
		if current.RemindAt != nil {
			return workspaceui.Row(core.Text{Content: "Reminder · " + mail.WhenText(*current.RemindAt, time.Now())}, button("Remove reminder", func() { w.remind(id, nil) }))
		}
		return nil
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
