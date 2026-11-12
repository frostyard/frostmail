package store

import (
	"reflect"
	"testing"
)

// TestGetMessages: a batch of IDs answers what GetMessage answers for each
// of them, in the order given, skipping an ID with no row, and no IDs
// answers an empty slice (issue #292: the tools read many messages in a
// fixed number of queries).
func TestGetMessages(t *testing.T) {
	d := readFixture(t)
	ctx := t.Context()
	var want []MessageDetail
	for _, id := range []int64{2, 1} {
		m, err := d.GetMessage(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, m)
	}
	got, err := d.GetMessages(ctx, []int64{2, 99, 1})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetMessages(2, 99, 1) =\n%+v\nwant\n%+v", got, want)
	}
	for _, ids := range [][]int64{nil, {}} {
		if got, err := d.GetMessages(ctx, ids); err != nil || got == nil || len(got) != 0 {
			t.Fatalf("GetMessages(%v) = %v, %v; want an empty, non-nil slice", ids, got, err)
		}
	}
	if got, err := d.GetMessages(ctx, []int64{99}); err != nil || len(got) != 0 {
		t.Fatalf("GetMessages(99) = %v, %v; want none", got, err)
	}
}

// TestGetMessagesTakesMoreIDsThanSQLiteBinds: message.getMany sets no limit
// on its IDs, so GetMessages must answer a batch longer than SQLite's
// 32,766 bound variables, with the missing IDs skipped.
func TestGetMessagesTakesMoreIDsThanSQLiteBinds(t *testing.T) {
	d := readFixture(t)
	ctx := t.Context()
	ids := []int64{2}
	for id := range int64(40_000) {
		ids = append(ids, 1_000_000+id)
	}
	ids = append(ids, 1)
	got, err := d.GetMessages(ctx, ids)
	if err != nil {
		t.Fatalf("GetMessages of %d IDs: %v", len(ids), err)
	}
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 1 {
		t.Fatalf("GetMessages of %d IDs = %d messages, want 2 then 1", len(ids), len(got))
	}
}
