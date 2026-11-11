package rpcserver_test

import (
	"errors"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// nextActivity waits for the next activity.added event on c.
func nextActivity(t *testing.T, c *api.Client) api.ActivityEntry {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case env, ok := <-c.Notifications():
			if !ok {
				t.Fatal("the connection closed")
			}
			ev, err := api.DecodeEvent(env.Event, env.Data)
			if err != nil {
				t.Fatal(err)
			}
			if a, ok := ev.(api.ActivityAdded); ok {
				return a.Entry
			}
		case <-timeout:
			t.Fatal("no activity.added event")
		}
	}
}

func TestActivityRecentAndLiveEntries(t *testing.T) {
	srv := rpctest.Start(t)
	ctx := t.Context()
	c := srv.Dial(t)
	srv.Activity.Record(activity.Pushed(1, 2))

	recent, err := c.Activity().Recent(ctx, &api.ActivityRecentParams{})
	if err != nil || len(recent) != 1 || recent[0].Kind != api.ActivityKindPushed || *recent[0].N != 2 {
		t.Fatalf("activity.recent = %+v, %v", recent, err)
	}
	limit := int64(activity.MaxRecent + 1)
	if _, err := c.Activity().Recent(ctx, &api.ActivityRecentParams{Limit: &limit}); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("a limit of %d = %v, want invalidParams", limit, err)
	}

	sub, err := c.Activity().Subscribe(ctx, &api.ActivitySubscribeParams{})
	if err != nil || sub.Seq != recent[0].Seq {
		t.Fatalf("activity.subscribe = %+v, %v; want seq %d", sub, err, recent[0].Seq)
	}
	if _, err := c.Activity().Subscribe(ctx, &api.ActivitySubscribeParams{}); !errors.Is(err, api.ErrConflict) {
		t.Errorf("a second activity.subscribe = %v, want conflict", err)
	}
	srv.Activity.Record(activity.Sent(1, time.Second))
	if e := nextActivity(t, c); e.Kind != api.ActivityKindSent || e.Seq != sub.Seq+1 {
		t.Errorf("live entry = %+v", e)
	}

	if err := c.Activity().Unsubscribe(ctx, &api.ActivityUnsubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Activity().Subscribe(ctx, &api.ActivitySubscribeParams{}); err != nil {
		t.Errorf("activity.subscribe after unsubscribe = %v", err)
	}
}

// Agents get no activity: the IDL leaves the domain to local and session
// callers (ADR-0055).
func TestAgentsGetNoActivity(t *testing.T) {
	srv := rpctest.Start(t)
	s, err := api.ParseScope("account=*;write;send")
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindAgent, Token: "claude", Scope: s})
	if _, err := agent.Activity().Recent(t.Context(), &api.ActivityRecentParams{}); !errors.Is(err, api.ErrMethodNotFound) {
		t.Errorf("activity.recent as an agent = %v, want methodNotFound", err)
	}
	if _, err := agent.Activity().Subscribe(t.Context(), &api.ActivitySubscribeParams{}); !errors.Is(err, api.ErrMethodNotFound) {
		t.Errorf("activity.subscribe as an agent = %v, want methodNotFound", err)
	}
	session, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindSession, Login: "ann@example.com"})
	if _, err := session.Activity().Recent(t.Context(), &api.ActivityRecentParams{}); err != nil {
		t.Errorf("activity.recent in a lodge session = %v", err)
	}
}
