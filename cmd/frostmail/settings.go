package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/desktop/organize"
	"github.com/frostyard/frostmail/internal/desktop/settings"
)

// The bounds of a change of mode's two commands (mode.rs): mailctl's own
// limit is 30 s, and the launcher exits once maild answers.
const (
	stopBound  = 35 * time.Second
	startBound = 60 * time.Second
)

// settingsWindow is the one settings window, made when first asked for. It
// belongs to the UI thread.
type settingsWindow struct {
	w     *settings.Window
	rules organize.RuleSheet
	win   *mygo.Window
	main  *mainwin.Window
	all   *windowSet
}

// newSettings makes the settings model, which lives as long as the app:
// the main window's first run needs it with no window open.
func newSettings(post func(func()), target conn.Target, main func() *mainwin.Window) (*settingsWindow, error) {
	file, err := config.AppConfigFile(os.Getenv)
	if err != nil {
		return nil, err
	}
	s := &settingsWindow{}
	redraw := func(fn func()) {
		post(func() {
			fn()
			if s.win != nil {
				s.win.Invalidate()
			}
		})
	}
	s.rules.Deps = organize.Deps{Post: redraw, Client: func() *api.Client { return main().Client() },
		Context: func() organize.Context { return main().OrganizeContext() }}
	s.w = settings.New(settings.Options{Post: redraw, ConfigFile: file, Socket: target.Socket,
		LoadFacts:    func() settings.ServerFacts { return settings.FactsFrom(os.Getenv) },
		SetTarget:    func(t conn.Target) { main().SetTarget(t) },
		StopLocal:    func(ctx context.Context) error { return bounded(ctx, stopBound, "mailctl", "shutdown") },
		StartLocal:   func(ctx context.Context) error { return bounded(ctx, startBound, "frostmail", "--start-maild") },
		OpenURL:      signInOpener(nil).OpenURL,
		OpenSettings: s.open,
		Connection:   func() conn.State { return main().ConnectionState() },
		Toast:        func(title string, err error) { main().ShowProblem(title, err) },
		EditRule:     s.editRule,
		Announce:     newAnnouncer(post, main),
	})
	return s, nil
}

// signInOpener opens a sign-in page. The URL comes from maild, which behind
// lodge is another machine, and a sign-in is always https; launch is the
// system's handler when nil.
func signInOpener(launch func(string) error) frostui.Opener {
	return frostui.Opener{Schemes: []string{"https"}, Launch: launch}
}

// editRule opens the rule sheet over the settings window.
func (s *settingsWindow) editRule(rule *api.Rule) {
	if rule != nil {
		s.rules.Edit(*rule)
		return
	}
	s.rules.New(len(s.main.MailState().Rules) + 1)
}

// open shows the window on a pane, raising it if it is open.
func (s *settingsWindow) open(pane string) {
	if pane != "" {
		s.w.Show(pane)
	}
	if s.win != nil {
		s.win.Restore()
		s.win.Focus()
		return
	}
	s.win = mygo.NewWindow(mygo.WindowOptions{Title: "Settings", Width: settings.Width, Height: settings.Height,
		MinWidth: settings.MinWidth, MinHeight: settings.MinHeight, TitleBarStyle: mygo.TitleBarHidden,
		Content: ui.View(func(c *ui.Context) {
			s.w.View(c)
			s.rules.View(c)
		})})
	s.all.add(s.win)
	s.win.OnClosed(func() { s.win = nil })
}

// close closes the window with the main one.
func (s *settingsWindow) close() {
	if s.win != nil {
		s.win.Close()
	}
}

// bounded runs a command for at most limit with no input; its error is
// the last line it wrote, else how it ended.
func bounded(ctx context.Context, limit time.Duration, program string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...) //nolint:gosec // the app's own programs
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return errors.New(last)
	}
	return fmt.Errorf("running %s: %w", program, err)
}
