package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

func TestActivityCommand(t *testing.T) {
	srv := rpctest.Start(t)
	ctx := t.Context()
	var acct, inbox int64
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "me@a.test",
			Auth: api.AuthKindPassword, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"},
			SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"}})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		acct, inbox = a.ID, mbs[0].ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.Activity.Record(activity.Connected(acct, 300*time.Millisecond))
	srv.Activity.Record(activity.MailboxSynced(acct, inbox, 3, 1, 0, 400*time.Millisecond))

	out := mustRun(t, "--socket", srv.Socket, "activity")
	if !strings.Contains(out, "  info   me@a.test  INBOX synced: 3 new, 1 changed (0.4 s)\n") || strings.Contains(out, "connected") {
		t.Errorf("activity (info and up) =\n%s", out)
	}
	if out := mustRun(t, "--socket", srv.Socket, "activity", "--level", "debug"); !strings.Contains(out, "me@a.test  connected to IMAP (0.3 s)") {
		t.Errorf("activity --level debug =\n%s", out)
	}
	if _, err := runMailctl(t, "--socket", srv.Socket, "activity", "--level", "loud"); err == nil {
		t.Error("activity --level loud: no error")
	}
}

// blockedWriter holds every write until open is closed.
type blockedWriter struct {
	open chan struct{}
	mu   sync.Mutex
	buf  strings.Builder
}

func (w *blockedWriter) Write(p []byte) (int, error) {
	<-w.open
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *blockedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// A slow stdout loses entries and says so; the stream stays open. Without
// the drain, the client's event buffer fills and it closes the connection.
func TestFollowOutlastsASlowOutput(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	c := api.NewClient(clientSide)
	defer c.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	hub := activity.New(nil)
	hub.Record(activity.SendFailed(1, errors.New("no"), true))
	entry := hub.Recent(1)[0]
	send := func(seq int64) error {
		e := entry
		e.Seq = seq
		env, err := api.NewEnvelope(0, api.ActivityAdded{Entry: e})
		if err != nil {
			return err
		}
		params, err := json.Marshal(env)
		if err != nil {
			return err
		}
		line, err := json.Marshal(api.Frame{JSONRPC: "2.0", Method: api.EventMethod, Params: params})
		if err != nil {
			return err
		}
		_, err = serverSide.Write(append(line, '\n'))
		return err
	}

	w := &blockedWriter{open: make(chan struct{})}
	names := &clientNames{c: c, accounts: map[int64]string{1: "me@a.test"}, mailboxes: map[int64]string{}}
	done := make(chan error, 1)
	go func() {
		done <- followActivity(ctx, c, drainActivity(c.Notifications(), api.ActivityLevelInfo), w, names, api.ActivityLevelInfo, 0)
	}()

	n := int64(2 * api.EventBuffer)
	for seq := int64(1); seq <= n; seq++ {
		if err := send(seq); err != nil {
			t.Fatalf("event %d: the connection closed behind a slow output: %v (%v)", seq, err, c.Err())
		}
	}
	close(w.open)

	// The next entry reaches the output, after the count of those lost.
	if err := send(n + 1); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !strings.Contains(w.String(), "not shown") {
		select {
		case <-tick.C:
			if err := send(n + 1); err != nil {
				t.Fatal(err)
			}
		case <-deadline:
			t.Fatalf("no dropped line in:\n%s", w.String())
		}
	}
	if err := c.Err(); err != nil {
		t.Errorf("the connection ended: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("follow = %v", err)
	}
}

func TestActivityJSONFiltersByLevel(t *testing.T) {
	srv := rpctest.Start(t)
	srv.Activity.Record(activity.Connected(1, time.Second))
	srv.Activity.Record(activity.Pushed(1, 2))
	out := mustRun(t, "--socket", srv.Socket, "--json", "activity", "--level", "info")
	var got []api.ActivityEntry
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != api.ActivityKindPushed {
		t.Fatalf("entries = %+v, want only the info entry", got)
	}
}

type activityWriter func([]byte) (int, error)

func (f activityWriter) Write(p []byte) (int, error) { return f(p) }

func TestActivityJSONFollowPrintsHistoryAndLiveObjects(t *testing.T) {
	srv := rpctest.Start(t)
	srv.Activity.Record(activity.Connected(1, time.Second))
	srv.Activity.Record(activity.Pushed(1, 2))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var got []api.ActivityKind
	w := activityWriter(func(p []byte) (int, error) {
		var e api.ActivityEntry
		if err := json.Unmarshal(p, &e); err != nil {
			return 0, err
		}
		if strings.Count(string(p), "\n") != 1 {
			t.Errorf("not one JSON object per line: %q", p)
		}
		got = append(got, e.Kind)
		if len(got) == 1 {
			srv.Activity.Record(activity.Connected(1, time.Second))
			srv.Activity.Record(activity.Sent(1, time.Second))
		} else {
			cancel()
		}
		return len(p), nil
	})
	clix.Stdout = w
	t.Cleanup(func() { clix.Stdout = nil })
	root := newRootCmd()
	root.SetContext(ctx)
	root.SetOut(w)
	root.SetArgs([]string{"--socket", srv.Socket, "--json", "activity", "--follow", "--level", "info"})
	if err := (&clix.App{Version: "test"}).RunContext(ctx, root); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != api.ActivityKindPushed || got[1] != api.ActivityKindSent {
		t.Fatalf("entries = %v, want pushed then sent", got)
	}
}
