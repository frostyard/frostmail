package mainwin

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// committingDelete models the transaction that can finish after the RPC
// connection ends. The store and subsequent reads are real; only the RPC
// handler's commit boundary is held by the test.
type committingDelete struct {
	api.MessageService
	db                   *store.DB
	account, from, trash int64
	entered, release     chan struct{}
	finished             chan error
	deletes, reads       atomic.Int32
	denyReads            atomic.Bool
}

func (d *committingDelete) Delete(ctx context.Context, p *api.MessageDeleteParams) error {
	if d.deletes.Add(1) != 1 {
		return api.Conflict("unexpected second delete")
	}
	// Explicitly keep this transaction alive across the closed connection,
	// exercising the conservative case the UI must handle.
	ctx = context.WithoutCancel(ctx)
	err := d.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.MoveMembership(ctx, p.IDs[0], d.from, d.trash); err != nil {
			return err
		}
		if err := tx.Emit(ctx, api.MessageChanged{AccountID: d.account, IDs: p.IDs}); err != nil {
			return err
		}
		close(d.entered)
		<-d.release
		return nil
	})
	d.finished <- err
	return err
}

func (d *committingDelete) Summaries(ctx context.Context, p *api.MessageSummariesParams) ([]api.MessageSummary, error) {
	if d.denyReads.Load() {
		return nil, api.Unavailable("read back held off for the view event")
	}
	got, err := d.MessageService.Summaries(ctx, p)
	d.reads.Add(1)
	return got, err
}

func TestDeleteCommitAcrossReconnect(t *testing.T) {
	for _, byEvent := range []bool{false, true} {
		name := "moved read"
		if byEvent {
			name = "view drop"
		}
		t.Run(name, func(t *testing.T) {
			d := &committingDelete{entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan error, 1)}
			srv := rpctest.StartWith(t, rpctest.Options{Services: func(s *api.Services) {
				d.MessageService, s.Message = s.Message, d
			}})
			d.db = srv.DB
			id := seedDeleteCommit(t, d)
			c := srv.Dial(t)
			loop := frostuitest.NewLoop()
			w := New(Options{Target: conn.Target{Socket: srv.Socket}, Post: loop.Post})
			w.q.Source, w.pane, w.readBackEvery = mail.Source{Kind: mail.Mailbox, ID: d.from}, paneList, time.Millisecond
			w.takeClient(c, live.NewEvents())
			t.Cleanup(w.closeView)
			poll(t, "the original view", w.view.Ready)
			w.view.Ensure(0, 1)
			poll(t, "the original row", func() bool { return w.view.IndexOf(id) >= 0 })
			w.chosen.Add(id)
			w.deleteKey()
			select {
			case <-d.entered:
			case err := <-d.finished:
				t.Fatalf("delete ended before the commit gate: %v", err)
			case <-t.Context().Done():
				t.Fatal(t.Context().Err())
			}
			var once sync.Once
			release := func() { once.Do(func() { close(d.release) }) }
			t.Cleanup(release)
			held := w.deleting[id]
			_ = c.Close()
			loop.RunUntil(t, func() bool { return held.unread })

			next := srv.Dial(t)
			events := holdEvents(next)
			if _, err := next.Events().Subscribe(t.Context(), &api.EventsSubscribeParams{}); err != nil {
				t.Fatal(err)
			}
			w.takeClient(next, events.Events)
			poll(t, "the reconnected view", w.view.Ready)
			w.view.Ensure(0, 1)
			poll(t, "the unchanged row", func() bool { return w.view.IndexOf(id) >= 0 })
			// Three reads include Undo's initial read and two on the new
			// connection: the first read-back's callback must have run.
			loop.RunUntil(t, func() bool { return w.deleting[id] != held || d.reads.Load() >= 3 })
			w.repairSelection()
			if w.deleting[id] != held || !held.unread || held.done {
				t.Fatalf("unchanged read released the hold before commit: %+v", w.deleting[id])
			}
			w.deleteKey()
			if w.deleting[id] != held || d.deletes.Load() != 1 {
				t.Fatal("a second Delete was sent before the first committed")
			}

			d.denyReads.Store(byEvent)
			release()
			if err := <-d.finished; err != nil {
				t.Fatal(err)
			}
			if !byEvent {
				loop.RunUntil(t, func() bool { return held.done })
				w.repairSelection()
				w.deleteKey()
				if w.deleting[id] != held {
					t.Fatal("the moved read released a row the list still shows unchanged")
				}
			}
			poll(t, "the view delta dropping the row", func() bool {
				events.deliver()
				return w.view.Count() == 0 && w.view.Settled()
			})
			w.repairSelection()
			if w.deleting[id] != nil || w.chosen.Has(id) {
				t.Fatal("a dropped row remains held or selected")
			}
			if d.deletes.Load() != 1 {
				t.Fatalf("sent %d deletes, want one", d.deletes.Load())
			}
		})
	}
}

func seedDeleteCommit(t *testing.T, d *committingDelete) int64 {
	t.Helper()
	ctx := t.Context()
	var id int64
	if err := d.db.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "me@x.test", Auth: api.AuthKindPassword,
			IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"},
			SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"}})
		if err != nil {
			return err
		}
		d.account = a.ID
		boxes, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{
			{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}, {Path: "Trash", Role: api.MailboxRoleTrash, Selectable: true},
		})
		if err != nil {
			return err
		}
		for _, box := range boxes {
			if box.Role == api.MailboxRoleInbox {
				d.from = box.ID
			} else {
				d.trash = box.ID
			}
		}
		ids, err := tx.InsertHeaders(ctx, a.ID, d.from, []store.MessageHeader{{UID: 1, Subject: "Keep in Trash", MessageID: "keep@x.test", Date: time.Now(), InternalDate: time.Now()}})
		if err == nil {
			id = ids[0]
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}
