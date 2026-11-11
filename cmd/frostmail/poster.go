package main

import (
	"sync"

	"github.com/egoist/mygo"
)

// poster runs functions on the UI thread through the window, keeping those
// posted before the window exists.
type poster struct {
	mu      sync.Mutex
	win     *mygo.Window
	pending []func()
}

func (p *poster) post(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.win == nil {
		p.pending = append(p.pending, fn)
		return
	}
	p.win.Update(fn)
}

func (p *poster) attach(w *mygo.Window) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.win = w
	for _, fn := range p.pending {
		w.Update(fn)
	}
	p.pending = nil
}
