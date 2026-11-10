// Package mainwin is the desktop app's main window: the frame, the panes
// and the commands, over the live data (plan 0015). All of its state
// belongs to the UI thread; goroutines hand results back through Post.
package mainwin

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/frame"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// The panes' widths (docs/specs/ui.md, Layout).
const (
	sidebarDefault = 220
	listDefault    = 360
)

// Options configure a Window.
type Options struct {
	Target conn.Target
	// Client names the app in rpc.hello.
	Client string
	// Post runs a function on the UI thread and redraws.
	Post func(func())
	// Wake is how often redraws from events may come [16ms].
	Wake time.Duration
	// Retry is the wait between connection attempts [1s].
	Retry time.Duration
	// Now is the clock for the list's dates [time.Now].
	Now func() time.Time
}

// Window is the main window's state.
type Window struct {
	o       Options
	session *conn.Session
	waker   *frame.Waker
	mail    *live.Mail
	act     *live.Activity
	shell   frame.Shell
	console *frostui.Console

	// The UI thread's.
	client        *api.Client
	events        *live.Events
	view          *live.View
	opened        string // the open view's query
	q             mail.ListState
	searchDraft   string
	searchToken   int
	searchFocused bool
	focusSearch   bool
	sidebarWidth  float32
	listWidth     float32
	sidebarHidden bool
	list          ui.ListState
	chosen        ui.Selection[int64]
	selected      int
	lastIndex     int64
	undo          mail.Undo
	contactPhotos bool
	photos        *live.Photos
	bitmaps       map[string]*ui.Bitmap
	state         conn.State
	handlers      map[commands.Command]func()
	toasts        []ui.Toast

	mu     sync.Mutex
	latest map[int64]string // each account's newest activity, for the status bar
}

// New returns a window that is not connected yet; Start connects it.
func New(o Options) *Window {
	if o.Wake == 0 {
		o.Wake = 16 * time.Millisecond
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	w := &Window{o: o, sidebarWidth: sidebarDefault, listWidth: listDefault, selected: -1,
		q:       mail.ListState{Source: mail.Source{Kind: mail.AllInboxes}, Conversations: true, Filter: mail.FilterAll, Sort: api.ViewSortDate, SearchAll: true},
		console: frostui.NewConsole(5000), latest: map[int64]string{}, state: conn.State{Target: o.Target}}
	w.waker = frame.NewWaker(o.Post, o.Wake)
	w.shell.Dialogs = &frostui.Dialogs{}
	w.mail = live.NewMail(w.waker.Wake)
	w.act = live.NewActivity(w.addActivity)
	w.console.SetNotify(w.waker.Wake)
	w.session = conn.NewSession(o.Target, conn.Options{Client: o.Client, Retry: o.Retry,
		OnConnect: w.connected, OnChange: w.changed})
	w.handlers = map[commands.Command]func(){
		commands.Palette:       w.shell.Palette.Show,
		commands.Activity:      func() { w.shell.Console.Open = !w.shell.Console.Open },
		commands.ToggleSidebar: func() { w.sidebarHidden = !w.sidebarHidden },
		commands.AllInboxes:    func() { w.setSource(mail.Source{Kind: mail.AllInboxes}) },
		commands.GetMail:       w.getMail,
		commands.Shortcuts:     w.showShortcuts,
		commands.About:         w.showAbout,
		commands.Undo:          w.runUndo,
		commands.Archive:       func() { w.archive(w.selectedIDs()) },
		commands.Delete:        func() { w.delete(w.selectedIDs()) },
		commands.Junk:          func() { w.toggleSpam(w.selectedIDs()) },
		commands.ToggleRead:    func() { w.toggleRead(w.selectedIDs()) },
		commands.ToggleFlag:    func() { w.toggleFlag(w.selectedIDs()) },
		commands.FocusSearch:   func() { w.focusSearch = true },
		commands.Escape:        w.escape,
	}
	return w
}

// Start connects and keeps the connection until ctx ends.
func (w *Window) Start(ctx context.Context) { go w.session.Run(ctx) }

// connected runs on the session's goroutine for each new connection.
func (w *Window) connected(ctx context.Context, c *api.Client, _ *api.Hello) error {
	events := live.NewEvents()
	go events.Run(c)
	if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		return err
	}
	if err := w.mail.Connect(ctx, c, events); err != nil {
		return err
	}
	if err := w.act.Connect(ctx, c, events); err != nil {
		return err
	}
	w.o.Post(func() {
		w.closeView()
		w.client, w.events = c, events
		w.photos, w.bitmaps = live.NewPhotos(c, w.waker.Wake), map[string]*ui.Bitmap{}
		w.openView()
	})
	return nil
}

