package store

import (
	"testing"
	"time"
)

func TestDraftChangesAreAlwaysNewerThanTheSavedCopy(t *testing.T) {
	d, acct, source := draftFixture(t)
	ctx := t.Context()
	dr := createDraft(t, d, sampleDraft(acct, source))
	// The copy is saved at the draft's updated_at; the clock does not move.
	if err := d.Tx(ctx, func(tx *Tx) error {
		return tx.SetDraftServerCopy(ctx, dr.ID, 7, dr.UpdatedAt)
	}); err != nil {
		t.Fatal(err)
	}
	var updated, touched Draft
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		if updated, err = tx.UpdateDraftContent(ctx, dr.ID, DraftContent{Subject: "edited"}); err != nil {
			return err
		}
		if err := tx.TouchDraft(ctx, dr.ID); err != nil {
			return err
		}
		touched, err = tx.GetDraft(ctx, dr.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.UpdatedAt.Equal(dr.UpdatedAt.Add(time.Millisecond)) {
		t.Errorf("updated_at = %v, want 1ms after %v", updated.UpdatedAt, dr.UpdatedAt)
	}
	if !touched.UpdatedAt.Equal(dr.UpdatedAt.Add(2 * time.Millisecond)) {
		t.Errorf("touched updated_at = %v, want 2ms after %v", touched.UpdatedAt, dr.UpdatedAt)
	}
	list, err := d.DraftsToSave(ctx, acct, touched.UpdatedAt)
	if err != nil || len(list) != 1 {
		t.Fatalf("DraftsToSave = %+v, %v; want the edited draft", list, err)
	}
	// A later clock wins over the bump.
	d.Now = func() time.Time { return testNow.Add(time.Hour) }
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.TouchDraft(ctx, dr.ID) }); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.GetDraft(ctx, dr.ID); !got.UpdatedAt.Equal(testNow.Add(time.Hour)) {
		t.Errorf("updated_at = %v, want the clock", got.UpdatedAt)
	}
}

func TestInsertHeadersCountsAddresses(t *testing.T) {
	d, _ := openTest(t)
	acct, inbox, _ := mailboxFixture(t, d)
	insertHeaders(t, d, acct, inbox, header(1, "One"), header(2, "Two"))
	got, err := d.SuggestAddresses(t.Context(), "bob", 0)
	if err != nil || len(got) != 1 || got[0].Name != "Bob Builder" {
		t.Fatalf("suggest(bob) = %+v, %v", got, err)
	}
	var count int
	var lastSeen string
	if err := d.db.QueryRowContext(t.Context(), `SELECT count, last_seen FROM addresses WHERE address = 'carol@mailtest.test'`).
		Scan(&count, &lastSeen); err != nil {
		t.Fatal(err)
	}
	if count != 2 || lastSeen != FormatTime(header(1, "").Date) {
		t.Errorf("carol: count %d, last seen %s", count, lastSeen)
	}
}
