// Package settings is the desktop app's settings window, its first run, and
// the change between this computer's maild and lodge's
// (docs/specs/settings-ui.md; plan 0015, Phase 8). All of its state belongs
// to the UI thread; goroutines hand results back through Post.
package settings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/live"
)

// The window's size, and the smallest it may be (docs/specs/settings-ui.md,
// Window).
const (
	Width     = 760
	Height    = 560
	MinWidth  = 640
	MinHeight = 440
)

// The panes, for Show.
const (
	PaneGeneral    = "general"
	PaneAccounts   = "accounts"
	PaneSignatures = "signatures"
	PaneSnippets   = "snippets"
	PaneRules      = "rules"
	PaneSignIn     = "signin"
	PaneServer     = "server"
)

// callTimeout bounds one request of a pane, which can be several calls to
// maild, as adding an account is.
const callTimeout = 2 * time.Minute

// Options configure a Window.
type Options struct {
	// Post runs a function on the UI thread and redraws.
	Post func(func())
	// Wake asks for a redraw; it may be nil.
	Wake func()
	// Now is the clock [time.Now].
	Now func() time.Time
	// Toast shows what went wrong where no pane can say; it may be nil.
	Toast func(title string, err error)
	// ConfigFile is app.json's path.
	ConfigFile string
	// Facts are the Server pane's local facts (FactsFrom).
	Facts ServerFacts
	// LoadFacts, when set, reads them afresh each time the Server pane
	// opens and after each change of mode, in place of Facts: a first start
	// of this computer's maild makes its database.
	LoadFacts func() ServerFacts
	// SetTarget moves the session to the maild now chosen.
	SetTarget func(conn.Target)
	// StopLocal stops this computer's maild and returns once it has
	// stopped: mailctl shutdown.
	StopLocal func(ctx context.Context) error
	// StartLocal starts this computer's maild and returns once it answers:
	// frostmail --start-maild. Its error is the launcher's last line.
	StartLocal func(ctx context.Context) error
	// OpenURL opens a sign-in in the system browser.
	OpenURL func(url string) error
	// EditRule opens the rule sheet: on rule, or a new rule for nil.
	EditRule func(rule *api.Rule)
	// Socket is this computer's maild, which a change to it connects to;
	// empty is FROSTMAIL_SOCKET or the default socket.
	Socket string
	// OpenSettings shows the settings window on a pane, focusing it when
	// it is open. First run calls it; nil only switches the pane.
	OpenSettings func(pane string)
	// Connection is the session's state, for the connecting text and to
	// tell lodge's maild from this computer's; nil reads app.json.
	Connection func() conn.State
	// Announce shows the notes of message.announced, which a maild with no
	// desktop to notify on sends; it runs on the events' goroutine, and may
	// be nil.
	Announce func(accountID int64, notes []api.Announcement)
}

// session is one connection: its client, and what ends with it.
type session struct {
	client *api.Client
	ctx    context.Context
	cancel context.CancelFunc
	off    func()
}

func (s *session) stop() {
	s.cancel()
	if s.off != nil {
		s.off()
	}
}

// data is what several panes read, from maild.
type data struct {
	accounts []api.Account
	loaded   bool // the first accounts answered
	settings *api.Settings
	smart    []api.SmartMailbox
	blocked  []api.BlockedSender
	rules    []api.Rule
}

// loads are the reads of data.
type loads struct {
	accounts, settings, smart, blocked, rules loader
}

// Window is the settings window's state.
type Window struct {
	o       Options
	shell   frostui.PaneShell
	table   *frostui.CommandTable
	layout  frostui.Settings
	dialogs frostui.Dialogs
	facts   ServerFacts

	cn        *session
	connected bool // connected at least once
	data      data
	loads     *loads
	autoOpen  bool // this connection found no account, and first run has yet to say so

	open string // the pane mounted
	gen  *generalPane
	acct *accountsPane
	sig  *signaturesPane
	snip *snippetsPane
	rule *rulesPane
	auth *signInPane

	// These stay across connections: a change of mode ends the connection
	// that started it.
	server   serverPane
	firstRun firstRun
}