// changed runs on the session's goroutine for each change of state.
func (w *Window) changed(st conn.State) {
	if !st.Connected {
		w.mail.Disconnect()
	}
	w.o.Post(func() {
		if !st.Connected {
			w.closeView()
			w.client, w.events = nil, nil
		}
		w.state = st
	})
}

// syncView opens the view of the list's current query, when it changed.
func (w *Window) syncView() {
	if w.client == nil {
		return
	}
	q := mail.ListQuery(w.q)
	key, _ := json.Marshal(q)
	if w.view != nil && string(key) == w.opened {
		return
	}
	w.closeView()
	w.opened = string(key)
	w.view = live.OpenView(w.client, w.events, q, w.waker.Wake)
}

func (w *Window) openView() { w.syncView() }

func (w *Window) closeView() {
	if w.view != nil {
		w.view.Close()
		w.view, w.opened = nil, ""
	}
}

// clearSelection forgets what the list had chosen, as a new list does.
func (w *Window) clearSelection() {
	w.chosen.Clear()
	w.selected, w.lastIndex = -1, 0
}

// setSource shows a source, leaving any search.
func (w *Window) setSource(s mail.Source) {
	w.q.Source, w.q.Search, w.searchDraft = s, "", ""
	w.clearSelection()
	w.syncView()
}

func (w *Window) getMail() {
	c, accounts := w.client, w.mail.State().Accounts
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, a := range accounts {
			_ = c.Sync().Now(ctx, &api.SyncNowParams{AccountID: a.ID})
		}
	}()
}

// names gives the console the names the sidebar shows.
type names struct{ s live.MailState }

func (n names) Account(id int64) string {
	for _, a := range n.s.Accounts {
		if a.ID == id {
			return a.Email
		}
	}
	return ""
}

func (n names) Mailbox(id int64) string {
	return mail.SourceTitle(mail.Source{Kind: mail.Mailbox, ID: id}, mail.Names{Accounts: n.s.Accounts, Mailboxes: n.s.Mailboxes})
}

func (w *Window) addActivity(e api.ActivityEntry) {
	source, text := activity.Line(e, names{w.mail.State()})
	level := frostui.LevelInfo
	switch e.Level {
	case api.ActivityLevelDebug:
		level = frostui.LevelDebug
	case api.ActivityLevelWarn:
		level = frostui.LevelWarn
	case api.ActivityLevelError:
		level = frostui.LevelError
	}
	w.console.Add(level, source, text)
	if e.AccountID != nil && e.Level != api.ActivityLevelDebug {
		w.mu.Lock()
		w.latest[*e.AccountID] = text
		w.mu.Unlock()
	}
}

// View draws the window.
func (w *Window) View(c *ui.Context) {
	frostui.Adapt(c)
	w.bindKeys(c)
	for _, t := range w.toasts {
		c.AddToast(t)
	}
	w.toasts = nil
	w.shell.View(c, frame.Spec{
		Bar:       func() { w.bar(c) },
		Actions:   func() { w.actions(c) },
		Menu:      w.menu,
		Content:   func() { w.panes(c) },
		Commands:  w.palette(),
		Status:    w.status(),
		StatusEnd: w.statusEnd(),
		Console:   w.console,
	})
}

func (w *Window) bindKeys(c *ui.Context) {
	for _, b := range commands.Bindings {
		run, ok := w.handlers[b.Command]
		if !ok {
			continue
		}
		mods, key, ok := keyOf(b.Key)
		if !ok {
			continue
		}
		k := b.Key
		c.OnWindowShortcut(mods, key, func() {
			if _, ok := commands.For(k, w.searchFocused); ok {
				run()
			}
		})
	}
}

// palette lists the current module's built commands.
func (w *Window) palette() []frostui.Command {
	selected := len(w.selectedIDs())
	var out []frostui.Command
	for _, it := range commands.Items(commands.Mail, selected) {
		run, ok := w.handlers[it.Command]
		if !ok {
			continue
		}
		cmd := frostui.Command{ID: string(it.Command), Title: it.Name, Group: it.Group, Run: run,
			Disabled: it.Reason != "", Reason: it.Reason}
		if len(it.Keys) > 0 {
			cmd.Keys = it.Keys[0].Parts()
		}
		out = append(out, cmd)
	}
	return out
}

