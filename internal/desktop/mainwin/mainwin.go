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
	"github.com/frostyard/frostmail/internal/desktop/compose"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/organize"
	"github.com/frostyard/frostmail/internal/desktop/pim"
	"github.com/frostyard/frostmail/internal/desktop/settings"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// The panes' IDs and widths (docs/specs/ui.md, Layout).
const (
	paneSidebar = "sidebar"
	paneList    = "list"
	paneReader  = "reader"

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
	// NewFrame makes a page for a message's body; nil shows none, as in
	// the tests that do not need one.
	NewFrame func() (uikit.Frame, error)
	// CacheDir is maild's cache, whose parts folder holds attachments.
	CacheDir string
	// DownloadDir is where attachments from lodge are downloaded.
	DownloadDir string
	// Launch and LaunchFile open links and files; nil is the system's.
	Launch     func(url string) error
	LaunchFile func(path string) error
	// SaveDialog asks where to save; "" for cancelled. It runs on a
	// worker. filter and extension may be empty.
	SaveDialog func(title, name, filter, extension string) (string, error)
	// OpenWindow shows or raises one of Calendar's windows by key;
	// CloseWindow closes it, and SetWindowTitle retitles it. nil for none.
	OpenWindow     func(pim.WindowSpec)
	CloseWindow    func(key string)
	SetWindowTitle func(key, title string)
	// OpenImage asks for a contact's photo, on a worker; "" for cancelled.
	OpenImage func() (string, error)
	// StateFile keeps the window's layout between runs; "" keeps none.
	StateFile string
	// Settings is the settings model, which the connection feeds and whose
	// welcome shows in place of the panes; nil for none.
	Settings *settings.Window
	// OpenSettings shows the settings window on a pane; "" keeps its pane.
	OpenSettings func(pane string)
	// SocketLost records that maild's local socket ended under the app
	// (problem records, ADR-0049); nil records nothing.
	SocketLost func(err error)
	// OpenCompose shows a draft's window; fresh says it was just made for
	// that window. nil for none.
	OpenCompose func(c *api.Client, draft int64, fresh bool)
	// Raise brings the window to the front, on the UI thread; nil for none.
	Raise func()
}

