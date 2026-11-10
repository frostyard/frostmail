package live_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// fixture is a maild with one account and n messages in its INBOX, and a
// client whose events reach a fan-out.
type fixture struct {
	srv    *rpctest.Server
	c      *api.Client
	events *live.Events
	acct   int64
	inbox  int64
	base   time.Time
}

func newFixture(t *testing.T, n int) *fixture {
	t.Helper()
	srv := rpctest.Start(t)
	ctx := t.Context()
	f := &fixture{srv: srv, base: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)}
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "me@a.test",
			Auth: api.AuthKindPassword, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"},
			SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"}})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		f.acct, f.inbox = a.ID, mbs[0].ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.add(t, 1, n)
	f.c = srv.Dial(t)
	if _, err := f.c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	f.events = live.NewEvents()
	go f.events.Run(f.c)
	return f
}

// add stores messages with UIDs from first to last, each a minute newer.
func (f *fixture) add(t *testing.T, first, last int) {
	t.Helper()
	ctx := t.Context()
	var hs []store.MessageHeader
	for uid := first; uid <= last; uid++ {
		d := f.base.Add(time.Duration(uid) * time.Minute)
		hs = append(hs, store.MessageHeader{UID: uint32(uid), Subject: fmt.Sprintf("Message %d", uid),
			MessageID: fmt.Sprintf("%d@x", uid), From: store.Address{Addr: "ann@x.test"}, Date: d, InternalDate: d})
	}
	if err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		ids, err := tx.InsertHeaders(ctx, f.acct, f.inbox, hs)
		if err != nil || len(ids) == 0 {
			return err
		}
		return tx.Emit(ctx, api.MessageChanged{AccountID: f.acct, IDs: ids}) // as sync does
	}); err != nil {
		t.Fatal(err)
	}
}

// changes counts notify calls and lets a test wait for a condition.
type changes struct {
	mu sync.Mutex
	n  int
	ch chan struct{}
}

func newChanges() *changes { return &changes{ch: make(chan struct{}, 1)} }

func (c *changes) notify() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
	select {
	case c.ch <- struct{}{}:
	default:
	}
}

func (c *changes) until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for !ok() {
		select {
		case <-c.ch:
		case <-time.After(50 * time.Millisecond):
		case <-timeout:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func TestViewPagesAndFollowsDeltas(t *testing.T) {
	f := newFixture(t, 250)
	ch := newChanges()
	v := live.OpenView(f.c, f.events, api.ViewQuery{MailboxID: &f.inbox}, ch.notify)
	defer v.Close()
	ch.until(t, "the view to open", v.Ready)
	if v.Count() != 250 {
		t.Fatalf("count = %d", v.Count())
	}
	v.Ensure(0, 30)
	ch.until(t, "page 0", func() bool { _, ok := v.Row(99); return ok })
	if _, ok := v.Row(100); ok {
		t.Error("row 100 loaded with only rows 0 to 30 in view")
	}
	newest, _ := v.Row(0)
	if newest.Subject != "Message 250" {
		t.Errorf("row 0 = %q, want the newest", newest.Subject)
	}
	v.Ensure(200, 250)
	ch.until(t, "page 2", func() bool { _, ok := v.Row(249); return ok })

	// New mail goes on top: every row moves down by 2.
	f.add(t, 251, 252)
	ch.until(t, "the delta", func() bool { return v.Count() == 252 })
	v.Ensure(0, 30)
	ch.until(t, "the new rows", func() bool { r, ok := v.Row(0); return ok && r.Subject == "Message 252" })
	if r, ok := v.Row(2); !ok || r.Subject != "Message 250" {
		t.Errorf("row 2 after 2 inserts = %q, %v", r.Subject, ok)
	}
	if i := v.IndexOf(newest.ID); i != 2 {
		t.Errorf("IndexOf(Message 250) = %d, want 2", i)
	}

	// A flag change from the server refreshes the row in place.
	ctx := t.Context()
	if err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		ids, err := tx.UpdateFlags(ctx, f.inbox, []store.FlagUpdate{{UID: 250, Flags: store.Flags{Flagged: true}}})
		if err != nil {
			return err
		}
		return tx.Emit(ctx, api.MessageChanged{AccountID: f.acct, IDs: ids})
	}); err != nil {
		t.Fatal(err)
	}
	ch.until(t, "the refreshed row", func() bool { r, _ := v.Row(2); return r.Flags.Flagged })
}

func TestMailStoreLoadsAndFollows(t *testing.T) {
	f := newFixture(t, 1)
	ch := newChanges()
	m := live.NewMail(ch.notify)
	if err := m.Connect(t.Context(), f.c, f.events); err != nil {
		t.Fatal(err)
	}
	st := m.State()
	if !st.Loaded || len(st.Accounts) != 1 || len(st.Mailboxes) != 1 {
		t.Fatalf("state = %+v", st)
	}
	ch.until(t, "settings", func() bool { return m.State().Settings != nil })
	if _, err := f.c.Vip().Add(t.Context(), &api.VipAddParams{Addresses: []string{"ann@x.test"}}); err != nil {
		t.Fatal(err)
	}
	ch.until(t, "the VIP", func() bool { return len(m.State().VIPs) == 1 })
	f.events.Deliver(api.SyncProgress{Status: api.SyncStatus{AccountID: f.acct, Phase: api.SyncPhaseOffline}})
	if s := m.State().Sync[f.acct]; s.Phase != api.SyncPhaseOffline {
		t.Errorf("sync after sync.progress = %+v", s)
	}
	m.Disconnect()
	f.events.Deliver(api.SyncProgress{Status: api.SyncStatus{AccountID: f.acct, Phase: api.SyncPhaseIdle}})
	if s := m.State().Sync[f.acct]; s.Phase != api.SyncPhaseOffline {
		t.Error("a disconnected store still follows events")
	}
}

func TestActivityHandsOnEachEntryOnce(t *testing.T) {
	f := newFixture(t, 0)
	var mu sync.Mutex
	var got []api.ActivityEntry
	ch := newChanges()
	a := live.NewActivity(func(e api.ActivityEntry) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
		ch.notify()
	})
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(got) }
	f.srv.Activity.Record(activity.Pushed(f.acct, 1))
	f.srv.Activity.Record(activity.Pushed(f.acct, 2))
	if err := a.Connect(t.Context(), f.c, f.events); err != nil {
		t.Fatal(err)
	}
	if count() != 2 {
		t.Fatalf("after connect: %d entries", count())
	}
	f.srv.Activity.Record(activity.Sent(f.acct, time.Second))
	ch.until(t, "the live entry", func() bool { return count() == 3 })

	// A reconnect to the same maild hands on nothing again.
	c2 := f.srv.Dial(t)
	ev2 := live.NewEvents()
	go ev2.Run(c2)
	if err := a.Connect(context.Background(), c2, ev2); err != nil {
		t.Fatal(err)
	}
	f.srv.Activity.Record(activity.Pushed(f.acct, 3))
	ch.until(t, "the entry after the reconnect", func() bool { return count() == 4 })
	mu.Lock()
	defer mu.Unlock()
	for i := 1; i < len(got); i++ {
		if got[i].Seq <= got[i-1].Seq {
			t.Errorf("entry %d seq %d after %d", i, got[i].Seq, got[i-1].Seq)
		}
	}
}
