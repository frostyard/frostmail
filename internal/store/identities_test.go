package store

import (
	"errors"
	"testing"
)

func TestAddAndDeleteIdentity(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	a := insertAccount(t, d, sampleAccount("ann@icloud.example"))
	add := func(accountID int64, name, email string) (Identity, error) {
		var out Identity
		err := d.Tx(ctx, func(tx *Tx) error {
			var err error
			out, err = tx.AddIdentity(ctx, accountID, name, email)
			return err
		})
		return out, err
	}
	alias, err := add(a.ID, "Ann", "ann@ann.example")
	if err != nil || alias.AccountID != a.ID || alias.Email != "ann@ann.example" || alias.Name != "Ann" || alias.IsDefault {
		t.Fatalf("added = %+v, %v", alias, err)
	}
	if list, _ := d.ListIdentities(ctx, a.ID); len(list) != 2 || !list[0].IsDefault || list[1].ID != alias.ID {
		t.Errorf("identities = %+v", list)
	}
	if _, err := add(a.ID, "", "ANN@ann.example"); !errors.Is(err, ErrConflict) {
		t.Errorf("the same address again: %v", err)
	}
	if _, err := add(9999, "", "x@y.example"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown account: %v", err)
	}

	// A draft from the alias moves to the default identity when it goes.
	def, _ := d.DefaultIdentity(ctx, a.ID)
	var draft Draft
	if err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		draft, err = tx.CreateDraft(ctx, Draft{AccountID: a.ID, Kind: "new", MessageID: "m@x",
			Content: DraftContent{IdentityID: alias.ID, Subject: "Hi"}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	del := func(id int64) error { return d.Tx(ctx, func(tx *Tx) error { return tx.DeleteIdentity(ctx, id) }) }
	if err := del(def.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("deleting the default: %v", err)
	}
	if err := del(alias.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.GetDraft(ctx, draft.ID); got.Content.IdentityID != def.ID || got.Content.Subject != "Hi" {
		t.Errorf("draft = %+v", got.Content)
	}
	if list, _ := d.ListIdentities(ctx, a.ID); len(list) != 1 {
		t.Errorf("identities = %+v", list)
	}
	if err := del(alias.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting it again: %v", err)
	}
}
