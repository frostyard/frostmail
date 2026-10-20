package engine_test

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// TestUnsavedDrafts: draft.unsaved lists the drafts no stored message
// holds and the Outbox does not, newest first, so Drafts can show them
// before their server copy arrives.
func TestUnsavedDrafts(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	a, err := c.Account().Create(ctx, createParams("me@mailtest.test"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Account().Create(ctx, createParams("you@mailtest.test"))
	if err != nil {
		t.Fatal(err)
	}
	draft := func(acct int64, subject string) int64 {
		t.Helper()
		d, err := c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew, AccountID: &acct,
			To: []api.Address{{Address: "ada@example.com"}}})
		if err != nil {
			t.Fatal(err)
		}
		content := d.Content
		content.Subject = subject
		if _, err := c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: content}); err != nil {
			t.Fatal(err)
		}
		return d.ID
	}
	subjects := func(list []api.Draft) []string {
		var out []string
		for _, d := range list {
			out = append(out, d.Content.Subject)
		}
		return out
	}
	kept := draft(a.ID, "Kept")
	saved := draft(a.ID, "Saved")
	queued := draft(a.ID, "Queued")
	gone := draft(a.ID, "Gone")
	draft(b.ID, "Other")

	// The server copy of Saved arrives in account A's Drafts.
	dr, err := srv.DB.GetDraft(ctx, saved)
	if err != nil {
		t.Fatal(err)
	}
	err = srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{{Path: "Drafts", Role: api.MailboxRoleDrafts, Selectable: true}})
		if err != nil {
			return err
		}
		now := time.Now()
		_, err = tx.InsertHeaders(ctx, a.ID, mbs[0].ID, []store.MessageHeader{{UID: 1, Subject: "Saved", MessageID: dr.MessageID,
			From: store.Address{Addr: "me@mailtest.test"}, Date: now, InternalDate: now, Flags: store.Flags{Draft: true, Seen: true}}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Draft().Send(ctx, &api.DraftSendParams{ID: queued}); err != nil {
		t.Fatal(err)
	}
	if err := c.Draft().Delete(ctx, &api.DraftDeleteParams{ID: gone}); err != nil {
		t.Fatal(err)
	}

	all, err := c.Draft().Unsaved(ctx, &api.DraftUnsavedParams{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := subjects(all), []string{"Other", "Kept"}; !slices.Equal(got, want) {
		t.Errorf("draft.unsaved = %q, want %q: not Saved, whose copy is stored, nor Queued, in the Outbox", got, want)
	}
	one, err := c.Draft().Unsaved(ctx, &api.DraftUnsavedParams{AccountID: &a.ID})
	if err != nil || len(one) != 1 || one[0].ID != kept || one[0].AccountID != a.ID {
		t.Errorf("draft.unsaved of account %d = %+v, %v; want Kept alone", a.ID, one, err)
	}
	// An agent cannot call it: the Drafts list is the app's.
	agent, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindAgent, Token: "t", Scope: api.Scope{All: true, Write: true}})
	if _, err := agent.Draft().Unsaved(ctx, &api.DraftUnsavedParams{}); err == nil {
		t.Error("draft.unsaved answered an agent")
	}
}