// Window is the main window's state.
type Window struct {
	o       Options
	session *conn.Session
	waker   *frostui.Waker
	mail    *live.Mail
	act     *live.Activity
	shell   frostui.PaneShell
	table   *frostui.CommandTable
	sources frostui.SourceList[string]
	console *frostui.Console
	daemon  conn.Target // the session goroutine's: the maild the state below is of

	// The UI thread's.
	gen           uint64 // counts the changes of maild
	client        *api.Client
	events        *live.Events
	view          *live.View
	opened        string // the open view's query
	q             mail.ListState
	searchDraft   string
	searchToken   int
	focusSearch   bool
	listWidth     float32
	tree          map[string]mail.Item // the sidebar's rows, by key
	kids          map[string][]string  // a row's children, for those with any
	branches      map[string]bool      // the branches opened once
	list          ui.ListState
	chosen        ui.Selection[int64]
	selected      int
	lastIndex     int64
	undo          *mail.Undo            // one maild's: replaced with the rest on a change
	deleting      map[int64]*heldDelete // deletes the list has not shown yet
	contactPhotos bool
	photos        *live.Photos
	bitmaps       map[string]*ui.Bitmap
	state         conn.State
	connected1    bool // connected at least once
	conv          *conv
	rawSource     *rawSourceSheet
	find          findState
	readerScroll  ui.ScrollState
	readerHeight  float32
	readerFocused bool
	focusPane     string              // the pane Tab asked to focus
	pane          string              // the pane holding the focus
	revealing     *api.MessageSummary // shown and selected whether the list has it or not
	pendingOpen   int64               // a message to show once connected
	handlers      map[commands.Command]func()
	toasts        []ui.Toast
	onFrame       []func(c *ui.Context) // what needs the window's context
	later         compose.TimeSheet
	pim           *pim.Module
	module        commands.Module
	tableModule   commands.Module
	todoOpen      bool
	smartSheet    *organize.SmartSheet
	mailboxSheet  *organize.MailboxSheet
	redirect      *redirectSheet

	// How long an action may take, and how often and how long a delete
	// whose end is unknown is read back (confirmDeletes).
	timeout, confirmEvery, confirmFor time.Duration

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
	w := &Window{o: o, listWidth: listDefault, selected: -1, table: frostui.NewCommandTable(), branches: map[string]bool{},
		q:       mail.ListState{Source: mail.Source{Kind: mail.AllInboxes}, Conversations: true, Filter: mail.FilterAll, Sort: api.ViewSortDate, SearchAll: true},
		console: frostui.NewConsole(5000), latest: map[int64]string{}, state: conn.State{Target: o.Target},
		daemon: o.Target, undo: &mail.Undo{}, deleting: map[int64]*heldDelete{},
		timeout: actionTimeout, confirmEvery: time.Second, confirmFor: actionTimeout}
	w.waker = frostui.NewWaker(o.Post, o.Wake)
	w.shell.Dialogs = &frostui.Dialogs{}
	w.shell.Console.Title = "Activity"
	w.shell.Console.Subtitle = "what maild does in the background"
	w.shell.Console.EmptyNote = "What maild does shows here as it happens."
	w.shell.Console.Offered = 1<<frostui.LevelDebug | 1<<frostui.LevelInfo | 1<<frostui.LevelWarn | 1<<frostui.LevelError
	// The main window has no text editor: its one-line fields leave Ctrl K
	// to the palette.
	w.table.Editing = func(*ui.Context) bool { return false }
	w.setUpSources()
	w.shell.Palette.Sources = w.paletteSources()
	w.newSheets()
	w.module, w.tableModule = commands.Mail, commands.Mail
	w.pim = pim.New(pim.Options{Post: o.Post, Wake: w.waker.Wake, Now: o.Now, Dialogs: w.shell.Dialogs, Toast: w.pimToast,
		Compose: w.composeTo, OpenMessage: w.showMessage, OpenWindow: w.openWindow, CloseWindow: o.CloseWindow,
		SetWindowTitle: o.SetWindowTitle, ShowModule: w.setModule, OpenImage: o.OpenImage})
	w.loadState()
	w.mail = live.NewMail(w.waker.Wake)
	w.act = live.NewActivity(w.addActivity)
	w.console.SetNotify(w.waker.Wake)
	w.session = conn.NewSession(o.Target, conn.Options{Client: o.Client, Retry: o.Retry,
		OnConnect: w.connected, OnChange: w.changed})
	w.handlers = map[commands.Command]func(){
		commands.ToggleSidebar: func() { w.shell.Panes.Toggle(paneSidebar) },
		commands.AllInboxes: func() {
			w.setModule(commands.Mail)
			w.setSource(mail.Source{Kind: mail.AllInboxes})
		},
		commands.ShowCalendar: func() { w.setModule(commands.Calendar) },
		commands.ShowPeople:   func() { w.setModule(commands.People) },
		commands.ShowTasks:    func() { w.setModule(commands.Tasks) },
		commands.GetMail:      w.getMail,
		commands.Shortcuts:    w.showShortcuts,
		commands.About:        w.showAbout,
		commands.Undo:         w.runUndo,
		commands.Archive:      func() { w.archive(w.selectedIDs()) },
		commands.Delete:       w.deleteKey,
		commands.Junk:         func() { w.toggleSpam(w.selectedIDs()) },
		commands.ToggleRead:   func() { w.toggleRead(w.selectedIDs()) },
		commands.ToggleFlag:   func() { w.toggleFlag(w.selectedIDs()) },
		commands.FocusSearch:  func() { w.focusSearch = true },
		commands.Escape:       w.escape,
		commands.Find:         w.startFind,
		commands.Print:        w.printMessage,
		commands.RawSource:    w.rawSourceCommand,
		commands.AllHeaders:   w.allHeadersCommand,
		commands.Reply:        func() { w.reply(api.DraftKindReply) },
		commands.ReplyAll:     func() { w.reply(api.DraftKindReplyall) },
		commands.Forward:      func() { w.forward() },
		commands.Compose:      w.compose,
		commands.PageDown:     func() { w.pageReader(1) },
		commands.PageUp:       func() { w.pageReader(-1) },
		commands.Open:         w.openSelected,
		commands.Redirect:     w.askRedirect,
		commands.NextPane:     func() { w.cyclePane(1) },
		commands.PreviousPane: func() { w.cyclePane(-1) },
	}
	if o.OpenSettings != nil {
		w.handlers[commands.Settings] = func() { o.OpenSettings("") }
	}
	return w
}

// Client is the connected client, nil while there is none. It belongs to
// the UI thread.
func (w *Window) Client() *api.Client { return w.client }

// ConnectionState is the session's state now.
func (w *Window) ConnectionState() conn.State { return w.session.State() }

