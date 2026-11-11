package rpcserver

import (
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
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
		st.setActivity(hooks, &feed{sub: last})
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

// M15: a subscription's channel keeps the entries it took before Close, so
// a sender that only read it would send them after activity.unsubscribe
// had answered. Once the feed has stopped, none goes.
func TestNoEntryGoesOutAfterTheFeedStops(t *testing.T) {
	hub := activity.New(nil)
	sub, _ := hub.Subscribe()
	f := &feed{sub: sub}
	hub.Record(activity.Pushed(1, 1))
	hub.Record(activity.Sent(1, time.Second))
	if len(sub.C()) != 2 {
		t.Fatalf("%d entries waiting, want 2", len(sub.C()))
	}
	f.stop()
	var sent []api.ActivityEntry
	f.run(t.Context(), func(e api.ActivityEntry) error {
		sent = append(sent, e)
		return nil
	})
	if len(sent) != 0 {
		t.Errorf("sent %+v after the feed stopped", sent)
	}
}

// An entry on its way when the feed stops goes out before stop returns:
// then no entry follows the reply to activity.unsubscribe.
func TestStopWaitsForTheEntryOnItsWay(t *testing.T) {
	hub := activity.New(nil)
	sub, _ := hub.Subscribe()
	f := &feed{sub: sub}
	hub.Record(activity.Pushed(1, 1))
	hub.Record(activity.Sent(1, time.Second))
	held, release, stopped, ran := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	sends := 0
	go func() {
		defer close(ran)
		f.run(t.Context(), func(api.ActivityEntry) error {
			sends++
			if sends == 1 {
				close(held)
				<-release
			}
			select {
			case <-stopped:
				t.Error("an entry went out after the feed stopped")
			default:
			}
			return nil
		})
	}()
	<-held
	go func() {
		f.stop()
		close(stopped)
	}()
	close(release)
	<-stopped
	<-ran
}
