package main

import (
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// repaintDelays are when the open windows repaint after one opens.
var repaintDelays = []time.Duration{60 * time.Millisecond, 300 * time.Millisecond}

// windowSet repaints the app's windows when another changes what they
// share. MyGo v0.8.3's windows share one glyph atlas: a window that does
// not rebuild after another has added glyphs draws stale positions, with
// text and icons scrambled until its next rebuild (seen in real windows,
// 2026-11-10; plan 0015). So every window repaints shortly after one
// opens, and a window repaints as it takes the focus.
type windowSet struct {
	mu  sync.Mutex
	all map[*mygo.Window]bool
	// closeCompose closes the compose windows; compose.go sets it.
	closeCompose func()
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
