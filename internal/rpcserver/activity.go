package rpcserver

import (
	"context"
	"sync"

	"github.com/frostyard/lodge/kit/jsonrpc"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
)

// Activity is the activity domain (ADR-0055) over Options.Activity. Its
// methods admit the IDL's callers (local and session); agents get
// methodNotFound from the router.
func (s *Server) Activity() api.ActivityService { return activityService{s} }

type activityService struct{ s *Server }

// Recent implements activity.recent.
func (a activityService) Recent(_ context.Context, p *api.ActivityRecentParams) ([]api.ActivityEntry, error) {
	limit := 0
	if p.Limit != nil {
		if *p.Limit < 1 || *p.Limit > activity.MaxRecent {
			return nil, api.InvalidParams("limit must be 1 to %d", activity.MaxRecent)
		}
		limit = int(*p.Limit)
	}
	return a.s.opts.Activity.Recent(limit), nil
}

// Subscribe implements activity.subscribe; delivery starts after the reply
// and lasts until activity.unsubscribe or the connection's end.
func (a activityService) Subscribe(ctx context.Context, _ *api.ActivitySubscribeParams) (*api.ActivitySubscription, error) {
	c, st := jsonrpc.ConnFrom(ctx), stateFrom(ctx)
	if c == nil || st == nil || a.s.opts.Activity == nil {
		return nil, api.Unavailable("activity is not available on this connection")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.activity != nil {
		return nil, api.Conflict("this connection is already subscribed to activity")
	}
	sub, seq := a.s.opts.Activity.Subscribe()
	f := &feed{sub: sub}
	st.setActivity(c, f)
	jsonrpc.AfterReply(ctx, func() {
		c.Go(func(ctx context.Context) {
			f.run(ctx, func(e api.ActivityEntry) error {
				env, err := api.NewEnvelope(0, api.ActivityAdded{Entry: e})
				if err != nil {
					a.s.opts.Logger.Warn("encode activity", "err", err)
					return nil
				}
				return c.Notify(api.EventMethod, env)
			})
		})
	})
	return &api.ActivitySubscription{Seq: seq}, nil
}

// Unsubscribe implements activity.unsubscribe. It returns once no entry
// can follow, so nothing comes after its reply.
func (a activityService) Unsubscribe(ctx context.Context, _ *api.ActivityUnsubscribeParams) error {
	st := stateFrom(ctx)
	if st == nil {
		return nil
	}
	st.closeActivity()
	return nil
}

// feed is a connection's activity subscription and the gate its sender
// passes with each entry. A subscription's channel still holds the entries
// it took before Close, so closing it is not enough to stop the sender:
// stop closes the gate, after the entry on its way if there is one.
type feed struct {
	sub     *activity.Sub
	mu      sync.Mutex // held while an entry goes out
	stopped bool
}

// run sends the subscription's entries with send until the feed stops, the
// subscription ends, ctx ends or a send fails.
func (f *feed) run(ctx context.Context, send func(api.ActivityEntry) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-f.sub.C():
			if !ok || !f.pass(e, send) {
				return
			}
		}
	}
}

// pass sends e unless the feed has stopped; false ends the sender.
func (f *feed) pass(e api.ActivityEntry, send func(api.ActivityEntry) error) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.stopped && send(e) == nil
}

// stop ends the feed: once it returns, the sender sends nothing more.
func (f *feed) stop() {
	f.mu.Lock()
	f.stopped = true
	f.mu.Unlock()
	f.sub.Close()
}

// setActivity makes f the connection's feed; st.mu is held. One hook
// stops whichever feed the connection has when it ends: a hook for each
// would keep every old subscription, with its buffer, until the
// disconnect.
func (st *state) setActivity(c interface{ OnClose(func()) }, f *feed) {
	st.activity = f
	if !st.activityOn {
		st.activityOn = true
		c.OnClose(st.closeActivity)
	}
}

// closeActivity stops the connection's feed, if it has one. It waits for
// an entry on its way, without st.mu, which that send does not need.
func (st *state) closeActivity() {
	st.mu.Lock()
	f := st.activity
	st.activity = nil
	st.mu.Unlock()
	if f != nil {
		f.stop()
	}
}