// SetTarget moves the session to another maild.
func (w *Window) SetTarget(t conn.Target) { w.session.SetTarget(t) }

// MailState is the mail the window holds.
func (w *Window) MailState() live.MailState { return w.mail.State() }

// OrganizeContext is what the condition and action editors need.
func (w *Window) OrganizeContext() organize.Context { return w.organizeContext() }

// Raise brings the window to the front, on the UI thread.
func (w *Window) Raise() {
	if w.o.Raise != nil {
		w.o.Raise()
	}
}

// ShowProblem shows a toast saying what went wrong.
func (w *Window) ShowProblem(title string, err error) { w.toast(title, err) }

// Start connects and keeps the connection until ctx ends.
func (w *Window) Start(ctx context.Context) { go w.session.Run(ctx) }

// connected runs on the session's goroutine for each new connection.
func (w *Window) connected(ctx context.Context, c *api.Client, _ *api.Hello) error {
	w.follow(w.session.Target())
	events := live.NewEvents()
	go events.Run(c)
	events.On(w.readerEvent)
	if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		return err
	}
	if err := w.mail.Connect(ctx, c, events); err != nil {
		return err
	}
	if err := w.act.Connect(ctx, c, events); err != nil {
		return err
	}
	if err := w.pim.Connect(ctx, c, events); err != nil {
		return err
	}
	if w.o.Settings != nil {
		if err := w.o.Settings.Connect(ctx, c, events); err != nil {
			return err
		}
	}
	w.o.Post(func() {
		w.closeView()
		w.client, w.events = c, events
		w.photos, w.bitmaps = live.NewPhotos(connContext(c), c, w.waker.Wake), map[string]*ui.Bitmap{}
		w.openView()
		w.deliverOpen()
	})
	return nil
}

// connContext returns a context that ends with the connection c.
func connContext(c *api.Client) context.Context {
	out, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		<-c.Done()
	}()
	return out
}

// follow notes the maild the session is on, on the session's goroutine.
// When it is another one, what the window holds of the last goes, before
// any of the new one's answers: the same ID means other mail there.
func (w *Window) follow(t conn.Target) {
	if t.Same(w.daemon) {
		return
	}
	w.daemon = t
	w.mail.Forget()
	w.mu.Lock()
	w.latest = map[int64]string{}
	w.mu.Unlock()
	w.o.Post(w.leaveDaemon)
}

// leaveDaemon resets what names the last maild's messages and mailboxes.
// The view choices stay, as they name none. A worker still running keeps
// the undo stack and the sheet it began with, and reaches neither of these.
func (w *Window) leaveDaemon() {
	w.gen++
	w.closeView()
	w.closeFind() // before the pages it searches close
	w.closeConv()
	w.pim.Leave()
	w.undo = &mail.Undo{}
	w.deleting = map[int64]*heldDelete{}
	w.q.Source, w.q.Search, w.searchDraft = mail.Source{Kind: mail.AllInboxes}, "", ""
	w.searchToken++
	w.clearSelection()
	w.revealing, w.pendingOpen = nil, 0
	w.branches = map[string]bool{}
	w.onFrame = nil
	w.rawSource, w.redirect = nil, nil
	// Its query, its results and what they were asked go: they name the
	// last maild's mail (its items do nothing now, chosen or not).
	w.shell.Palette.Open = false
	w.later = compose.TimeSheet{}
	w.newSheets()
	*w.shell.Dialogs = frostui.Dialogs{} // the same one Calendar's module holds
}

// newSheets makes the organize sheets, closed.
func (w *Window) newSheets() {
	deps := organize.Deps{Post: w.o.Post, Client: func() *api.Client { return w.client }, Context: w.organizeContext}
	gen := w.gen
	w.smartSheet = &organize.SmartSheet{Deps: deps, Created: func(id int64) {
		if gen == w.gen {
			w.setSource(mail.Source{Kind: mail.Smart, ID: id})
		}
	}}
	w.mailboxSheet = &organize.MailboxSheet{Deps: deps}
}