func (w *Window) actions(c *ui.Context) {
	b := frostui.IconButton(c, "utilities-terminal", "Activity (Ctrl J)").Selected(w.shell.Console.Open)
	if b.Clicked() {
		w.handlers[commands.Activity]()
	}
}

func (w *Window) menu(m *ui.Menu) {
	if m.Item("Keyboard Shortcuts").Chosen() {
		w.showShortcuts()
	}
	if m.Item("About Frostmail").Chosen() {
		w.showAbout()
	}
}

func (w *Window) mailNames() mail.Names {
	st := w.mail.State()
	n := mail.Names{Accounts: st.Accounts, Mailboxes: st.Mailboxes, VIPs: st.VIPs, Smart: st.Smart}
	if st.Settings != nil {
		n.FlagNames = st.Settings.FlagNames
	}
	return n
}

func (w *Window) panes(c *ui.Context) {
	list := func() {
		ui.Split(c, &w.listWidth, func() { w.messages(c) }, func() { w.reader(c) }).Grow(1)
	}
	if w.sidebarHidden {
		list()
		return
	}
	ui.Split(c, &w.sidebarWidth, func() { w.sidebar(c) }, list).Grow(1)
}

func (w *Window) reader(c *ui.Context) {
	frostui.Empty(c, "mail-read", "No Message Selected", "")
}

func (w *Window) connectingText() string {
	if w.state.Err != nil {
		return "Reconnecting to maild…"
	}
	return "Connecting to maild…"
}

// status lists the connection first, then each account's sync, then the
// Outbox (plan 0015, shared rule 2).
func (w *Window) status() []frostui.StatusItem {
	out := []frostui.StatusItem{w.connectionItem()}
	st := w.mail.State()
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, a := range st.Accounts {
		s, ok := st.Sync[a.ID]
		if !ok && w.latest[a.ID] == "" {
			continue
		}
		item := frostui.StatusItem{Text: a.Email + ": " + syncText(s, w.latest[a.ID]), Progress: -1}
		switch s.Phase {
		case api.SyncPhaseOffline, api.SyncPhaseFailed:
			item.Tone = frostui.Warning
		case api.SyncPhaseUnauthorized:
			item.Tone = frostui.Danger
		}
		item.OnClick = func() { w.shell.Console.Open = true }
		out = append(out, item)
	}
	if n := len(st.Outbox); n > 0 {
		out = append(out, frostui.StatusItem{Text: fmt.Sprintf("Outbox %d", n), Progress: -1})
	}
	return out
}

func (w *Window) connectionItem() frostui.StatusItem {
	it := frostui.StatusItem{Text: "● " + w.state.Target.String(), Progress: -1}
	if !w.state.Connected {
		it.Tone = frostui.Warning
		it.Text = "● Reconnecting to maild…"
		it.Tooltip = w.state.Target.String()
		if w.state.Err != nil {
			it.Tooltip += ": " + w.state.Err.Error()
		}
	}
	return it
}

func (w *Window) statusEnd() []frostui.StatusItem {
	if w.view == nil {
		return nil
	}
	return []frostui.StatusItem{{Text: plural(w.view.Count(), "message", "messages"), Progress: -1}}
}

func (w *Window) showShortcuts() {
	var entries []frostui.ShortcutEntry
	for _, it := range commands.Items(commands.Mail, 1) {
		if len(it.Keys) > 0 {
			entries = append(entries, frostui.ShortcutEntry{Group: it.Group, Keys: it.Keys[0].Parts(), Title: it.Name})
		}
	}
	w.shell.Info.Show("Keyboard Shortcuts", frostui.ShortcutsBody(entries))
}

func (w *Window) showAbout() {
	w.shell.Info.Show("About Frostmail", frostui.AboutBody(frostui.About{Name: "Frostmail", Version: w.o.Client}))
}

func syncText(s api.SyncStatus, latest string) string {
	switch s.Phase {
	case api.SyncPhaseConnecting:
		return "connecting"
	case api.SyncPhaseListing:
		return "listing mailboxes"
	case api.SyncPhaseSyncing:
		if s.Total > 0 {
			return fmt.Sprintf("syncing %d of %d", s.Done, s.Total)
		}
		return "syncing"
	case api.SyncPhaseOffline:
		return "offline"
	case api.SyncPhaseUnauthorized:
		return "sign in again"
	case api.SyncPhaseFailed:
		return "failed"
	}
	if latest != "" {
		return latest
	}
	return "up to date"
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
