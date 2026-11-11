package mainwin_test

import (
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
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

	var u mail.Undo
	undone := make(chan bool, 1)
	err = u.Move(ctx, f.c, []int64{id}, func() error {
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
		case ok := <-undone:
			t.Errorf("the undo ended during the action: %v", ok)
		case <-time.After(200 * time.Millisecond):
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
