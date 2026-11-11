package rpcserver

import (
	"testing"

	"github.com/frostyard/frostmail/internal/activity"
)

// closeHooks stands for a connection: it keeps what OnClose was given.
type closeHooks struct{ fns []func() }

func (h *closeHooks) OnClose(fn func()) { h.fns = append(h.fns, fn) }

// However often a connection subscribes, one hook serves it, so the old
// subscriptions are not kept until the disconnect; that hook closes the
// subscription the connection has at its end.
func TestOneCloseHookServesEverySubscription(t *testing.T) {
	hub := activity.New(nil)
	st := &state{}
	hooks := &closeHooks{}
	var last *activity.Sub
	for i := range 3 {
		if i > 0 {
			st.closeActivity()
		}
		last, _ = hub.Subscribe()
		st.mu.Lock()
		st.setActivity(hooks, last)
		st.mu.Unlock()
	}
	if len(hooks.fns) != 1 {
		t.Fatalf("%d close hooks for 3 subscriptions, want 1", len(hooks.fns))
	}
	hooks.fns[0]()
	if _, open := <-last.C(); open {
		t.Error("the connection's end left its subscription open")
	}
	if st.activity != nil {
		t.Error("the connection still holds its subscription")
	}
}
