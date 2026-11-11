package settings_test

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/settings"
	"github.com/frostyard/frostmail/internal/discover"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("FROSTUI_ICON_THEME", "Adwaita")
	frostui.SetIcons(frostui.NewIcons(icons.Embedded()))
	os.Exit(m.Run())
}

// noDNS answers no SRV records, so discovery never leaves the test.
type noDNS struct{}

func (noDNS) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	return "", nil, io.EOF
}

// fixture is the window over a real maild, with every effect outside the
// window recorded.
type fixture struct {
	t      *testing.T
	srv    *rpctest.Server
	c      *api.Client // the test's own connection
	loop   *frostuitest.Loop
	win    *settings.Window
	tt     *ui.Tester
	config string
	events *live.Events

	mu      sync.Mutex
	opened  []string
	panes   []string
	rules   []*api.Rule
	targets []conn.Target
	toasts  []string
	state   conn.State
	openErr error
}

type setup struct {
	server  rpctest.Options
	options func(*settings.Options)
	offline bool
	first   func(f *fixture) // before the window connects
}

// newFixture runs the window against a real maild, in a tall window, so
// that a whole form shows.
func newFixture(t *testing.T, s setup) *fixture {
	t.Helper()
	if s.server.Discovery.Get == nil {
		s.server.Discovery = discover.Deps{Get: func(context.Context, string) ([]byte, error) { return nil, io.EOF }, Resolver: noDNS{}}
	}
	f := &fixture{t: t, loop: frostuitest.NewLoop(), config: filepath.Join(t.TempDir(), "frostmail", "app.json")}
	f.state = conn.State{Target: conn.Target{Socket: "/none"}}
	o := settings.Options{Post: f.loop.Post, ConfigFile: f.config, Socket: "/local.sock",
		SetTarget:  func(c conn.Target) { f.record(func() { f.targets = append(f.targets, c) }) },
		StopLocal:  func(context.Context) error { return nil },
		StartLocal: func(context.Context) error { return nil },
		OpenURL: func(u string) error {
			var err error
			f.record(func() { f.opened = append(f.opened, u); err = f.openErr })
			return err
		},
		EditRule:     func(r *api.Rule) { f.record(func() { f.rules = append(f.rules, r) }) },
		OpenSettings: func(p string) { f.record(func() { f.panes = append(f.panes, p) }) },
		Toast:        func(title string, err error) { f.record(func() { f.toasts = append(f.toasts, title) }) },
		Connection:   func() conn.State { f.mu.Lock(); defer f.mu.Unlock(); return f.state },
	}
	if s.options != nil {
		s.options(&o)
	}
	if !s.offline {
		f.srv = rpctest.StartWith(t, s.server)
		f.c = f.srv.Dial(t)
		if s.first != nil {
			s.first(f)
		}
	}
	f.win = settings.New(o)
	f.tt = ui.NewTester(func(c *ui.Context) {
		f.loop.Flush()
		f.win.View(c)
	}, 1000, 2400)
	if !s.offline {
		f.connect()
	}
	return f
}

// connect gives the window a new connection, as the session does.
func (f *fixture) connect() {
	f.t.Helper()
	c := f.srv.Dial(f.t)
	events := live.NewEvents()
	f.events = events
	go events.Run(c)
	if _, err := c.Events().Subscribe(f.t.Context(), &api.EventsSubscribeParams{}); err != nil {
		f.t.Fatal(err)
	}
	if err := f.win.Connect(f.t.Context(), c, events); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) record(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

// until redraws until the window shows what ok wants.
func (f *fixture) until(what string, ok func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		f.tt.Frame()
		if ok() {
			return
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("timed out waiting for %s; the window shows %q", what, f.tt.Texts())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (f *fixture) sees(text string) {
	f.t.Helper()
	f.until("«"+text+"»", func() bool { return f.tt.HasText(text) })
}

func (f *fixture) notSeen(text string) {
	f.t.Helper()
	f.tt.Frame()
	if f.tt.HasText(text) {
		f.t.Errorf("%q shows in %q", text, f.tt.Texts())
	}
}

func (f *fixture) click(name string) {
	f.t.Helper()
	if err := f.tt.Click(name); err != nil {
		f.t.Fatalf("click %q: %v; the window shows %q", name, err, f.tt.Texts())
	}
}

// show opens a pane.
func (f *fixture) show(pane string) {
	f.t.Helper()
	f.win.Show(pane)
	f.tt.Frame()
}

// control is the element of a role named name.
func (f *fixture) control(role ui.Role, name string) ui.AccessInfo {
	f.t.Helper()
	f.tt.Frame()
	a, ok := f.tt.AccessByRole(role, name)
	if !ok {
		f.t.Fatalf("no control %q of role %v in %q", name, role, f.tt.Texts())
	}
	return a
}

func (f *fixture) press(a ui.AccessInfo) {
	f.tt.ClickAt(a.Bounds.X+a.Bounds.W/2, a.Bounds.Y+a.Bounds.H/2)
}

// fill replaces what the field named name holds.
func (f *fixture) fill(name, text string) {
	f.t.Helper()
	f.press(f.control(ui.RoleTextField, name))
	f.tt.Key(ui.Cmd, ui.KeyA)
	if text == "" {
		f.tt.Key(0, ui.KeyBackspace)
		return
	}
	f.tt.Type(text)
}

// choose picks an option of the drop-down named name.
func (f *fixture) choose(name, option string) {
	f.t.Helper()
	f.press(f.control(ui.RolePopUpButton, name))
	f.press(f.control(ui.RoleOption, option))
}

// open selects the account with this name in the list.
func (f *fixture) open(name string) {
	f.t.Helper()
	f.sees(name)
	f.click(name)
}

// choice is what the drop-down named name shows.
func (f *fixture) choice(name string) string {
	f.t.Helper()
	return f.control(ui.RolePopUpButton, name).Value
}

// value is what the field named name holds.
func (f *fixture) value(name string) string {
	f.t.Helper()
	return f.control(ui.RoleTextField, name).Value
}

// check toggles the check box named name.
func (f *fixture) check(name string) {
	f.t.Helper()
	f.press(f.control(ui.RoleCheckBox, name))
}

func (f *fixture) checked(name string) bool {
	f.t.Helper()
	return f.control(ui.RoleCheckBox, name).Checked
}

func (f *fixture) disabled(name string) bool {
	f.t.Helper()
	f.tt.Frame()
	a, ok := f.tt.Access(name)
	if !ok {
		f.t.Fatalf("no element %q in %q", name, f.tt.Texts())
	}
	return a.Disabled
}

func (f *fixture) wantOpened() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.opened)
}

// addIMAP makes an account directly, for a test about something else.
func (f *fixture) addIMAP(email string) *api.Account {
	f.t.Helper()
	a, err := f.c.Account().Create(f.t.Context(), &api.AccountCreateParams{Kind: api.AccountKindIMAP, Email: email, DisplayName: "", Auth: api.AuthKindPassword,
		IMAP: &api.ServerConfig{Host: "imap.x.test", Port: 993, TLS: api.TLSModeTLS, Username: email},
		SMTP: &api.ServerConfig{Host: "smtp.x.test", Port: 465, TLS: api.TLSModeTLS, Username: email}})
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}
