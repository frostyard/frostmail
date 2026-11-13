package main

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"

	"github.com/frostyard/frostmail/internal/notify"
)

// P-805: Reply goes to the app, and Archive and Trash to the sync engine;
// an action the engine refuses is logged, not fatal.
func TestMailActions(t *testing.T) {
	var replied []int64
	type move struct {
		id     int64
		action string
	}
	var moved []move
	act := mailActions(t.Context(), func(_ context.Context, id int64, action string) error {
		moved = append(moved, move{id, action})
		if id == 3 {
			return errors.New("read-only")
		}
		return nil
	}, func(id int64) { replied = append(replied, id) }, slog.New(slog.DiscardHandler))
	act(1, notify.ActionReply)
	act(2, notify.ActionArchive)
	act(3, notify.ActionTrash)
	if !reflect.DeepEqual(replied, []int64{1}) {
		t.Errorf("replied to %v, want message 1", replied)
	}
	if want := []move{{2, notify.ActionArchive}, {3, notify.ActionTrash}}; !reflect.DeepEqual(moved, want) {
		t.Errorf("moved %+v, want %+v", moved, want)
	}
}
