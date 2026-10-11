package engine

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/store"
)

// TestUndoDelayFollowsTheSetting: the undoDelay setting decides how long a
// sent message waits, 0 included, unless Deps.UndoDelay overrides it.
func TestUndoDelayFollowsTheSetting(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	d := Deps{DB: db}
	if got, err := d.undoDelay(ctx); err != nil || got != 10*time.Second {
		t.Errorf("default = %v, %v; want 10s", got, err)
	}
	for _, secs := range []int{0, 30} {
		s := store.DefaultSettings()
		s.UndoDelay = secs
		if err := db.Tx(ctx, func(tx *store.Tx) error { return tx.SetSettings(ctx, s) }); err != nil {
			t.Fatal(err)
		}
		if got, err := d.undoDelay(ctx); err != nil || got != time.Duration(secs)*time.Second {
			t.Errorf("setting %d = %v, %v", secs, got, err)
		}
	}
	d.UndoDelay = 50 * time.Millisecond
	if got, _ := d.undoDelay(ctx); got != 50*time.Millisecond {
		t.Errorf("override = %v, want 50ms", got)
	}
}