// New returns a window that is not connected yet.
func New(o Options) *Window {
	if o.Now == nil {
		o.Now = time.Now
	}
	w := &Window{o: o, loads: &loads{}, table: frostui.NewCommandTable(), facts: o.Facts}
	w.shell.Dialogs = &w.dialogs
	// No text editor here binds Ctrl K: the fields leave it to the palette.
	w.table.Editing = func(*ui.Context) bool { return false }
	w.layout.Selected = PaneAccounts
	w.open = PaneAccounts
	w.mount(PaneAccounts)
	w.readServer(false)
	w.firstRun.dismissed = moveNoteDismissed(o.ConfigFile)
	return w
}

// Show switches the window to a pane; unknown names are ignored. It does
// not open the window.
func (w *Window) Show(pane string) {
	switch pane {
	case PaneGeneral, PaneAccounts, PaneSignatures, PaneSnippets, PaneRules, PaneSignIn, PaneServer:
		w.layout.Selected = pane
	}
}

// Connect shows the window on a new connection, whose events the caller
// subscribed to, and loads what the panes read. It may run on any
// goroutine. What the panes held of the last connection goes: another
// maild's ids mean other things here.
func (w *Window) Connect(ctx context.Context, c *api.Client, events *live.Events) error {
	cn := &session{client: c}
	cn.ctx, cn.cancel = context.WithCancel(context.Background())
	// Listen first, so a change made during the loads reloads after them.
	// One that comes before the window takes this connection precedes
	// every read, which sees it.
	cn.off = events.On(func(ev api.Event) {
		if e, ok := ev.(api.MessageAnnounced); ok && w.o.Announce != nil {
			w.o.Announce(e.AccountID, e.Notes)
		}
		if wanted(ev) {
			w.o.Post(func() { w.event(cn, ev) })
		}
	})
	w.o.Post(func() { w.adopt(cn) })
	accounts, err := c.Account().List(ctx, &api.AccountListParams{})
	if err != nil {
		cn.stop()
		w.o.Post(func() { w.leave(cn) })
		return err
	}
	w.o.Post(func() { w.firstAccounts(cn, accounts) })
	return nil
}

// Disconnect drops the connection that ended; it may run on any goroutine.
// The Server pane stays usable.
func (w *Window) Disconnect() {
	w.o.Post(func() { w.leave(w.cn) })
}

func (w *Window) adopt(cn *session) {
	if w.cn != nil {
		w.cn.stop()
	}
	w.cn, w.connected = cn, true
	w.data, w.loads, w.autoOpen = data{}, &loads{}, false
	w.mount(w.open)
}

func (w *Window) leave(cn *session) {
	if cn == nil || w.cn != cn {
		return
	}
	cn.stop()
	w.cn = nil
}

// firstAccounts takes the accounts Connect read, unless an event read them
// since: its answer is newer.
func (w *Window) firstAccounts(cn *session, accounts []api.Account) {
	if w.cn != cn {
		return
	}
	if !w.loads.accounts.started {
		w.loads.accounts.started = true
		w.setAccounts(accounts)
	}
	w.autoOpen = len(accounts) == 0
	w.loadSettings()
	w.loadSmart()
	w.loadBlocked()
	w.loadRules()
	w.syncPane()
}

// wanted says which events the window acts on, so that the rest are never
// posted to the UI thread.
func wanted(ev api.Event) bool {
	switch ev.(type) {
	case api.AccountChanged, api.MailboxChanged, api.SettingsChanged, api.SmartChanged, api.BlockChanged, api.RuleChanged, api.SnippetChanged:
		return true
	}
	return false
}

func (w *Window) event(cn *session, ev api.Event) {
	if w.cn != cn {
		return
	}
	switch e := ev.(type) {
	case api.AccountChanged:
		w.loadAccounts()
		w.accountChanged(e.ID)
	case api.MailboxChanged:
		w.loadEraseNames(w.acct)
	case api.SettingsChanged:
		w.loadSettings()
	case api.SmartChanged:
		w.loadSmart()
	case api.BlockChanged:
		w.loadBlocked()
	case api.RuleChanged:
		w.loadRules()
	case api.SnippetChanged:
		w.loadSnippets()
	}
}

// loader runs one read at a time: a change during a read reads once more
// after it, and only the last answer counts.
type loader struct {
	started bool
	running bool
	again   bool
	read    func(context.Context, *api.Client) func()
}

