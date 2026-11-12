package main

import (
	"context"
	"errors"
	"sync"
)

// storeGate keeps work that outlives its caller from reading the store after
// maild closes it: lodge's unread badge runs in a goroutine of its own, which
// the lodge socket's own end does not wait for. close refuses later calls and
// waits for the calls under way.
type storeGate struct {
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// newStoreGate returns an open gate.
func newStoreGate() *storeGate { return &storeGate{} }

// enter reports whether a call may read the store; a true must be followed by
// leave.
func (g *storeGate) enter() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	g.wg.Add(1)
	return true
}

// leave ends a call that enter let in.
func (g *storeGate) leave() { g.wg.Done() }

// close refuses later calls and waits for the calls under way.
func (g *storeGate) close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
	g.wg.Wait()
}

// unreadGated is a store read behind g: once g is closed the call is refused
// and read does not run.
func unreadGated(g *storeGate, read func(context.Context) (int, error)) func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		if !g.enter() {
			return 0, errors.New("maild is stopping")
		}
		defer g.leave()
		return read(ctx)
	}
}
