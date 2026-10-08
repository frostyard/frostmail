package engine_test

// CONTRACT TEST for task card T-0002 (docs/tasks): the account API end to
// end, over the socket, through the engine, into the store. Do not edit.

import (
	"errors"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func createParams(email string) *api.AccountCreateParams {
	return &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: email, DisplayName: "Test", Auth: api.AuthKindPassword,
		IMAP: &api.ServerConfig{Host: "imap.mailtest.test", Port: 993, TLS: api.TLSModeTLS, Username: email},
		SMTP: &api.ServerConfig{Host: "smtp.mailtest.test", Port: 587, TLS: api.TLSModeStartTLS, Username: email},
	}
}

func TestAccountAPI(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	if _, err := c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}

	a, err := c.Account().Create(ctx, createParams("test1@mailtest.test"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if a.ID == 0 || a.Email != "test1@mailtest.test" || a.IMAP.Port != 993 || a.CreatedAt.IsZero() {
		t.Fatalf("created = %+v", a)
	}
	select {
	case ev := <-c.Notifications():
		e, err := api.DecodeEvent(ev.Event, ev.Data)
		if err != nil || e != (api.AccountChanged{ID: a.ID}) || ev.Seq == 0 {
			t.Fatalf("event = %+v (%v)", ev, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no account.changed event")
	}

	list, err := c.Account().List(ctx, nil)
	if err != nil || len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}
	name := "Renamed"
	u, err := c.Account().Update(ctx, &api.AccountUpdateParams{ID: a.ID, DisplayName: &name})
	if err != nil || u.DisplayName != "Renamed" || u.SMTP != a.SMTP {
		t.Fatalf("update = %+v, %v", u, err)
	}
	if _, err := c.Account().Create(ctx, createParams("TEST1@mailtest.test")); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("duplicate create = %v, want conflict", err)
	}
	bad := createParams("not an address")
	if _, err := c.Account().Create(ctx, bad); !errors.Is(err, api.ErrInvalidParams) {
		t.Fatalf("invalid email = %v, want invalidParams", err)
	}
	if err := c.Account().Delete(ctx, &api.AccountDeleteParams{ID: a.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := c.Account().Get(ctx, &api.AccountGetParams{ID: a.ID}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("get after delete = %v, want notFound", err)
	}
}
