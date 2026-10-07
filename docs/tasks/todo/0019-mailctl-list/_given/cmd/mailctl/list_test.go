package main

// CONTRACT TEST for task card T-0019 (docs/tasks). Do not edit.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// listFixture stores one account with INBOX (three messages) and Archive.
func listFixture(t *testing.T) *rpctest.Server {
	t.Helper()
	time.Local = time.UTC // dates print in local time
	srv := rpctest.Start(t)
	ctx := t.Context()
	c := srv.Dial(t)
	server := api.ServerConfig{Host: "h", Port: 993, TLS: api.TLSModeTLS, Username: "u"}
	acct, err := c.Account().Create(ctx, &api.AccountCreateParams{Kind: api.AccountKindIMAP, Email: "a@mailtest.test", Auth: api.AuthKindPassword, IMAP: server, SMTP: server})
	if err != nil {
		t.Fatal(err)
	}
	msg := func(uid uint32, day int, subject, from string, flags store.Flags) store.MessageHeader {
		return store.MessageHeader{
			UID: uid, Subject: subject, From: store.Address{Name: from, Addr: strings.ToLower(from) + "@mailtest.test"},
			InternalDate: time.Date(2026, 10, day, 9, 30, 0, 0, time.UTC), Date: time.Date(2026, 10, day, 9, 30, 0, 0, time.UTC),
			Flags: flags, Preview: subject + " preview",
		}
	}
	err = srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, acct.ID, []store.ServerMailbox{
			{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true, Subscribed: true},
			{Path: "Archive", Role: api.MailboxRoleArchive, Selectable: true, Subscribed: true},
		})
		if err != nil {
			return err
		}
		ids, err := tx.InsertHeaders(ctx, acct.ID, mbs[0].ID, []store.MessageHeader{
			msg(1, 5, "Lunch on Thursday?", "Bob", store.Flags{Seen: true}),
			msg(2, 6, "Invoice for October", "Carol", store.Flags{Flagged: true, Color: 1}),
			msg(3, 7, "A rather long subject line that goes on and on past the column width", "Dan", store.Flags{}),
		})
		if err != nil {
			return err
		}
		for i, id := range ids {
			doc := store.SearchDoc{Subject: []string{"Lunch on Thursday?", "Invoice for October", "A rather long subject"}[i]}
			if err := tx.IndexMessage(ctx, id, doc); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func TestMailboxesCommand(t *testing.T) {
	srv := listFixture(t)
	out, err := runMailctl(t, "--socket", srv.Socket, "mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	want := "ID  ACCOUNT  ROLE     TOTAL  UNREAD  PATH\n" +
		"1   1        inbox    3      2       INBOX\n" +
		"2   1        archive  0      0       Archive\n"
	if out != want {
		t.Fatalf("mailboxes =\n%s\nwant\n%s", out, want)
	}
	out, err = runMailctl(t, "--socket", srv.Socket, "--json", "mailboxes", "--account", "1")
	if err != nil {
		t.Fatal(err)
	}
	var mbs []api.Mailbox
	if err := json.Unmarshal([]byte(out), &mbs); err != nil || len(mbs) != 2 || mbs[0].Unread != 2 {
		t.Fatalf("JSON mailboxes = %q (%v)", out, err)
	}
}

func TestLsCommand(t *testing.T) {
	srv := listFixture(t)
	out, err := runMailctl(t, "--socket", srv.Socket, "ls", "1")
	if err != nil {
		t.Fatal(err)
	}
	want := "ID  FLAGS  DATE              FROM   SUBJECT\n" +
		"3   N      2026-10-07 09:30  Dan    A rather long subject line that goes on and o…\n" +
		"2   N!     2026-10-06 09:30  Carol  Invoice for October\n" +
		"1          2026-10-05 09:30  Bob    Lunch on Thursday?\n"
	if out != want {
		t.Fatalf("ls =\n%q\nwant\n%q", out, want)
	}
	out, err = runMailctl(t, "--socket", srv.Socket, "ls", "1", "--limit", "1", "--unread")
	if err != nil || !strings.Contains(out, "\n3   N ") || strings.Count(out, "\n") != 2 {
		t.Fatalf("ls --limit 1 --unread = %q, %v", out, err)
	}
	out, err = runMailctl(t, "--socket", srv.Socket, "ls", "1", "--flagged")
	if err != nil || strings.Count(out, "\n") != 2 || !strings.Contains(out, "Invoice for October") {
		t.Fatalf("ls --flagged = %q, %v", out, err)
	}
	out, err = runMailctl(t, "--socket", srv.Socket, "--json", "ls", "1")
	if err != nil {
		t.Fatal(err)
	}
	var rows []api.MessageSummary
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 3 || rows[0].ID != 3 {
		t.Fatalf("JSON ls = %q (%v)", out, err)
	}
	if _, err := runMailctl(t, "--socket", srv.Socket, "ls", "inbox"); err == nil {
		t.Fatal("ls with a non-numeric mailbox succeeded")
	}
}

func TestSearchCommand(t *testing.T) {
	srv := listFixture(t)
	out, err := runMailctl(t, "--socket", srv.Socket, "search", "inv", "oct")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "\n") != 2 || !strings.Contains(out, "Invoice for October") {
		t.Fatalf("search = %q", out)
	}
	out, err = runMailctl(t, "--socket", srv.Socket, "search", "zebra")
	if err != nil || out != "no messages\n" {
		t.Fatalf("empty search = %q, %v", out, err)
	}
	if _, err := runMailctl(t, "--socket", srv.Socket, "search"); err == nil {
		t.Fatal("search without terms succeeded")
	}
}
