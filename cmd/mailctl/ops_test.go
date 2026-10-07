package main

// CONTRACT TEST for task card T-0023 (docs/tasks). Do not edit.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/rpctest"
)

type opsEnv struct {
	srv     *rpctest.Server
	c       *api.Client
	mem     *imapxtest.Mem
	inbox   int64
	archive int64
	trash   int64
	ids     []int64 // INBOX messages, newest first
}

// opsFixture syncs the five seed messages from an in-memory server with
// MOVE and UIDPLUS and returns the IDs maild gave them.
func opsFixture(t *testing.T) *opsEnv {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	files, _ := filepath.Glob("../../dev/incus/seed/*.eml")
	slices.Sort(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := mem.User.Append("INBOX", bytes.NewReader(raw), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := mailsync.Config{PollInterval: time.Hour, MinBackoff: 50 * time.Millisecond}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg})
	c := srv.Dial(t)
	ctx := t.Context()
	if _, err := c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	o := mem.DialOptions()
	server := api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: o.Username}
	if _, err := c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "test@mailtest.test", Auth: api.AuthKindPassword, IMAP: server, SMTP: server,
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: 1, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	for ev := range c.Notifications() {
		e, _ := api.DecodeEvent(ev.Event, ev.Data)
		if p, ok := e.(api.SyncProgress); ok && p.Status.Phase == api.SyncPhaseIdle {
			break
		}
	}
	env := &opsEnv{srv: srv, c: c, mem: mem}
	mbs, err := c.Mailbox().List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, mb := range mbs {
		switch mb.Role {
		case api.MailboxRoleInbox:
			env.inbox = mb.ID
		case api.MailboxRoleArchive:
			env.archive = mb.ID
		case api.MailboxRoleTrash:
			env.trash = mb.ID
		}
	}
	v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &env.inbox}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: v.Count})
	if err != nil || len(rows) != 5 {
		t.Fatalf("INBOX rows = %d, %v", len(rows), err)
	}
	for _, r := range rows {
		env.ids = append(env.ids, r.ID)
	}
	return env
}

func (e *opsEnv) get(t *testing.T, id int64) *api.Message {
	t.Helper()
	m, err := e.c.Message().Get(t.Context(), &api.MessageGetParams{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// serverCount polls a mailbox's message count on the IMAP server.
func (e *opsEnv) waitServerCount(t *testing.T, mailbox string, want uint32) {
	t.Helper()
	s, err := imapx.Open(context.Background(), e.mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(5 * time.Second)
	for {
		st, err := s.Status(context.Background(), mailbox)
		if err == nil && st.Messages == want {
			return
		}
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("%s on the server has %d messages, want %d", mailbox, st.Messages, want)
		}
	}
}

func idArg(i int64) string { return fmt.Sprint(i) }

func TestFlagCommand(t *testing.T) {
	e := opsFixture(t)
	out, err := runMailctl(t, "--socket", e.srv.Socket, "flag", idArg(e.ids[0]), idArg(e.ids[1]), "--seen", "--color", "3")
	if err != nil || out != "updated 2 messages\n" {
		t.Fatalf("flag = %q, %v", out, err)
	}
	for _, i := range e.ids[:2] {
		f := e.get(t, i).Summary.Flags
		if !f.Seen || !f.Flagged || f.FlagColor != 3 {
			t.Fatalf("flags of %d = %+v", i, f)
		}
	}
	if _, err := runMailctl(t, "--socket", e.srv.Socket, "flag", idArg(e.ids[0]), "--unflag", "--unseen"); err != nil {
		t.Fatal(err)
	}
	if f := e.get(t, e.ids[0]).Summary.Flags; f.Seen || f.Flagged || f.FlagColor != 0 {
		t.Fatalf("after --unflag --unseen: %+v", f)
	}
	if _, err := runMailctl(t, "--socket", e.srv.Socket, "flag", idArg(e.ids[2]), "--flag"); err != nil {
		t.Fatal(err)
	}
	if f := e.get(t, e.ids[2]).Summary.Flags; !f.Flagged || f.FlagColor != 1 {
		t.Fatalf("after --flag: %+v (red is color 1)", f)
	}
	for _, bad := range [][]string{
		{"flag", idArg(e.ids[0])},                       // no change
		{"flag", idArg(e.ids[0]), "--seen", "--unseen"}, // contradiction
		{"flag", idArg(e.ids[0]), "--flag", "--unflag"},
		{"flag", idArg(e.ids[0]), "--color", "9"},
		{"flag", "--seen"}, // no IDs
		{"flag", "x", "--seen"},
	} {
		if _, err := runMailctl(t, append([]string{"--socket", e.srv.Socket}, bad...)...); err == nil {
			t.Errorf("mailctl %s succeeded", strings.Join(bad, " "))
		}
	}
}

func TestMvAndRmCommands(t *testing.T) {
	e := opsFixture(t)
	out, err := runMailctl(t, "--socket", e.srv.Socket, "mv", idArg(e.archive), idArg(e.ids[0]), idArg(e.ids[1]))
	if err != nil || out != "moved 2 messages\n" {
		t.Fatalf("mv = %q, %v", out, err)
	}
	if got := e.get(t, e.ids[0]).Summary.MailboxIDs; !slices.Equal(got, []int64{e.archive}) {
		t.Fatalf("mailboxes after mv = %v, want [%d]", got, e.archive)
	}
	e.waitServerCount(t, "Archive", 2)

	out, err = runMailctl(t, "--socket", e.srv.Socket, "rm", idArg(e.ids[2]))
	if err != nil || out != "deleted 1 message\n" {
		t.Fatalf("rm = %q, %v", out, err)
	}
	e.waitServerCount(t, "Trash", 1)
	if _, err := runMailctl(t, "--socket", e.srv.Socket, "mv", "x", idArg(e.ids[3])); err == nil {
		t.Fatal("mv to a non-numeric mailbox succeeded")
	}
	if _, err := runMailctl(t, "--socket", e.srv.Socket, "mv", idArg(e.archive)); err == nil {
		t.Fatal("mv without message IDs succeeded")
	}
	if _, err := runMailctl(t, "--socket", e.srv.Socket, "rm"); err == nil {
		t.Fatal("rm without message IDs succeeded")
	}
}
