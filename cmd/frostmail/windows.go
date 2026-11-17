package main

import (
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// repaintDelays are when the open windows repaint after one opens.
var repaintDelays = []time.Duration{60 * time.Millisecond, 300 * time.Millisecond}

// windowSet repaints the app's windows when another changes what they
// share. MyGo's windows share one glyph atlas. With v0.8.3 a window that
// did not rebuild after another had added glyphs drew stale positions,
// with text and icons scrambled until its next rebuild (seen in real
// windows, 2026-11-10; plan 0015). v0.9.0 records which atlas a scene was
// painted with, which may make this unneeded; until a real window shows
// that, every window repaints shortly after one opens, and a window
// repaints as it takes the focus.
type windowSet struct {
	mu  sync.Mutex
	all map[*mygo.Window]bool
	// closeCompose closes the compose windows; compose.go sets it.
	closeCompose func()
	// textScale is the user's text size as a scale (mail.TextScale of the
	// main window's settings); nil, or 1, is the usual size. It runs on the
	// UI thread.
	textScale func() float32
}

// sized wraps the content view of a window, which every window of the app
// has, so that it draws at the user's text size: the theme's font size
// scales, and with it every frostui text size (uikit.Adapt, docs/specs/ui.md,
// Tokens). The view's own frostui.Adapt then keeps that theme. A test
// holds each mygo.NewWindow of this package to it.
func (s *windowSet) sized(view func(c *ui.Context)) func(c *ui.Context) {
	return func(c *ui.Context) {
		scale := float32(1)
		if s.textScale != nil {
			scale = s.textScale()
		}
		uikit.Adapt(c, scale)
		view(c)
	}
}

// add tracks a window from when it opens to when it closes.
func (s *windowSet) add(win *mygo.Window) {
	s.mu.Lock()
	if s.all == nil {
		s.all = map[*mygo.Window]bool{}
	}
	s.all[win] = true
	s.mu.Unlock()
	win.OnFocus(win.Invalidate)
	win.OnClosed(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.all, win)
	})
	for _, d := range repaintDelays {
		time.AfterFunc(d, s.repaint)
	}
}

func (s *windowSet) repaint() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for win := range s.all {
		win.Invalidate()
	}
}
