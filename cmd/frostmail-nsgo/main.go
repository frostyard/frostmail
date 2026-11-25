package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/problemrec"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/frostyard/std/problems"
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/dragdrop"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// version is the build's version: a release's is stamped at its build
// (scripts/build-nsgo-native.sh), and any other build is "dev", which
// records no problem (internal/problemrec).
var version = "dev"

// codeSocketLost is the problem record of maild's local socket ending
// under the app (ADR-0049), as the MyGo client records it.
const codeSocketLost = "bridge.socket_lost"

func main() {
	prepareLaunch()
	rec := problemrec.New(version, problems.ComponentShell)
	defer rec.RecoverPanic()
	if rec.Err() == nil { // an unreleased build records nothing
		if err := rec.CrashOutput(); err != nil {
			slog.Warn("recording crashes", "err", err)
		}
	}
	view := rootWith(clientOptions{socketLost: func(err error) { rec.Error(codeSocketLost, err) }})
	nsgo.Run(func(app *core.Scope) { nsgo.SetMainView(app, view) }, nsgo.InstanceKey(os.Getenv("FROSTMAIL_INSTANCE")), nsgo.WindowSize(1280, 820),
		nsgo.WindowConstraints(core.WindowConstraints{MinWidth: minWidth, MinHeight: minHeight}))
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
	// connected says the last report of the connection was a live one, so
	// that its end is a loss.
	connected bool
	// firstLook says this connection's accounts have not been looked at
	// yet, for the first run. hostMail reports a host install's mail, and
	// settingsStart is the category Settings opens on next.
	firstLook     bool
	hostMail      func() bool
	settingsStart string
	dropping      bool
	nav           *core.Signal[*moduleNav]
	listWidth     *core.Signal[core.Points]
	contentWidth  *core.Signal[core.Points]
	tier          *core.Memo[tier]
}

func invalidate(s *core.Signal[int]) { s.Set(s.GetUntracked() + 1) }

// clientOptions are what the client's start is given.
type clientOptions struct {
	// socketLost, when set, is told that maild's local socket ended under
	// the app.
	socketLost func(error)
	// hostMail reports whether the first run says how to move a host
	// install's mail; nil asks the platform (hostMailFound).
	hostMail func() bool
}

// root is the client as a test starts it: with nothing to tell of a lost
// socket.
func root(cx *core.Scope) core.Node { return rootWith(clientOptions{})(cx) }

// rootWith is the client with its options.
func rootWith(o clientOptions) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node { return rootView(cx, o) }
}

func rootView(cx *core.Scope, o clientOptions) core.Node {
	socketLost := o.socketLost
	cx.SetTitle("FrostMail")
	// The Frostyard look and the bundled Inter (plan 0016, rule 1). A font
	// the platform refuses leaves the text in the system's face.
	_ = workspaceui.Apply(cx)
	// NSGo's own scroll bars, over the content and in the theme's colors,
	// in place of the platform's. A browser's bar takes 12 points of a
	// scrolling view's width that the layout does not know of: in Lodge it
	// cut the ends of the compose sheet's fields, and stood as a light
	// strip beside the lists.
	base.SetScrollbarTheme(cx, ui.ScrollbarTheme(cx))
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
	w.nav = core.NewSignalFunc(cx, (*moduleNav)(nil), func(a, b *moduleNav) bool { return a == b })
	w.watchReader()
	// How the list is shown is the device's, kept from the last run.
	kept, photos := keptListState(cx)
	w.showPhotos = core.NewSignal(cx, photos)
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
	w.listState = kept
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
				// The first look at this connection's accounts decides whether
				// the first run opens the account's dialog.
				w.firstLook = true
				w.online.Set(true)
				w.connected = true
				w.openView()
				if f := w.deferredOpen; f != nil {
					w.deferredOpen = nil
					f()
				}
				return
			}
			// maild's local socket ended under the app: a problem record.
			// Lodge's WebSocket never counts: its losses are the network's.
			if w.connected && !st.Target.Remote() && st.Err != nil && socketLost != nil {
				socketLost(st.Err)
			}
			w.connected = false
			w.clearView()
			w.online.Set(false)
			if st.Err != nil {
				w.status.Set("Reconnecting · " + st.Err.Error())
			} else {
				w.status.Set("Connecting to maild…")
			}
		})
	}})
	w.hostMail = o.hostMail
	if w.hostMail == nil {
		w.hostMail = hostMailFound
	}
	w.watchFirstRun(cx)
	// The content follows whether the store has no account, and not every
	// change of the store: the list and its search field are not built
	// again as mail arrives.
	firstRun := core.NewMemo(cx, w.needsFirstRun)
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
	w.keepWindow(cx)
	panes := w.panes()
	listWidth := core.NewSignal(cx, panes.List)
	// A drag of the list's edge reports many widths: the last is kept.
	var keepList func() bool
	resizeList := func(width core.Points) {
		listWidth.Set(width)
		if keepList != nil {
			keepList()
		}
		keepList = cx.AfterFunc(200*time.Millisecond, func() { w.keepList(width) })
	}
	w.listWidth, w.contentWidth = listWidth, core.NewSignal(cx, core.Points(0))
	w.tier = core.NewMemo(cx, w.toolbarTier)
	split := ui.ResizablePanelGroup{Style: ui.FillRest, HandleLabel: "Resize message list", Panels: []base.ResizablePanel{
		{Size: listWidth.Get, OnResize: resizeList, MinSize: listLeast, Children: []core.Node{list}},
		{MinSize: 220, Children: []core.Node{core.Dynamic(w.messageReader)}},
	}}
	shell := workspaceui.Shell{Brand: "FrostMail", Sections: w.navSections, Selected: w.navSelected, Navigate: w.pressNav,
		Widths: panes.PaneWidths, OnWidthsChange: w.keepPanes, Inspector: &workspaceui.Inspector{Title: "To-Do Bar", Available: func() bool { return w.module.Get() == "Mail" }, Content: w.todoBar},
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
			}
			// A store with no account shows the welcome in place of the
			// modules. Settings and Activity, which need none, stay.
			if firstRun.Get() {
				return w.welcome(cx)
			}
			switch w.module.Get() {
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
					back := ui.Button{Label: "Back to message list", Text: "Messages", Icon: lucide.ChevronLeft, Variant: ui.ButtonGhost, OnPress: func() {
						if w.backHistory != nil && w.backHistory() {
							return
						}
						w.generation++
						w.opened.Set(0)
						w.reader.Set(nil)
					}}
					return core.View{Style: ui.FillRest, Children: []core.Node{core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{back}}, core.Dynamic(w.messageReader)}}
				})
			}
		})}}}}
	return workspaceui.Frame(dragdrop.Area{Children: []core.Node{shell}}, ui.Toasts{Toaster: w.toaster, Placement: ui.ToastBottomEnd, StyleFunc: workspaceui.DialogPanel(cx)}, core.Dynamic(func(cx *core.Scope) core.Node {
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
