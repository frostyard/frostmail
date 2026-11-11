package mainwin_test

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/store"
)

// A row leaves the list while its action is still running, so an undo at
// that moment waits for the action instead of finding nothing to undo.
func TestUndoWaitsForTheActionRunning(t *testing.T) {
	f := newSyncFixture(t)
	ctx := t.Context()
	info, err := f.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.c.View().Range(ctx, &api.ViewRangeParams{ID: info.ID, Start: 0, End: 1})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	id := rows[0].ID

	waiting := make(chan struct{})
	u := mail.Undo{BeforeUndo: func() { close(waiting) }}
	undone := make(chan bool, 1)
	_, err = u.Move(ctx, f.c, []int64{id}, func() error {
		if err := f.c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: []int64{id}}); err != nil {
			return err
		}
		go func() {
			ok, err := u.Do(ctx, f.c)
			if err != nil {
				t.Errorf("undo: %v", err)
			}
			undone <- ok
		}()
		select {
		case <-waiting:
		case <-time.After(30 * time.Second):
			t.Fatal("the undo did not reach its wait")
		}
		select {
		case ok := <-undone:
			t.Errorf("the undo ended during the action: %v", ok)
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ok := <-undone:
		if !ok {
			t.Error("the undo found nothing to undo")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the undo did not end")
	}
}

// messageID finds a message by its subject, in an open view of all mail.
func (f *syncFixture) messageID(t *testing.T, subject string) int64 {
	t.Helper()
	ctx := t.Context()
	info, err := f.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.c.View().Range(ctx, &api.ViewRangeParams{ID: info.ID, Start: 0, End: info.Count})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Subject == subject {
			return r.ID
		}
	}
	t.Fatalf("no message %q in %+v", subject, rows)
	return 0
}

func (f *syncFixture) mailboxIDs(t *testing.T, id int64) []int64 {
	t.Helper()
	got, err := f.c.Message().Summaries(t.Context(), &api.MessageSummariesParams{IDs: []int64{id}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		return nil
	}
	return got[0].MailboxIDs
}

// landed reports whether a message's mailboxes have reached the server:
// until then maild skips a delete of it.
func (f *syncFixture) landed(t *testing.T, id int64) bool {
	t.Helper()
	ctx := t.Context()
	ok := false
	if err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mems, err := tx.Memberships(ctx, []int64{id})
		if err != nil {
			return err
		}
		ok = len(mems) > 0 && !slices.ContainsFunc(mems, func(m store.Membership) bool { return m.UID == 0 })
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return ok
}

// Of two selected messages, another client moves one to Trash. Delete then
// acts on the one left: the other would be erased for good.
func TestDeleteSparesAMessageThatLeftTheList(t *testing.T) {
	f := newSyncFixture(t)
	beta, gamma := f.messageID(t, "Beta plan"), f.messageID(t, "Gamma notes")
	if err := f.tt.Click("Beta plan"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	if err := f.tt.ClickWith(ui.Cmd, "Gamma notes"); err != nil {
		t.Fatal(err)
	}
	f.has(t, "both selected", "2 Messages Selected")

	if err := f.c.Message().Delete(t.Context(), &api.MessageDeleteParams{IDs: []int64{beta}}); err != nil {
		t.Fatal(err)
	}
	f.hasNot(t, "the row gone to Trash", "Beta plan")
	f.hasNot(t, "the departed message out of the selection", "2 Messages Selected")
	f.until(t, "the move's replay", func() bool { return f.landed(t, beta) })

	f.tt.Key(0, ui.KeyDelete)
	f.hasNot(t, "the selected row deleted", "Gamma notes")
	if got := f.mailboxIDs(t, gamma); len(got) == 0 {
		t.Error("the selected message was erased, not moved to Trash")
	}
	if got := f.mailboxIDs(t, beta); len(got) == 0 {
		t.Error("the message another client moved to Trash was erased")
	}
}

// Delete and Backspace act in the list alone (docs/specs/ui.md, Keyboard
// map): with the sidebar focused they leave the selected mail alone.
func TestDeleteKeysActOnlyInTheList(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.tt.Click("Beta plan"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	f.tt.Frame()
	f.tt.Key(ui.Shift, ui.KeyTab) // list to sidebar
	f.tt.Frame()
	f.tt.Frame()
	beta := f.messageID(t, "Beta plan")
	before := f.mailboxIDs(t, beta)
	for _, key := range []ui.Key{ui.KeyDelete, ui.KeyBackspace} {
		f.tt.Key(0, key)
		for range 40 {
			f.tt.Frame()
			time.Sleep(5 * time.Millisecond)
		}
		if got := f.mailboxIDs(t, beta); !slices.Equal(got, before) {
			t.Fatalf("with the sidebar focused, key %v moved the message to %v", key, got)
		}
	}
	f.tt.Key(0, ui.KeyTab) // back to the list
	f.tt.Frame()
	f.tt.Frame()
	f.tt.Key(0, ui.KeyDelete)
	f.hasNot(t, "the row deleted from the list", "Beta plan")
}
