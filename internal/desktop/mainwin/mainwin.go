// Package mainwin is the desktop app's main window: the frame, the panes
// and the commands, over the live data (plan 0015). All of its state
// belongs to the UI thread; goroutines hand results back through Post.
package mainwin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// The panes' widths (docs/specs/ui.md, Layout).
const (
	sidebarDefault = 220
	listDefault    = 360
	allInboxes     = "role:inbox"
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
	source        string
	sidebarWidth  float32
	listWidth     float32
	sidebarHidden bool
	list          ui.ListState
	selected      int
	state         conn.State
	handlers      map[commands.Command]func()

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
	w := &Window{o: o, source: allInboxes, sidebarWidth: sidebarDefault, listWidth: listDefault, selected: -1,
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
		commands.AllInboxes:    func() { w.choose(allInboxes) },
		commands.GetMail:       w.getMail,
		commands.Shortcuts:     w.showShortcuts,
		commands.About:         w.showAbout,
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

// query is the view of the source chosen in the sidebar.
func (w *Window) query() api.ViewQuery {
	if id, ok := strings.CutPrefix(w.source, "mailbox:"); ok {
		if n, err := strconv.ParseInt(id, 10, 64); err == nil {
			return api.ViewQuery{MailboxID: &n}
		}
	}
	role := api.MailboxRoleInbox
	return api.ViewQuery{Role: &role}
}

func (w *Window) openView() {
	if w.client != nil {
		w.view = live.OpenView(w.client, w.events, w.query(), w.waker.Wake)
	}
}

func (w *Window) closeView() {
	if w.view != nil {
		w.view.Close()
		w.view = nil
	}
}

func (w *Window) choose(source string) {
	w.source, w.selected = source, -1
	w.closeView()
	w.openView()
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
	for _, m := range n.s.Mailboxes {
		if m.ID == id {
			return m.Name
		}
	}
	return ""
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
		c.OnWindowShortcut(mods, key, run)
	}
}

// palette lists the current module's built commands.
func (w *Window) palette() []frostui.Command {
	selected := 0
	if w.selected >= 0 {
		selected = 1
	}
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

func (w *Window) bar(c *ui.Context) {
	sidebar := float32(0)
	if !w.sidebarHidden {
		sidebar = w.sidebarWidth - frostui.SpaceM
	}
	ui.Row(c).Width(sidebar).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
		if frostui.IconButton(c, "sidebar-show", "Show or hide the sidebar").Clicked() {
			w.handlers[commands.ToggleSidebar]()
		}
		if frostui.IconButton(c, "mail-send-receive", "Get Mail").Clicked() {
			w.getMail()
		}
	})
	ui.Column(c).Children(func() {
		frostui.Strong(c, w.title()).SingleLine()
		frostui.Caption(c, w.subtitle()).SingleLine()
	})
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

func (w *Window) title() string {
	if w.source == allInboxes {
		return "All Inboxes"
	}
	if id, ok := strings.CutPrefix(w.source, "mailbox:"); ok {
		n, _ := strconv.ParseInt(id, 10, 64)
		if name := (names{w.mail.State()}).Mailbox(n); name != "" {
			return name
		}
	}
	return "Mail"
}

func (w *Window) subtitle() string {
	if w.view == nil {
		return ""
	}
	return plural(w.view.Count(), "message", "messages")
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

func (w *Window) sidebar(c *ui.Context) {
	st := w.mail.State()
	before := w.source
	ui.Sidebar(c, &w.source, func() {
		ui.SidebarSection(c, "Favorites", nil, func() {
			ui.SidebarItem(c, allInboxes, frostui.IconSVG("mail-inbox"), "All Inboxes")
		})
		for _, a := range st.Accounts {
			ui.SidebarSection(c.Key(a.ID), a.Email, nil, func() {
				for _, mb := range st.Mailboxes {
					if mb.AccountID != a.ID {
						continue
					}
					item := ui.SidebarItem(c.Key(mb.ID), "mailbox:"+strconv.FormatInt(mb.ID, 10), nil, mb.Name)
					if mb.Unread > 0 {
						item.Children(func() { ui.Badge(c, strconv.FormatInt(mb.Unread, 10)) })
					}
				}
			})
		}
	}).Grow(1)
	if w.source != before {
		w.choose(w.source)
	}
}

// messages draws the list; its rows ask the view for what shows.
func (w *Window) messages(c *ui.Context) {
	if w.view == nil {
		frostui.Loading(c, w.connectingText())
		return
	}
	v := w.view
	if v.Ready() && v.Count() == 0 {
		frostui.Empty(c, "mail-inbox", "No Messages", "")
		return
	}
	w.list.Selected = &w.selected
	low, high := int64(-1), int64(-1)
	ui.List(c, &w.list, int(v.Count()), func(i int) {
		n := int64(i)
		if low < 0 || n < low {
			low = n
		}
		high = max(high, n)
		r, ok := v.Row(n)
		if !ok {
			uikit.SummaryRow(c, uikit.Summary{})
			return
		}
		uikit.SummaryRow(c, uikit.Summary{Title: sender(r.From), Date: formatDate(r.Date, w.o.Now()),
			Subtitle: r.Subject, Preview: r.Preview, Unread: !r.Flags.Seen})
	}).Grow(1)
	if low >= 0 {
		v.Ensure(low, high+1)
	}
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

func sender(a api.Address) string {
	if a.Name != "" {
		return a.Name
	}
	return a.Address
}

// formatDate writes a row's date: the time today, else the date.
func formatDate(d, now time.Time) string {
	d = d.In(now.Location())
	if y, m, day := d.Date(); y == now.Year() && m == now.Month() && day == now.Day() {
		return d.Format("15:04")
	}
	return d.Format("2006-01-02")
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
