package live_test

import (
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// #358: a conversation view's row follows a flag change of an older
// message of its conversation, which message.changed does not name the
// row for; a view without threads still refreshes the named row.
func TestConversationViewFollowsOlderMessageFlags(t *testing.T) {
	cfg := mailsync.Config{PollInterval: time.Hour, NoPrefetch: true}
	f := newFixtureWith(t, 0, rpctest.Options{Sync: &cfg})
	ctx := t.Context()
	var ids []int64
	if err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		ids, err = tx.InsertHeaders(ctx, f.acct, f.inbox, []store.MessageHeader{
			{UID: 1, MessageID: "older@x", Subject: "Plan", InternalDate: f.base},
			{UID: 2, MessageID: "newer@x", InReplyTo: "older@x", References: []string{"older@x"}, Subject: "Re: Plan", InternalDate: f.base.Add(time.Minute)},
		})
		if err != nil {
			return err
		}
		_, err = tx.AssignThreads(ctx, f.acct, ids)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	seen := true
	if err := f.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: ids[1:], Changes: api.FlagChanges{Seen: &seen}}); err != nil {
		t.Fatal(err)
	}
	ch := newChanges()
	threads := true
	v := live.OpenView(f.c, f.events, api.ViewQuery{Threads: &threads}, ch.notify)
	defer v.Close()
	v.Ensure(0, 10)
	ch.until(t, "the conversation's unread count of 1", func() bool {
		r, ok := v.Row(0)
		return ok && r.ID == ids[1] && r.Flags.Seen && r.ThreadUnread != nil && *r.ThreadUnread == 1
	})
	plain := live.OpenView(f.c, f.events, api.ViewQuery{}, ch.notify)
	defer plain.Close()
	plain.Ensure(0, 10)
	ch.until(t, "the older unread row without threads", func() bool { r, ok := plain.Row(1); return ok && r.ID == ids[0] && !r.Flags.Seen })
	if err := f.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: ids[:1], Changes: api.FlagChanges{Seen: &seen}}); err != nil {
		t.Fatal(err)
	}
	ch.until(t, "the conversation's unread count of 0 after only its older message changed", func() bool {
		r, ok := v.Row(0)
		return ok && r.ThreadUnread != nil && *r.ThreadUnread == 0
	})
	ch.until(t, "the named row's Seen without threads", func() bool {
		r, ok := plain.Row(1)
		return ok && r.ID == ids[0] && r.Flags.Seen && r.ThreadUnread == nil
	})
}