// changed runs on the session's goroutine for each change of state.
func (w *Window) changed(st conn.State) {
	if !st.Connected {
		w.mail.Disconnect()
		w.pim.Disconnect()
		if w.o.Settings != nil {
			w.o.Settings.Disconnect()
		}
	}
	w.follow(st.Target)
	w.o.Post(func() {
		if !st.Connected {
			w.closeView()
			w.client, w.events = nil, nil
			// Lodge's WebSocket never counts: its losses are the network's.
			if w.state.Connected && !st.Target.Remote() && st.Err != nil && w.o.SocketLost != nil {
				w.o.SocketLost(st.Err)
			}
		}
		w.state = st
		w.connected1 = w.connected1 || st.Connected
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
	// Before anything can act on the selection, in whichever module.
	w.repairSelection()
	// First, so that what a palette command changes shows in this frame:
	// the rest of View reads that state before the shell would run it.
	w.shell.Palette.RunPending()
	frostui.Adapt(c)
	for _, t := range w.toasts {
		c.AddToast(t)
	}
	w.toasts = nil
	for _, fn := range w.onFrame {
		fn(c)
	}
	w.onFrame = nil
	w.syncReveal()
	w.syncTable()
	conn := w.connection()
	frame := frostui.PaneFrame{
		Menu:        w.table.Menu(string(commands.Settings), "", string(commands.Shortcuts), string(commands.About)),
		Table:       w.table,
		Connection:  &conn,
		Status:      w.status(),
		StatusEnd:   w.statusEnd(),
		ConsoleData: w.console,
	}
	if w.o.Settings != nil && w.o.Settings.NeedsFirstRun() {
		frame.Content = func() { w.o.Settings.FirstRun(c) }
	} else {
		frame.SidebarPane, frame.Panes = paneSidebar, w.modulePanes(c)
	}
	w.shell.View(c, frame)
	w.rawSourceView(c)
	w.later.View(c, w.o.Now())
	w.smartSheet.View(c)
	w.mailboxSheet.View(c)
	w.redirectView(c)
}

func (w *Window) mailNames() mail.Names {
	st := w.mail.State()
	n := mail.Names{Accounts: st.Accounts, Mailboxes: st.Mailboxes, VIPs: st.VIPs, Smart: st.Smart}
	if st.Settings != nil {
		n.FlagNames = st.Settings.FlagNames
	}
	return n
}

func (w *Window) panes(c *ui.Context) []frostui.Pane {
	return []frostui.Pane{
		{ID: paneSidebar, Label: "Mailboxes", Width: sidebarDefault, Min: 160, Max: 320, Collapsible: true,
			Bar: func() { w.sidebarBar(c) }, Content: func() { w.sidebar(c) }},
		{ID: paneList, Label: "Messages", Width: listDefault, Min: 280, Max: 560,
			Bar: func() { w.listBar(c) }, Content: func() { w.messages(c) }},
		{ID: paneReader, Label: "Reader", Min: 360, Content: func() { w.readerAndToDo(c) }},
	}
}

// connection says how the window stands with maild, for the status bar
// and the connecting screen.
func (w *Window) connection() frostui.Connection {
	st := w.state
	out := frostui.Connection{Daemon: "maild", Host: st.Target.Host(),
		OnClick: w.showConnection, OnRetry: w.session.Retry, OnDetails: w.showConnection}
	switch {
	case st.Connected:
		out.State = frostui.Connected
	case w.connected1:
		out.State = frostui.Reconnecting
	default:
		out.State = frostui.Connecting
	}
	if st.Err != nil {
		out.Reason = st.Err.Error()
	}
	return out
}

func (w *Window) showConnection() {
	st := w.state
	w.shell.Info.Show("Connection", func(c *ui.Context) {
		rows := []frostui.KeyValue{{Key: "Connects to", Value: st.Target.String()}}
		if st.Hello != nil {
			rows = append(rows, frostui.KeyValue{Key: "Server", Value: st.Hello.Server})
		}
		if st.Err != nil {
			rows = append(rows, frostui.KeyValue{Key: "Last error", Value: st.Err.Error()})
		}
		frostui.KeyValues(c, rows)
		if w.o.OpenSettings != nil && ui.Button(c, "Server Settings…").Clicked() {
			w.shell.Info.Open = false
			w.o.OpenSettings(settings.PaneServer)
		}
	})
}

// status lists each account's sync, then the Outbox, after the connection
// (plan 0015, shared rule 2).
func (w *Window) status() []frostui.StatusItem {
	var out []frostui.StatusItem
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

func (w *Window) statusEnd() []frostui.StatusItem {
	if w.view == nil || w.module != commands.Mail {
		return nil
	}
	return []frostui.StatusItem{{Text: plural(w.view.Count(), "message", "messages"), Progress: -1}}
}

func (w *Window) showShortcuts() {
	w.shell.Info.Show("Keyboard Shortcuts", frostui.ShortcutsBody(w.table.Shortcuts()))
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
