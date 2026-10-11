//go:build integration

package mailsync_test

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
)

// TestDovecotMailboxes: against Dovecot (test2), a nested mailbox made here
// is renamed with the one inside it and deleted with it, on the server too.
func TestDovecotMailboxes(t *testing.T) {
	opts := dovecotTest2(t)
	ctx := t.Context()
	other, err := imapx.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	onServer := func() []string {
		list, err := other.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, mb := range list {
			out = append(out, mb.Path)
		}
		return out
	}
	h := newHarness(t, opts, opts.Password)
	h.waitPhase(api.SyncPhaseIdle)

	parent, err := h.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: h.acct, Name: "it-Parent"})
	if err != nil {
		t.Fatal(err)
	}
	h.waitReplayed()
	child, err := h.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: h.acct, Name: "Child", ParentID: &parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	sep := child.Delimiter
	h.waitReplayed()
	waitUntil(t, 5*time.Second, "both on the server", func() bool {
		return slices.Contains(onServer(), "it-Parent"+sep+"Child")
	})

	if _, err := h.c.Mailbox().Rename(ctx, &api.MailboxRenameParams{ID: parent.ID, Name: "it-Renamed"}); err != nil {
		t.Fatal(err)
	}
	if got := h.mailboxByPath("it-Renamed" + sep + "Child"); got.ID != child.ID {
		t.Errorf("the child after the rename = %+v", got)
	}
	h.waitReplayed()
	waitUntil(t, 5*time.Second, "the rename on the server", func() bool {
		got := onServer()
		return slices.Contains(got, "it-Renamed"+sep+"Child") && !slices.Contains(got, "it-Parent")
	})

	if err := h.c.Mailbox().Delete(ctx, &api.MailboxDeleteParams{ID: parent.ID}); err != nil {
		t.Fatal(err)
	}
	h.waitReplayed()
	waitUntil(t, 5*time.Second, "both gone from the server", func() bool {
		return !slices.ContainsFunc(onServer(), func(p string) bool { return p == "it-Renamed" || p == "it-Renamed"+sep+"Child" })
	})
}