// run reads on a worker; read returns what to do with the answer, on the UI
// thread. A slow answer can arrive after a newer change: it is dropped.
func (l *loader) run(w *Window, read func(context.Context, *api.Client) func()) {
	cn := w.cn
	if cn == nil {
		return
	}
	l.started, l.read = true, read
	if l.running {
		l.again = true
		return
	}
	l.running = true
	go func() {
		ctx, cancel := context.WithTimeout(cn.ctx, callTimeout)
		defer cancel()
		apply := read(ctx, cn.client)
		w.o.Post(func() {
			l.running = false
			if w.cn != cn {
				return
			}
			if l.again {
				l.again = false
				l.run(w, l.read)
				return
			}
			apply()
		})
	}()
}

// load is a read whose failure leaves what was shown; set takes the answer.
func load[T any](w *Window, l *loader, what string, call func(context.Context, *api.Client) (T, error), set func(T)) {
	l.run(w, func(ctx context.Context, c *api.Client) func() {
		v, err := call(ctx, c)
		return func() {
			if err != nil {
				slog.Warn("settings: "+what, "err", err)
				return
			}
			set(v)
		}
	})
}

func (w *Window) loadAccounts() {
	load(w, &w.loads.accounts, "account.list", func(ctx context.Context, c *api.Client) ([]api.Account, error) {
		return c.Account().List(ctx, &api.AccountListParams{})
	}, w.setAccounts)
}

func (w *Window) setAccounts(accounts []api.Account) {
	w.data.accounts, w.data.loaded = accounts, true
	w.syncAccounts(w.acct)
	w.loadIdentities()
}

func (w *Window) loadSettings() {
	load(w, &w.loads.settings, "settings.get", func(ctx context.Context, c *api.Client) (*api.Settings, error) {
		return c.Settings().Get(ctx, &api.SettingsGetParams{})
	}, func(s *api.Settings) { w.data.settings = s })
}

func (w *Window) loadSmart() {
	load(w, &w.loads.smart, "smart.list", func(ctx context.Context, c *api.Client) ([]api.SmartMailbox, error) {
		return c.Smart().List(ctx, &api.SmartListParams{})
	}, func(s []api.SmartMailbox) { w.data.smart = s })
}

func (w *Window) loadBlocked() {
	load(w, &w.loads.blocked, "block.list", func(ctx context.Context, c *api.Client) ([]api.BlockedSender, error) {
		return c.Block().List(ctx, &api.BlockListParams{})
	}, func(s []api.BlockedSender) { w.data.blocked = s })
}

func (w *Window) loadRules() {
	load(w, &w.loads.rules, "rule.list", func(ctx context.Context, c *api.Client) ([]api.Rule, error) {
		return c.Rule().List(ctx, &api.RuleListParams{})
	}, func(s []api.Rule) { w.data.rules = s; w.rulesChanged() })
}

// request is a pane's one request at a time, and its last error.
type request struct {
	busy bool
	err  string
}

// run runs f on a worker while the pane shows busy. explain may word its
// error; nil, or "" from it, keeps maild's own message.
func (w *Window) run(r *request, f func(ctx context.Context, c *api.Client) error, explain func(error) string) {
	cn := w.cn
	if cn == nil || r.busy {
		return
	}
	r.busy, r.err = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(cn.ctx, callTimeout)
		defer cancel()
		err := f(ctx, cn.client)
		w.o.Post(func() {
			r.busy = false
			if err == nil {
				return
			}
			r.err = message(err)
			if explain != nil {
				if text := explain(err); text != "" {
					r.err = text
				}
			}
		})
	}()
}

// message is what an error says to the user: maild's own words, without
// the code in front of them.
func message(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Message
	}
	return err.Error()
}

// noClient words a sign-in that has no OAuth client to use.
func noClient(err error) string {
	var e *api.Error
	if errors.As(err, &e) && e.Code == api.CodeUnavailable {
		return "Set up your Google client under Sign-In first."
	}
	return ""
}

// ask confirms a change, on the maild it was asked about: an answer that
// comes after a change of connection does nothing, as another maild's ids
// mean other things.
func (w *Window) ask(req frostui.Confirm) {
	cn, confirm := w.cn, req.OnConfirm
	req.OnConfirm = func() {
		if w.cn == cn {
			confirm()
		}
	}
	w.dialogs.Ask(req)
}

