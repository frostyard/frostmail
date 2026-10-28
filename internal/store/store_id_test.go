package store

import (
	"encoding/hex"
	"path/filepath"
	"testing"
)

func TestStoreIDIsStableAcrossReopen(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "frostmail.db")
	d, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := d.StoreID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := hex.DecodeString(id); err != nil || len(id) != 32 || len(raw) != 16 {
		t.Fatalf("StoreID = %q; want 32 hex characters", id)
	}
	if again, err := d.StoreID(ctx); err != nil || again != id {
		t.Fatalf("second StoreID = %q, %v; want %q", again, err, id)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if again, err := d.StoreID(ctx); err != nil || again != id {
		t.Fatalf("StoreID after reopen = %q, %v; want %q", again, err, id)
	}
}

func TestStoreIDDiffersBetweenDatabases(t *testing.T) {
	ctx := t.Context()
	ids := map[string]bool{}
	for range 2 {
		d, err := Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
		if err != nil {
			t.Fatal(err)
		}
		id, err := d.StoreID(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ids[id] = true
		_ = d.Close()
	}
	if len(ids) != 2 {
		t.Fatalf("two databases got the same store id")
	}
}
