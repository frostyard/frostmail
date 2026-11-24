package mainwin_test

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// rowHasDot reports whether the list row whose subject is subject shows
// the unread dot: an element named "Unread" on the row's first line, just
// above the subject (the filter bar's Unread tab is elsewhere).
func rowHasDot(f *syncFixture, subject string) bool {
	rect, ok := f.tt.Find(subject)
	if !ok {
		return false
	}
	// The current Dot is exposed as a group; also accept its image role.
	for _, a := range f.tt.AccessAll() {
		if (a.Role == ui.RoleImage || a.Role == ui.RoleGroup) && a.Name == "Unread" && a.Bounds.Y >= rect.Y-30 && a.Bounds.Y < rect.Y {
			return true
		}
	}
	return false
}

// #358: a conversation row shows the unread dot while any of its messages
// in the list's scope is unread, also when its newest message is read;
// with Conversations off each row keeps its own message's dot.
func TestConversationUnreadDotFollowsAllMessages(t *testing.T) {
	f := newSyncFixture(t)
	ctx := t.Context()
	const older = "Lunch on Thursday?"
	const newer = "Re: Lunch on Thursday?"
	for _, raw := range []string{
		"From: Bob Builder <bob@x.test>\r\nTo: me@x.test\r\nSubject: Lunch on Thursday?\r\nMessage-ID: <thread-0001@x.test>\r\nDate: Tue, 10 Nov 2026 11:15:00 -0400\r\n\r\nLunch?\r\n",
		"From: Me <me@x.test>\r\nTo: bob@x.test\r\nSubject: Re: Lunch on Thursday?\r\nMessage-ID: <thread-0002@x.test>\r\nIn-Reply-To: <thread-0001@x.test>\r\nReferences: <thread-0001@x.test>\r\nDate: Tue, 10 Nov 2026 12:02:00 -0400\r\n\r\nYes.\r\n",
	} {
		if _, err := f.mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: f.acct}); err != nil {
		t.Fatal(err)
	}
	f.until(t, "Follow Up to show", func() bool { return f.tt.HasText("Follow Up") })
	// The same subject also appears in Follow Up; collapse that section so
	// Find identifies the message-list subject rather than the sidebar copy.
	if err := f.tt.Click("Follow Up"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the list subject", func() bool { return f.tt.HasText(newer) })
	f.until(t, "the unread conversation with its count", func() bool { return f.tt.HasText(newer) && f.tt.HasText("2") && rowHasDot(f, newer) })
	ids, err := f.srv.DB.ViewIDs(ctx, store.ViewFilter{})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	var olderID, newerID int64
	for _, r := range rows {
		if r.Subject == older {
			olderID = r.ID
		}
		if r.Subject == newer {
			newerID = r.ID
		}
	}
	if olderID == 0 || newerID == 0 {
		t.Fatal("missing conversation messages")
	}
	setSeen := func(id int64, seen bool) {
		t.Helper()
		if err := f.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{id}, Changes: api.FlagChanges{Seen: &seen}}); err != nil {
			t.Fatal(err)
		}
	}
	setSeen(newerID, true)
	// Wait for the server change to reach the window before checking the dot.
	f.until(t, "the newest read with the conversation still unread", func() bool { return f.tt.HasText("2 unread") && rowHasDot(f, newer) })
	setSeen(olderID, true)
	f.until(t, "the conversation dot gone after the older message is read", func() bool { return f.tt.HasText(newer) && !rowHasDot(f, newer) })
	setSeen(olderID, false)
	if err := f.tt.Click("Sort by Date"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Conversations"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "individual message dots with Conversations off", func() bool {
		return f.tt.HasText(older) && f.tt.HasText(newer) && rowHasDot(f, older) && !rowHasDot(f, newer)
	})
}
