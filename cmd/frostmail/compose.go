package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/compose"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/desktop/settings"
)

// composeWindows keeps one window per draft. It belongs to the UI thread.
type composeWindows struct {
	windows map[int64]*mygo.Window
	main    *mainwin.Window
	target  conn.Target
	paths   config.Paths
	// settings opens the settings window on a pane.
	settings func(pane string)
	all      *windowSet
}

// open shows a draft's window, raising it if it is open.
func (s *composeWindows) open(ctx context.Context, c *api.Client, draft int64, fresh bool) {
	if win := s.windows[draft]; win != nil {
		win.Restore()
		win.Focus()
		return
	}
	post := &poster{}
	var win *mygo.Window
	w := compose.New(compose.Options{Client: c, Draft: draft, Fresh: fresh, Post: post.post,
		Close:        func() { win.Close() },
		SetTitle:     func(title string) { win.SetTitle(title) },
		OpenFiles:    openFiles,
		FilesAsBytes: s.target.Remote() || os.Getenv("FLATPAK_ID") != "",
		LoadPart:     s.loadPart,
		Sent:         s.main.Sent,
		EditSnippets: func() { s.settings(settings.PaneSnippets) },
	})
	win = mygo.NewWindow(mygo.WindowOptions{Title: "New Message", Width: 720, Height: 560, MinWidth: 480, MinHeight: 360,
		TitleBarStyle: mygo.TitleBarHidden, Content: ui.View(w.View)})
	post.attach(win)
	s.all.add(win)
	s.windows[draft] = win
	win.OnClosed(func() {
		w.Closing()
		delete(s.windows, draft)
	})
	w.Start(ctx)
}

// loadPart reads a mailpart: image from maild's parts cache, for the
// editor. Behind lodge the cache is on lodge's host, and the image shows
// as an empty box; it stays in the draft.
func (s *composeWindows) loadPart(src string) ([]byte, error) {
	rel := strings.TrimPrefix(src, "mailpart://localhost/")
	root, err := os.OpenRoot(filepath.Join(s.paths.CacheDir, "parts"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(rel)
}

func openFiles() ([]string, error) {
	return mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "Attach Files", Multiple: true})
}