func (w *Window) toast(title string, err error) {
	if w.o.Toast != nil {
		w.o.Toast(title, err)
	}
}

// mount gives a pane fresh state, as opening it does, and loads what only
// it reads.
func (w *Window) mount(pane string) {
	w.open = pane
	w.gen, w.acct, w.sig, w.snip, w.rule, w.auth = nil, nil, nil, nil, nil, nil
	switch pane {
	case PaneGeneral:
		w.gen = &generalPane{focused: -1}
	case PaneAccounts:
		w.acct = newAccountsPane()
	case PaneSignatures:
		w.sig = &signaturesPane{}
	case PaneSnippets:
		w.snip = &snippetsPane{}
	case PaneRules:
		w.rule = &rulesPane{}
	case PaneSignIn:
		w.auth = &signInPane{}
	case PaneServer:
		w.loadFacts()
	}
	w.syncPane()
}

// syncPane starts the open pane's own reads, once it has a connection and
// knows the accounts.
func (w *Window) syncPane() {
	if w.cn == nil || !w.data.loaded {
		return
	}
	switch w.open {
	case PaneAccounts:
		w.syncAccounts(w.acct)
	case PaneSignatures:
		w.loadIdentities()
	case PaneSnippets:
		w.loadSnippets()
	case PaneSignIn:
		w.loadClient()
	}
}

// View draws the settings window's content.
func (w *Window) View(c *ui.Context) {
	// First, so that what a palette command changes shows in this frame:
	// the rest of View reads that state before the shell would run it.
	w.shell.Palette.RunPending()
	frostui.Adapt(c)
	if w.layout.Selected != w.open {
		w.mount(w.layout.Selected)
	}
	w.expireSignIn(w.acct)
	bar := func() { frostui.Strong(c, "Settings").SingleLine() }
	content := func() { w.layout.View(c, w.pages(c)) }
	w.shell.View(c, frostui.PaneFrame{Table: w.table, Panes: []frostui.Pane{{ID: "settings", Label: "Settings", Bar: bar, Content: content}}})
}

// pages are the window's panes. Without a maild only the Server pane
// works: it asks the shell, not maild.
func (w *Window) pages(c *ui.Context) []frostui.SettingsPage {
	pane := func(body func(*ui.Context)) func(*ui.Context) {
		return func(c *ui.Context) {
			if w.cn == nil || !w.data.loaded {
				w.offline(c)
				return
			}
			body(c)
		}
	}
	general := frostui.SettingsPage{ID: PaneGeneral, Title: "General", Icon: "preferences-system", Body: pane(w.generalBody)}
	if w.cn != nil && w.data.loaded && w.open == PaneGeneral {
		general.Groups = w.generalGroups()
	}
	return []frostui.SettingsPage{
		general,
		{ID: PaneAccounts, Title: "Accounts", Icon: "mail-unread", Body: pane(w.accountsBody)},
		{ID: PaneSignatures, Title: "Signatures", Icon: "document-edit", Body: pane(w.signaturesBody)},
		{ID: PaneSnippets, Title: "Snippets", Icon: "text-x-generic", Body: pane(w.snippetsBody)},
		{ID: PaneRules, Title: "Rules", Icon: "view-list", Body: pane(w.rulesBody)},
		{ID: PaneSignIn, Title: "Sign-In", Icon: "dialog-password", Body: pane(w.signInBody)},
		{ID: PaneServer, Title: "Server", Icon: "network-server", Body: w.serverBody},
	}
}

// offline is a pane without a maild: the main window's connecting text.
func (w *Window) offline(c *ui.Context) {
	ui.Column(c).Padding(frostui.SpaceXL).AlignItems(ui.Center).Children(func() {
		frostui.Muted(c, w.connectingText()).Wrap()
	})
}

func (w *Window) connectingText() string {
	var reason string
	if w.o.Connection != nil {
		if err := w.o.Connection().Err; err != nil {
			reason = err.Error()
		}
	}
	switch {
	case reason != "":
		return "Reconnecting to maild… (" + reason + ")"
	case w.connected:
		return "Reconnecting to maild…"
	}
	return "Connecting to maild…"
}

// onThisComputer reports whether the app reads this computer's maild.
func (w *Window) onThisComputer() bool {
	if w.o.Connection != nil {
		return !w.o.Connection().Target.Remote()
	}
	return w.server.stored == ""
}
