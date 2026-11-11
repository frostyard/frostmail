package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// draftKey names a draft: its maild, as conn.Target.Key names it, and
// its ID there. A draft's ID means something only to its maild, so the same
// ID on another maild is another draft.
type draftKey struct {
	target string
	draft  int64
}

// composeWindows keeps one window per draft. It belongs to the UI thread.
type composeWindows struct {
	windows map[draftKey]*mygo.Window
	main    *mainwin.Window
	// target is the one the app started on; the session's is the one now.
	target conn.Target
	paths  config.Paths
	// settings opens the settings window on a pane.
	settings func(pane string)
	all      *windowSet
	// leaving holds the drafts whose windows are hidden, saving; gone what
	// to run when a draft's window has closed, or stayed open as its save
	// failed.
	leaving map[draftKey]bool
	gone    map[draftKey][]func()
	// quitting says a quit is asking the windows to close. A window still
	// saving turns it down, which stops it there, before the windows after
	// it: quitWaits holds those drafts, and each asks again through quit
	// once it has closed.
	quitting  bool
	quitWaits map[draftKey]bool
	quit      func()
	// models are the open windows' drafts, saved at once when the OS
	// ends the session.
	models map[draftKey]*compose.Window
}

// prepare runs before the first window opens.
func (s *composeWindows) prepare() {
	if s.gone != nil {
		return
	}
	s.leaving, s.gone, s.quitWaits = map[draftKey]bool{}, map[draftKey][]func(){}, map[draftKey]bool{}
	s.models = map[draftKey]*compose.Window{}
	s.quit = mygo.App.Quit
	s.all.mu.Lock()
	s.all.closeCompose = s.closeAll
	s.all.mu.Unlock()
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if e.SessionEnding {
			s.saveAllNow(sessionEndBudget)
			return
		}
		s.quitting = true
		// After this quit has asked the windows, which it does in the
		// main thread's task that runs this.
		go mygo.RunOnMain(func() { s.quitting = false })
	})
}

// open shows a draft's window, raising it if it is open. c is the client
// the draft was made or found on: the session's now, else nothing opens, as
// the draft may be another maild's.
func (s *composeWindows) open(ctx context.Context, c *api.Client, draft int64, fresh bool) {
	s.prepare()
	if c != s.main.Client() {
		return // that connection has ended, and maybe its maild with it
	}
	key := draftKey{target: s.main.ConnectionState().Target.Key(), draft: draft}
	if win, ok := s.existing(key, func() { s.reopen(ctx, key) }); ok {
		if win != nil {
			win.Restore()
			win.Focus()
		}
		return
	}
	client := boundClient(func() conn.Target { return s.main.ConnectionState().Target }, s.main.Client)
	post := &poster{}
	var win *mygo.Window
	w := compose.New(compose.Options{Client: client, Draft: draft, Fresh: fresh, Post: post.post,
		Close: func() { win.Close() },
		Hide:  func() { win.Hide() },
		Show: func() {
			win.Show()
			win.Focus()
		},
		SetTitle:     func(title string) { win.SetTitle(title) },
		OpenFiles:    openFiles,
		FilesAsBytes: func() bool { return s.main.ConnectionState().Target.Remote() || os.Getenv("FLATPAK_ID") != "" },
		LoadPart:     s.loadPart,
		Sent:         s.main.Sent,
		EditSnippets: func() { s.settings(settings.PaneSnippets) },
		Kept:         func() { s.kept(key) },
	})
	win = mygo.NewWindow(mygo.WindowOptions{Title: "New Message", Width: 720, Height: 560, MinWidth: 480, MinHeight: 360,
		TitleBarStyle: mygo.TitleBarHidden, Content: ui.View(w.View)})
	post.attach(win)
	s.all.add(win)
	s.windows[key] = win
	s.models[key] = w
	win.OnClose(func(e *mygo.CloseEvent) {
		if !w.RequestClose() {
			s.refused(key)
			e.PreventDefault()
		}
	})
	win.OnClosed(func() { s.closed(key) })
	w.Start(ctx)
}

// existing finds the window of key's draft, kept or not, to raise; another
// maild's draft of the same ID has a window of its own. A window still
// saving is not raised, win is nil, and its draft opens again through
// reopen once it has closed.
func (s *composeWindows) existing(key draftKey, reopen func()) (win *mygo.Window, ok bool) {
	win = s.windows[key]
	if win == nil {
		return nil, false
	}
	if s.leaving[key] {
		s.gone[key] = append(s.gone[key], reopen)
		return nil, true
	}
	return win, true
}

