package store

import (
	"errors"
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func createSmart(t *testing.T, d *DB, s SmartMailbox) SmartMailbox {
	t.Helper()
	var out SmartMailbox
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		var err error
		out, err = tx.CreateSmartMailbox(t.Context(), s)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func smartNames(t *testing.T, d *DB) []string {
	t.Helper()
	list, err := d.SmartMailboxes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for i, s := range list {
		if s.Position != i {
			t.Errorf("%s at position %d, index %d", s.Name, s.Position, i)
		}
		out = append(out, s.Name)
	}
	return out
}

// TestSmartMailboxesKeepOrder: created at the end, moved, deleted, the
// positions stay 0..n-1 and the conditions read back as stored.
func TestSmartMailboxesKeepOrder(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	flagged := one("flagged", "is", "true")
	a := createSmart(t, d, SmartMailbox{Name: "A", Conditions: flagged, IncludeSent: true})
	b := createSmart(t, d, SmartMailbox{Name: "B", Conditions: api.Conditions{Match: api.ConditionMatchAny, Conditions: []api.Condition{}}})
	c := createSmart(t, d, SmartMailbox{Name: "C", Conditions: flagged})
	if got := smartNames(t, d); !slices.Equal(got, []string{"A", "B", "C"}) {
		t.Fatalf("created = %v", got)
	}
	got, err := d.SmartMailbox(ctx, a.ID)
	if err != nil || got.Conditions.Match != api.ConditionMatchAll || len(got.Conditions.Conditions) != 1 ||
		got.Conditions.Conditions[0] != flagged.Conditions[0] || !got.IncludeSent || got.IncludeTrash {
		t.Errorf("A = %+v, %v", got, err)
	}
	if got, _ := d.SmartMailbox(ctx, b.ID); got.Conditions.Conditions == nil || got.Conditions.Match != api.ConditionMatchAny {
		t.Errorf("B = %+v", got)
	}
	move := func(id int64, pos int) {
		t.Helper()
		if err := d.Tx(ctx, func(tx *Tx) error { return tx.MoveSmartMailbox(ctx, id, pos) }); err != nil {
			t.Fatal(err)
		}
	}
	move(c.ID, 0)
	if got := smartNames(t, d); !slices.Equal(got, []string{"C", "A", "B"}) {
		t.Errorf("C to 0 = %v", got)
	}
	move(c.ID, 99)
	if got := smartNames(t, d); !slices.Equal(got, []string{"A", "B", "C"}) {
		t.Errorf("C to the end = %v", got)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.DeleteSmartMailbox(ctx, a.ID) }); err != nil {
		t.Fatal(err)
	}
	if got := smartNames(t, d); !slices.Equal(got, []string{"B", "C"}) {
		t.Errorf("after deleting A = %v", got)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.DeleteSmartMailbox(ctx, a.ID) }); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting A again: %v", err)
	}
	n := 0
	for _, e := range *events {
		if e.Event == "smart.changed" {
			n++
		}
	}
	if n != 6 {
		t.Errorf("%d smart.changed events, want 6 (3 creates, 2 moves, 1 delete)", n)
	}
}

// TestDeletingTheScopeSmartMailbox: the notification scope that named a
// deleted smart mailbox goes back to Inbox only.
func TestDeletingTheScopeSmartMailbox(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	s := createSmart(t, d, SmartMailbox{Name: "Urgent", Conditions: one("flagged", "is", "true")})
	settings := DefaultSettings()
	settings.NotifyScope, settings.NotifySmartID = string(api.NotifyScopeSmart), s.ID
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.SetSettings(ctx, settings) }); err != nil {
		t.Fatal(err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.DeleteSmartMailbox(ctx, s.ID) }); err != nil {
		t.Fatal(err)
	}
	got, err := d.Settings(ctx)
	if err != nil || got.NotifyScope != string(api.NotifyScopeInbox) || got.NotifySmartID != 0 {
		t.Errorf("settings = %+v, %v; want inbox", got, err)
	}
}

// TestSmartMailboxViews: a smart mailbox lists its conditions' messages,
// leaving out Trash and Sent unless it includes them; a filter narrows
// any view; FilterIDs takes what a filter lists of given messages.
func TestSmartMailboxViews(t *testing.T) {
	f := newConditionsFixture(t)
	ctx := t.Context()
	all := api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{}}
	none := createSmart(t, f.d, SmartMailbox{Name: "Everything", Conditions: all})
	trash := createSmart(t, f.d, SmartMailbox{Name: "And Trash", Conditions: all, IncludeTrash: true})
	both := createSmart(t, f.d, SmartMailbox{Name: "And both", Conditions: all, IncludeTrash: true, IncludeSent: true})
	flagged := createSmart(t, f.d, SmartMailbox{Name: "Flagged", Conditions: one("flagged", "is", "true"), IncludeTrash: true})
	ids := func(filter ViewFilter) []int64 {
		t.Helper()
		got, err := f.d.ViewIDs(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		return got
	}
	for name, c := range map[string]struct {
		filter ViewFilter
		want   []int64
	}{
		"neither":          {ViewFilter{SmartMailboxID: none.ID}, []int64{f.lunch, f.invoice}},
		"trash":            {ViewFilter{SmartMailboxID: trash.ID}, []int64{f.lunch, f.invoice, f.old}},
		"both":             {ViewFilter{SmartMailboxID: both.ID}, []int64{f.lunch, f.invoice, f.old, f.report}},
		"flagged":          {ViewFilter{SmartMailboxID: flagged.ID}, []int64{f.lunch, f.old}},
		"unknown":          {ViewFilter{SmartMailboxID: 9999}, nil},
		"filtered":         {ViewFilter{SmartMailboxID: both.ID, Filter: &api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{{Field: "tome", Op: "is", Value: "true"}}}}, []int64{f.lunch, f.old}},
		"filter and conds": {ViewFilter{Conditions: ptrTo(one("role", "is", "trash")), Filter: ptrTo(one("flagged", "is", "true"))}, []int64{f.old}},
	} {
		if got := ids(c.filter); !slices.Equal(got, c.want) {
			t.Errorf("%s = %v, want %v", name, got, c.want)
		}
	}
	got, err := f.d.FilterIDs(ctx, ViewFilter{SmartMailboxID: flagged.ID}, []int64{f.old, f.invoice, f.report})
	if err != nil || !slices.Equal(got, []int64{f.old}) {
		t.Errorf("FilterIDs = %v, %v; want [old]", got, err)
	}
	_, unread, err := f.d.CountView(ctx, ViewFilter{SmartMailboxID: flagged.ID})
	if err != nil || unread != 1 {
		t.Errorf("unread in Flagged = %d, %v; want 1", unread, err)
	}
}

func ptrTo[T any](v T) *T { return &v }
