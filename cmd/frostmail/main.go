// Command frostmail is the desktop app: a native window on MyGo and
// frostui, and a client of maild (docs/plans/0015-mygo-desktop.md).
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// version is stamped at build time.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "frostmail:", err)
		os.Exit(1)
	}
}

func run() error {
	target, err := conn.TargetFrom(os.Getenv)
	if err != nil {
		return err
	}
	if err := frostui.SetAppID(config.AppID, "Frostmail"); err != nil {
		return err
	}
	mygo.App.SetName(config.AppID)
	if !mygo.App.RequestSingleInstanceLock() {
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
		main = mainwin.New(mainwin.Options{Target: target, Client: "frostmail " + version, Post: post.post,
			CacheDir:    paths.CacheDir,
			DownloadDir: filepath.Join(paths.CacheDir, "frostmail-app", "attachments"),
			NewFrame:    func() (uikit.Frame, error) { return frames.New() },
			SaveDialog:  saveDialog,
		})
		win = mygo.NewWindow(mygo.WindowOptions{
			Title: "Frostmail", Width: 1200, Height: 780, MinWidth: 800, MinHeight: 500,
			TitleBarStyle: mygo.TitleBarHidden,
			Content:       ui.View(main.View),
		})
		post.attach(win)
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
