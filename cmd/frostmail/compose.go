package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/desktop/compose"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/desktop/settings"
)

// partTimeout bounds reading one image from lodge.
const partTimeout = 30 * time.Second

// composeWindows keeps one window per draft. It belongs to the UI thread.
type composeWindows struct {
	windows map[int64]*mygo.Window
	main    *mainwin.Window
	// target is the one the app started on; the session's is the one now.
	target conn.Target
	paths  config.Paths
	// settings opens the settings window on a pane.
	settings func(pane string)
	all      *windowSet
	// leaving holds the drafts whose windows are hidden, saving; gone what
	// to run when a draft's window has closed.
	leaving map[int64]bool
	gone    map[int64][]func()
	// quitAsked says the app was asked to quit, which a window still
	// saving turns down: the last to close asks again.
	quitAsked bool
}

// prepare runs before the first window opens.
func (s *composeWindows) prepare() {
	if s.gone != nil {
		return
	}
	s.leaving, s.gone = map[int64]bool{}, map[int64][]func(){}
	s.all.mu.Lock()
	s.all.closeCompose = s.closeAll
	s.all.mu.Unlock()
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { s.quitAsked = true })
}

// open shows a draft's window, raising it if it is open. c is the client
// the draft was made or found on.
func (s *composeWindows) open(ctx context.Context, c *api.Client, draft int64, fresh bool) {
	s.prepare()
	if win := s.windows[draft]; win != nil {
		if s.leaving[draft] {
			s.gone[draft] = append(s.gone[draft], func() { s.open(ctx, c, draft, false) })
			return
		}
		win.Restore()
		win.Focus()
		return
	}
	if c != s.main.Client() {
		return // that connection has ended, and maybe its maild with it
	}
	client := boundClient(func() conn.Target { return s.main.ConnectionState().Target }, s.main.Client)
	post := &poster{}
	var win *mygo.Window
	w := compose.New(compose.Options{Client: client, Draft: draft, Fresh: fresh, Post: post.post,
		Close:        func() { win.Close() },
		Hide:         func() { win.Hide() },
		SetTitle:     func(title string) { win.SetTitle(title) },
		OpenFiles:    openFiles,
		FilesAsBytes: func() bool { return s.main.ConnectionState().Target.Remote() || os.Getenv("FLATPAK_ID") != "" },
		LoadPart:     s.loadPart,
		Sent:         s.main.Sent,
		EditSnippets: func() { s.settings(settings.PaneSnippets) },
		SaveFailed:   func(err error) { s.main.ShowProblem("The draft was not saved", err) },
	})
	win = mygo.NewWindow(mygo.WindowOptions{Title: "New Message", Width: 720, Height: 560, MinWidth: 480, MinHeight: 360,
		TitleBarStyle: mygo.TitleBarHidden, Content: ui.View(w.View)})
	post.attach(win)
	s.all.add(win)
	s.windows[draft] = win
	win.OnClose(func(e *mygo.CloseEvent) {
		if !w.RequestClose() {
			s.leaving[draft] = true
			e.PreventDefault()
		}
	})
	win.OnClosed(func() { s.closed(draft) })
	w.Start(ctx)
}

// boundClient is a window's client: the current one, across reconnects,
// while the target is the one it opened on. A draft's ID means something
// only to its maild, so past a change of target there is none.
func boundClient(target func() conn.Target, current func() *api.Client) func() *api.Client {
	openedOn := target().String()
	return func() *api.Client {
		if target().String() != openedOn {
			return nil
		}
		return current()
	}
}

// closed forgets a draft's window, once its save has ended.
func (s *composeWindows) closed(draft int64) {
	hooks := s.gone[draft]
	delete(s.windows, draft)
	delete(s.leaving, draft)
	delete(s.gone, draft)
	for _, fn := range hooks {
		fn()
	}
	if s.quitAsked && len(s.windows) == 0 {
		mygo.App.Quit()
	}
}

// closeAll closes the compose windows and returns when their saves have
// ended, for a change of maild: their drafts belong to the old one, so
// they save there, before the session moves. It waits, so it runs on a
// worker.
func (s *composeWindows) closeAll() {
	var closing sync.WaitGroup
	mygo.RunOnMain(func() {
		for draft, win := range s.windows {
			closing.Add(1)
			s.gone[draft] = []func(){closing.Done} // and no reopening: its maild is going
			win.Close()
		}
	})
	closing.Wait()
}

// closeComposeWindows closes the compose windows, if any ever opened.
func (s *windowSet) closeComposeWindows() {
	s.mu.Lock()
	closeAll := s.closeCompose
	s.mu.Unlock()
	if closeAll != nil {
		closeAll()
	}
}

// loadPart reads a mailpart: image for the editor, on a worker: from
// maild's parts cache, or behind lodge, where that cache is on lodge's
// host, from lodge's content route.
func (s *composeWindows) loadPart(c *api.Client, src string) ([]byte, error) {
	rel := strings.TrimPrefix(src, "mailpart://localhost/")
	if l := s.main.ConnectionState().Target.Lodge; l != nil {
		return s.fetchPart(c, *l, rel)
	}
	root, err := os.OpenRoot(filepath.Join(s.paths.CacheDir, "parts"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(rel)
}

// partForm is a message part's path in the parts cache: m/<message>/ and
// the part's specifier, then its extension or its file name.
var partForm = regexp.MustCompile(`^m/([0-9]+)/([0-9]+(?:\.[0-9]+)*)(?:\.[A-Za-z0-9]+|/[^/]+)$`)

// partOf reads which message and part a path in the parts cache is.
func partOf(rel string) (message int64, part string, ok bool) {
	m := partForm.FindStringSubmatch(rel)
	if m == nil {
		return 0, "", false
	}
	message, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, "", false
	}
	return message, m[2], true
}

// fetchPart asks maild to name a part on its content route, and reads it
// from lodge. Only what partOf reads goes to maild: the path is the mail's.
func (s *composeWindows) fetchPart(c *api.Client, l conn.Lodge, rel string) ([]byte, error) {
	if c == nil {
		return nil, errors.New("not connected to maild")
	}
	message, part, ok := partOf(rel)
	if !ok {
		return nil, fmt.Errorf("not a message part: %s", rel)
	}
	ctx, cancel := context.WithTimeout(context.Background(), partTimeout)
	defer cancel()
	f, err := c.Message().Part(ctx, &api.MessagePartParams{ID: message, Path: part})
	if err != nil {
		return nil, err
	}
	if f.ContentPath == nil {
		return nil, errors.New("maild names no content path for the part")
	}
	if err := os.MkdirAll(s.paths.CacheDir, 0o700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(s.paths.CacheDir, "compose-part-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path, err := l.Download(ctx, dir, *f.ContentPath, f.Filename)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path) //nolint:gosec // the file just written under dir
}

func openFiles() ([]string, error) {
	return mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "Attach Files", Multiple: true})
}
