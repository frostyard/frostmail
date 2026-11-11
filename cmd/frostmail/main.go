// Command frostmail is the desktop app: a native window on MyGo and
// frostui, and a client of maild (docs/plans/0015-mygo-desktop.md).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
	"github.com/frostyard/frostmail/internal/problemrec"
)

// version is stamped at build time.
var version = "dev"

// testLaunch stands in for the system's handler of links in the automation
// build, when a check asks; nil opens them.
var testLaunch func(url string) error

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "frostmail:", err)
		os.Exit(1)
	}
}

// codeSocketLost is the problem record of maild's local socket ending
// under the app (ADR-0049).
const codeSocketLost = "bridge.socket_lost"

func run() error {
	rec := problemrec.New(version, problems.ComponentShell)
	defer rec.RecoverPanic()
	if rec.Err() == nil { // an unreleased build records nothing
		if err := rec.CrashOutput(); err != nil {
			slog.Warn("recording crashes", "err", err)
		}
	}
	target, err := conn.TargetFrom(os.Getenv)
	if err != nil {
		return err
	}
	if err := frostui.SetAppID(config.AppID, "Frostmail"); err != nil {
		return err
	}
	mygo.App.SetName(config.AppID)
	if !lockSingleInstance() {
		return nil // the running app takes over
	}
	frostui.SetIcons(frostui.NewIcons(icons.Embedded()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var win *mygo.Window
	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return err
	}
	var main *mainwin.Window
	post := &poster{}
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if win == nil {
			return
		}
		win.Restore()
		win.Focus()
		if id, ok := openMessageArg(args); ok {
			post.post(func() { main.OpenMessage(id) })
		}
	})
	mygo.App.WhenReady(func() {
		var frames *uikit.SafeHTMLView
		var composer *composeWindows
		windows := &windowSet{}
		stateFile := ""
		if file, err := config.AppConfigFile(os.Getenv); err == nil {
			stateFile = filepath.Join(filepath.Dir(file), "window.json")
		}
		prefs, err := newSettings(post.post, target, func() *mainwin.Window { return main })
		if err != nil {
			slog.Error("the settings window", "err", err)
			mygo.App.Quit()
			return
		}
		calendarWindows := &pimWindows{windows: map[string]*mygo.Window{}, all: windows}
		// Calendar's windows draw state the main window's posts change.
		postAll := func(fn func()) {
			post.post(func() {
				fn()
				calendarWindows.redraw()
			})
		}
		main = mainwin.New(mainwin.Options{Target: target, Client: "frostmail " + version, Post: postAll,
			OpenWindow: calendarWindows.open, CloseWindow: calendarWindows.close, SetWindowTitle: calendarWindows.setTitle,
			OpenImage: openImage, StateFile: stateFile,
			CacheDir:     paths.CacheDir,
			DownloadDir:  filepath.Join(paths.CacheDir, "frostmail-app", "attachments"),
			NewFrame:     func() (uikit.Frame, error) { return frames.New() },
			SaveDialog:   saveDialog,
			Launch:       testLaunch,
			SocketLost:   func(err error) { rec.Error(codeSocketLost, err) },
			Settings:     prefs.w,
			OpenSettings: prefs.open,
			OpenCompose: func(c *api.Client, draft int64, fresh bool) {
				composer.open(ctx, c, draft, fresh)
			},
		})
		composer = &composeWindows{windows: map[int64]*mygo.Window{}, main: main, target: target, paths: paths, settings: prefs.open, all: windows}
		prefs.main, prefs.all = main, windows
		win = mygo.NewWindow(mygo.WindowOptions{
			Title: "Frostmail", Width: 1200, Height: 780, MinWidth: 800, MinHeight: 500,
			TitleBarStyle: mygo.TitleBarHidden,
			Content:       ui.View(main.View),
		})
		post.attach(win)
		windows.add(win)
		win.OnClosed(prefs.close)
		win.OnClosed(main.SaveState)
		frames = uikit.NewSafeHTMLView(win, main.OpenLink, func() { post.post(func() {}) })
		main.Start(ctx)
		if id, ok := openMessageArg(os.Args[1:]); ok {
			post.post(func() { main.OpenMessage(id) })
		}
	})
	return mygo.App.Run()
}

// openMessageArg finds --open-message <id> in a launch's arguments.
func openMessageArg(args []string) (int64, bool) {
	i := slices.Index(args, "--open-message")
	if i < 0 || i+1 >= len(args) {
		return 0, false
	}
	id, err := strconv.ParseInt(args[i+1], 10, 64)
	return id, err == nil && id > 0
}

// saveDialog asks where to save a file; "" when cancelled.
func saveDialog(title, name, filter, extension string) (string, error) {
	opts := mygo.SaveDialogOptions{Title: title, DefaultPath: name}
	if filter != "" {
		opts.Filters = []mygo.FileFilter{{Name: filter, Extensions: []string{extension}}}
	}
	return mygo.Dialog.Save(opts)
}