// reopen opens a draft again after its window closed, while the session is
// on the draft's maild.
func (s *composeWindows) reopen(ctx context.Context, key draftKey) {
	if s.main.ConnectionState().Target.Key() != key.target {
		return
	}
	s.open(ctx, s.main.Client(), key.draft, false)
}

// refused records a draft's window turning down a close while it saves: a
// quit asking it then stops there, and asks again when it has closed.
func (s *composeWindows) refused(key draftKey) {
	s.leaving[key] = true
	if s.quitting {
		s.quitWaits[key] = true
	}
}

// boundClient is a window's client: the current one, across reconnects,
// while the target is the one it opened on. A draft's ID means something
// only to its maild, so past a change of target there is none.
func boundClient(target func() conn.Target, current func() *api.Client) func() *api.Client {
	openedOn := target()
	return func() *api.Client {
		if !target().Same(openedOn) {
			return nil
		}
		return current()
	}
}

// closed forgets a draft's window, once its save has ended. A quit that
// stopped at it goes on: the windows after it have not been asked.
func (s *composeWindows) closed(key draftKey) {
	hooks := s.gone[key]
	delete(s.windows, key)
	delete(s.models, key)
	delete(s.leaving, key)
	delete(s.gone, key)
	for _, fn := range hooks {
		fn()
	}
	if s.quitWaits[key] {
		delete(s.quitWaits, key)
		s.quit()
	}
}

// kept learns that a draft's window stayed open, as its save on closing
// failed: what waited for it to close goes on, and a quit that stopped at
// it ends there, for the window says why.
func (s *composeWindows) kept(key draftKey) {
	hooks := s.gone[key]
	delete(s.leaving, key)
	delete(s.gone, key)
	if s.quitWaits[key] {
		clear(s.quitWaits)
	}
	for _, fn := range hooks {
		fn()
	}
}

// closeAll closes the compose windows and returns when their saves have
// ended, for a change of maild: their drafts belong to the old one, so
// they save there, before the session moves, and before that maild stops
// (settings.Options.LeaveLocal). A window whose save failed stays open,
// saying so. It waits, so it runs on a worker.
func (s *composeWindows) closeAll() {
	var closing sync.WaitGroup
	mygo.RunOnMain(func() {
		for key, win := range s.windows {
			closing.Add(1)
			s.gone[key] = []func(){closing.Done} // and no reopening: its maild is going
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

// loadPart reads a mailpart: image for the editor, on a worker.
func (s *composeWindows) loadPart(c *api.Client, src string) ([]byte, error) {
	return loadPart(c, s.main.ConnectionState().Target, s.paths.CacheDir, src)
}

// loadPart reads a mailpart: image from maild's parts cache under cacheDir,
// or, behind lodge, where that cache is on lodge's host, from lodge's
// content route.
func loadPart(c *api.Client, target conn.Target, cacheDir, src string) ([]byte, error) {
	rel := strings.TrimPrefix(src, "mailpart://localhost/")
	if l := target.Lodge; l != nil {
		return fetchPart(c, *l, cacheDir, rel)
	}
	root, err := os.OpenRoot(filepath.Join(cacheDir, "parts"))
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
func fetchPart(c *api.Client, l conn.Lodge, cacheDir, rel string) ([]byte, error) {
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
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(cacheDir, "compose-part-*")
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

// sessionEndBudget bounds the saves when the OS ends the session: Windows
// gives an app a few seconds before it ends it.
const sessionEndBudget = 3 * time.Second

// saveAllNow writes every open draft's last edits before it returns, within
// budget in all: the OS ends the session, the quit cannot be refused, and
// the process may end once the quit's listeners return (MyGo v0.9.0's
// QuitEvent.SessionEnding, on Windows and macOS).
func (s *composeWindows) saveAllNow(budget time.Duration) {
	deadline := time.Now().Add(budget)
	for key, w := range s.models {
		left := time.Until(deadline)
		if left <= 0 {
			return
		}
		if err := w.SaveNow(left); err != nil {
			slog.Warn("saving a draft as the session ends", "draft", key.draft, "err", err)
		}
	}
}
