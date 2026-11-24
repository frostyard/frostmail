package store

import (
	"reflect"
	"testing"
)

func TestThreadUnread(t *testing.T) {
	d := threadViewFixture(t)
	ctx := t.Context()
	// Thread 101's newest row is read; its older message is unread.
	// Thread 100 has only an unread archive message and a deleted unread one.
	if _, err := d.db.ExecContext(ctx, `UPDATE messages SET seen = CASE WHEN id IN (1, 6) THEN 1 ELSE 0 END`); err != nil {
		t.Fatal(err)
	}
	ids := []int64{1, 6, 4, 999}
	want := map[int64]int64{1: 0, 6: 1, 4: 1}
	for _, unread := range []*bool{nil, viewBool(true), viewBool(false)} {
		f := ViewFilter{Role: "inbox", Threads: true, Unread: unread, Keep: []int64{3}}
		got, err := d.ThreadUnread(ctx, f, ids)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("unread %v: %v, %v; want %v", unread, got, err, want)
		}
		if f.Unread != unread || !f.Threads || !reflect.DeepEqual(f.Keep, []int64{3}) {
			t.Fatal("caller filter changed")
		}
	}
	// Without the role constraint, the archive message counts (not the deleted one).
	got, err := d.ThreadUnread(ctx, ViewFilter{}, []int64{1})
	if err != nil || !reflect.DeepEqual(got, map[int64]int64{1: 1}) {
		t.Fatalf("all mail: %v, %v", got, err)
	}
	got, err = d.ThreadUnread(ctx, ViewFilter{Accounts: []int64{2}}, ids)
	if err != nil || !reflect.DeepEqual(got, map[int64]int64{1: 0, 6: 0, 4: 0}) {
		t.Fatalf("other account: %v, %v", got, err)
	}
	if _, err := d.db.ExecContext(ctx, `UPDATE messages SET seen = 1 WHERE id IN (2, 4)`); err != nil {
		t.Fatal(err)
	}
	got, err = d.ThreadUnread(ctx, ViewFilter{Role: "inbox"}, []int64{6, 4})
	if err != nil || !reflect.DeepEqual(got, map[int64]int64{6: 0, 4: 0}) {
		t.Fatalf("all read: %v, %v", got, err)
	}
	got, err = d.ThreadUnread(ctx, ViewFilter{}, nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty: %#v, %v", got, err)
	}
}
