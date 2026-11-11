package rpcserver

import (
	"context"

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
	st.setActivity(c, sub)
	jsonrpc.AfterReply(ctx, func() {
		c.Go(func(ctx context.Context) {
			for {
				select {
				case <-ctx.Done():
					return
				case e, ok := <-sub.C():
					if !ok {
						return
					}
					env, err := api.NewEnvelope(0, api.ActivityAdded{Entry: e})
					if err != nil {
						a.s.opts.Logger.Warn("encode activity", "err", err)
						continue
					}
					if c.Notify(api.EventMethod, env) != nil {
						return
					}
				}
			}
		})
	})
	return &api.ActivitySubscription{Seq: seq}, nil
}

// Unsubscribe implements activity.unsubscribe.
func (a activityService) Unsubscribe(ctx context.Context, _ *api.ActivityUnsubscribeParams) error {
	st := stateFrom(ctx)
	if st == nil {
		return nil
	}
	st.closeActivity()
	return nil
}

// setActivity makes sub the connection's subscription; st.mu is held. One
// hook closes whichever subscription the connection has when it ends: a
// hook for each would keep every old subscription, with its buffer, until
// the disconnect.
func (st *state) setActivity(c interface{ OnClose(func()) }, sub *activity.Sub) {
	st.activity = sub
	if !st.activityOn {
		st.activityOn = true
		c.OnClose(st.closeActivity)
	}
}

// closeActivity ends the connection's subscription, if it has one.
func (st *state) closeActivity() {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.activity != nil {
		st.activity.Close()
		st.activity = nil
	}
}
