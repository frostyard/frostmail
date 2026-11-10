package main

import (
	"strings"
	"testing"
	"time"

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
