package main

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

func TestWaitingCommand(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	srv := rpctest.StartWith(t, rpctest.Options{Now: func() time.Time { return now }})
	ctx := t.Context()
	var ann, bob int64
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "me@a.test",
			Auth: api.AuthKindPassword, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"},
			SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"}})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, acct.ID, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true},
			{Path: "Sent", Role: api.MailboxRoleSent, Selectable: true}})
		if err != nil {
			return err
		}
		two, six := now.Add(-48*time.Hour), now.Add(-6*24*time.Hour)
		in, err := tx.InsertHeaders(ctx, acct.ID, mbs[0].ID, []store.MessageHeader{{UID: 1, Subject: "Saturday?", MessageID: "1@x",
			From: store.Address{Addr: "ann@x.test"}, To: []store.Address{{Addr: "me@a.test"}}, Date: two, InternalDate: two}})
		if err != nil {
			return err
		}
		out, err := tx.InsertHeaders(ctx, acct.ID, mbs[1].ID, []store.MessageHeader{{UID: 1, Subject: "Invoice", MessageID: "2@x",
			From: store.Address{Addr: "me@a.test"}, To: []store.Address{{Addr: "bob@x.test"}}, Date: six, InternalDate: six}})
		if err != nil {
			return err
		}
		ann, bob = in[0], out[0]
		_, err = tx.AssignThreads(ctx, acct.ID, []int64{ann, bob})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"ID", "DAYS", "VIP", "FROM", "SUBJECT"}, {itoa64(ann), "2", "-", "ann@x.test", "Saturday?"}}
	if got := cells(mustRun(t, "--socket", srv.Socket, "waiting")); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("waiting = %q, want %q", got, want)
	}
	want = [][]string{{"ID", "DAYS", "VIP", "TO", "SUBJECT"}, {itoa64(bob), "6", "-", "bob@x.test", "Invoice"}}
	if got := cells(mustRun(t, "--socket", srv.Socket, "waiting", "--others")); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("waiting --others = %q, want %q", got, want)
	}
	if out := mustRun(t, "--socket", srv.Socket, "waiting", "--waiting-days", "3"); out != "nothing waiting\n" {
		t.Errorf("waiting --waiting-days 3 = %q", out)
	}
	if _, err := runMailctl(t, "--socket", srv.Socket, "waiting", "--limit", "26"); err == nil {
		t.Error("waiting --limit 26 succeeded")
	}
}
