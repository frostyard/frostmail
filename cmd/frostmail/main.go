// Command frostmail is the desktop app: a native window on MyGo and
// frostui, and a client of maild (docs/plans/0015-mygo-desktop.md).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
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
	frostui.SetIcons(frostui.NewIcons(nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var win *mygo.Window
	mygo.App.OnSecondInstance(func([]string, string) {
		if win != nil {
			win.Restore()
			win.Focus()
		}
	})
	mygo.App.WhenReady(func() {
		post := &poster{}
		main := mainwin.New(mainwin.Options{Target: target, Client: "frostmail " + version, Post: post.post})
		win = mygo.NewWindow(mygo.WindowOptions{
			Title: "Frostmail", Width: 1200, Height: 780, MinWidth: 800, MinHeight: 500,
			TitleBarStyle: mygo.TitleBarHidden,
			Content:       ui.View(main.View),
		})
		post.attach(win)
		main.Start(ctx)
	})
	return mygo.App.Run()
}
