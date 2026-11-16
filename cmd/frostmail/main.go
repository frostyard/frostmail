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
	"github.com/frostyard/frostmail/internal/desktop/launcher"
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

// appName is the name MyGo knows the app by, which names its single
// instance. FROSTMAIL_INSTANCE, which the development run sets
// (scripts/dev-app.sh), gives a run an instance of its own: without it, the
// development run would hand its arguments to the user's app, or take the
// user's, as the one instance of the app.
func appName(getenv func(string) string) string {
	if instance := getenv("FROSTMAIL_INSTANCE"); instance != "" {
		return config.AppID + "." + instance
	}
	return config.AppID
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
	mygo.App.SetName(appName(os.Getenv))
	if !lockSingleInstance() {
		return nil // the running app takes over
	}
	// A second instance never touches the badge of the first, so the entry
	// opens after the lock.
	var main *mainwin.Window
	badge := launcher.Open(slog.Default(), func() int64 { return main.InboxesUnread() })
	mygo.App.OnQuit(badge.Close)
	defer badge.Close()
	frostui.SetIcons(frostui.NewIcons(icons.Embedded()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var win *mygo.Window
	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return err
	}
	post := &poster{}
	// The tray reads the count after main exists, as the posted functions
	// run only once the window is ready.
	tc := &trayCount{post: post.post, count: func() int64 { return main.InboxesUnread() }}
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if win == nil {
			return
		}
		raise(win)
		if id, ok := openMessageArg(args); ok {
			post.post(func() { main.OpenMessage(id) })
		}
		if id, ok := replyMessageArg(args); ok {
			post.post(func() { main.ReplyTo(id) })
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
			Raise:        func() { raise(win) },
			OpenCompose: func(c *api.Client, draft int64, fresh bool) {
				composer.open(ctx, c, draft, fresh)
			},
			UnreadChanged: func() { badge.Changed(); tc.Changed() },
		})
		composer = &composeWindows{windows: map[draftKey]*mygo.Window{}, main: main, target: target, paths: paths, settings: prefs.open, all: windows}
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
		tray, err := frostui.NewTray(frostui.TrayOptions{ID: appName(os.Getenv), ToolTip: "Frostmail", OpenLabel: "Open Frostmail", Icon: icons.Tray(), Window: win, Items: trayItems(main.NewMessage)})
		if err != nil {
			slog.Info("no tray icon", "err", err)
		} else {
			wireTray(win, tray, tc, prefs.w)
			mygo.App.OnQuit(tray.Destroy)
		}
		frames = uikit.NewSafeHTMLView(win, "frostmail-reader", main.OpenLink, func() { post.post(func() {}) })
		main.Start(ctx)
		if id, ok := openMessageArg(os.Args[1:]); ok {
			post.post(func() { main.OpenMessage(id) })
		}
		if id, ok := replyMessageArg(os.Args[1:]); ok {
			post.post(func() { main.ReplyTo(id) })
		}
	})
	return mygo.App.Run()
}

// openMessageArg finds --open-message <id> in a launch's arguments.
func openMessageArg(args []string) (int64, bool) { return messageArg(args, "--open-message") }

// replyMessageArg finds --reply-message <id>, a notification's Reply.
func replyMessageArg(args []string) (int64, bool) { return messageArg(args, "--reply-message") }

func messageArg(args []string, flag string) (int64, bool) {
	i := slices.Index(args, flag)
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
