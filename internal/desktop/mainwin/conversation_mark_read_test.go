package mainwin_test

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// seenOf is each message's seen flag in maild.
func (f *syncFixture) seenOf(t *testing.T, ids ...int64) map[int64]bool {
	t.Helper()
	rows, err := f.c.Message().Summaries(t.Context(), &api.MessageSummariesParams{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[int64]bool, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Flags.Seen
	}
	return out
}

// #380: in conversation mode, a row that shows the unread dot counts as
// unread for Mark as Read/Unread; Mark as Read marks its whole
// conversation read, Undo takes that back, and Mark as Unread marks the
// row's own message unread. Two rows are selected, so no reader opens
// and marks them read on its own.
func TestConversationMarkAsReadMarksTheConversation(t *testing.T) {
	f, olderID, newerID := lunchFixture(t)
	gammaID := f.idOf(t, "Gamma notes")
	f.setSeen(t, newerID, true)
	f.until(t, "the dot of the older unread message", func() bool { return f.tt.HasText("2 unread") && rowHasDot(f, lunchNewer) })
	if err := f.tt.Click("Gamma notes"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ClickWith(ui.Cmd, lunchNewer); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	if err := f.tt.RightClick(lunchNewer); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Mark as Read"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the whole conversation read", func() bool {
		s := f.seenOf(t, olderID, newerID, gammaID)
		return s[olderID] && s[newerID] && s[gammaID] && !rowHasDot(f, lunchNewer)
	})

	f.tt.Key(ui.Cmd, ui.KeyZ)
	f.until(t, "the older message unread again after Undo", func() bool {
		s := f.seenOf(t, olderID, newerID, gammaID)
		return !s[olderID] && s[newerID] && s[gammaID] && rowHasDot(f, lunchNewer)
	})

	f.setSeen(t, olderID, true)
	f.until(t, "no dot on the read conversation", func() bool { return !rowHasDot(f, lunchNewer) })
	if err := f.tt.RightClick(lunchNewer); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Mark as Unread"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the rows' own messages unread", func() bool {
		s := f.seenOf(t, olderID, newerID, gammaID)
		return s[olderID] && !s[newerID] && !s[gammaID] && rowHasDot(f, lunchNewer)
	})
}

// idOf is the ID of the message whose subject is subject.
func (f *syncFixture) idOf(t *testing.T, subject string) int64 {
	t.Helper()
	ids, err := f.srv.DB.ViewIDs(t.Context(), store.ViewFilter{})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.c.Message().Summaries(t.Context(), &api.MessageSummariesParams{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Subject == subject {
			return r.ID
		}
	}
	t.Fatalf("no message %q", subject)
	return 0
}
