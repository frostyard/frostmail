package store

import (
	"slices"
	"testing"
	"time"
)

func TestMatchingIDsCharacterization(t *testing.T) {
	f := newConditionsFixture(t)
	c := one("flagged", "is", "true")

	got, err := f.d.MatchingIDs(t.Context(), c, []int64{f.report, f.old, f.invoice, f.lunch}, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{f.lunch, f.old}; !slices.Equal(got, want) {
		t.Fatalf("MatchingIDs = %v, want %v", got, want)
	}

	got, err = f.d.MatchingIDs(t.Context(), c, []int64{999999}, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("MatchingIDs with no matches = %v, want nil", got)
	}
}

func TestClearMessageRemindersCharacterization(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, _ := mailboxFixture(t, d)
	ids := insertHeaders(t, d, acct, inbox, header(1, "a"), header(2, "b"), header(3, "c"))

	if err := d.Tx(ctx, func(tx *Tx) error {
		return tx.SetMessageReminders(ctx, []int64{ids[2], ids[0]}, testNow)
	}); err != nil {
		t.Fatal(err)
	}

	var got []int64
	if err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		got, err = tx.ClearMessageReminders(ctx, []int64{ids[1], ids[2], ids[0]})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []int64{ids[0], ids[2]}; !slices.Equal(got, want) {
		t.Fatalf("ClearMessageReminders = %v, want %v", got, want)
	}

	if err := d.Tx(ctx, func(tx *Tx) error {
		got, err := tx.ClearMessageReminders(ctx, []int64{ids[0], ids[2]})
		if err != nil {
			return err
		}
		if got != nil {
			t.Fatalf("second ClearMessageReminders = %v, want nil", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
