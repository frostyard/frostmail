// CONTRACT TEST for task card T-0096 (docs/tasks). Do not edit.
package engine_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
)

// TestIdentityAliases: an address added to an account is one of its own,
// so an invitation to it can be answered; removing it undoes that.
func TestIdentityAliases(t *testing.T) {
	e := newInviteEnv(t)
	ctx := t.Context()
	invite := e.mail(t, request("REQUEST", 1, "ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:me@alias.example"))
	if inv := e.invitation(t, invite); inv.CanRespond {
		t.Fatal("an invitation to an unknown address can be answered")
	}
	ids := e.c.Identity()
	list, err := ids.List(ctx, &api.IdentityListParams{AccountID: &e.acct})
	if err != nil || len(list) != 1 || !list[0].IsDefault {
		t.Fatalf("identities = %+v, %v", list, err)
	}
	def := list[0]

	added, err := ids.Create(ctx, &api.IdentityCreateParams{AccountID: e.acct, Email: "  me@alias.example "})
	if err != nil || added.Email != "me@alias.example" || added.Name != def.Name || added.IsDefault || added.AccountID != e.acct {
		t.Fatalf("created = %+v, %v", added, err)
	}
	if inv := e.invitation(t, invite); !inv.CanRespond || inv.Event.Answer == nil || *inv.Event.Answer != api.PartStatNeedsaction {
		t.Errorf("after adding the alias: canRespond %v, answer %v", inv.CanRespond, inv.Event.Answer)
	}
	named, err := ids.Create(ctx, &api.IdentityCreateParams{AccountID: e.acct, Email: "work@alias.example", Name: ptr("Ann at Work")})
	if err != nil || named.Name != "Ann at Work" {
		t.Errorf("named = %+v, %v", named, err)
	}
	if list, _ := ids.List(ctx, &api.IdentityListParams{AccountID: &e.acct}); len(list) != 3 || list[0].ID != def.ID {
		t.Errorf("identities = %+v", list)
	}

	for _, c := range []struct {
		p    api.IdentityCreateParams
		want api.ErrorCode
	}{
		{api.IdentityCreateParams{AccountID: e.acct, Email: "ME@alias.example"}, api.CodeConflict},
		{api.IdentityCreateParams{AccountID: e.acct, Email: "not an address"}, api.CodeInvalidParams},
		{api.IdentityCreateParams{AccountID: e.acct, Email: "x@alias.example", Name: ptr("Two\nlines")}, api.CodeInvalidParams},
		{api.IdentityCreateParams{AccountID: 99999, Email: "x@alias.example"}, api.CodeNotFound},
	} {
		if _, err := ids.Create(ctx, &c.p); code(err) != c.want {
			t.Errorf("create %+v: %v, want %v", c.p, err, c.want)
		}
	}

	if err := ids.Delete(ctx, &api.IdentityDeleteParams{ID: def.ID}); code(err) != api.CodeConflict {
		t.Errorf("deleting the account's own address: %v", err)
	}
	if err := ids.Delete(ctx, &api.IdentityDeleteParams{ID: added.ID}); err != nil {
		t.Fatal(err)
	}
	if err := ids.Delete(ctx, &api.IdentityDeleteParams{ID: added.ID}); code(err) != api.CodeNotFound {
		t.Errorf("deleting it again: %v", err)
	}
	if inv := e.invitation(t, invite); inv.CanRespond {
		t.Error("an invitation to a removed address can still be answered")
	}
}
