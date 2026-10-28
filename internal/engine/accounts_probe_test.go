package engine_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/secrets"
)

// noKeyringStore is a secret store that keeps nothing, as maild's store
// does in the Flatpak with no Secret Service.
type noKeyringStore struct{}

var errKeyring = errors.New("no keyring here: run `flatpak override --user --env=FROSTMAIL_SECRETS=file org.frostyard.Frostmail`")

func (noKeyringStore) Get(context.Context, string) (string, error) { return "", errKeyring }
func (noKeyringStore) Set(context.Context, string, string) error {
	return errors.Join(secrets.ErrNoKeyring, errKeyring)
}
func (noKeyringStore) Delete(context.Context, string) error { return errKeyring }

// account.create probes the secret store before it inserts: a store that
// cannot keep a secret fails it with unavailable, names the cause, and
// adds nothing.
func TestAccountCreateFailsWhenTheSecretStoreCannotKeepASecret(t *testing.T) {
	srv := rpctest.StartWith(t, rpctest.Options{Secrets: noKeyringStore{}})
	c := srv.Dial(t)
	ctx := t.Context()
	_, err := c.Account().Create(ctx, createParams("probe@mailtest.test"))
	if !errors.Is(err, api.ErrUnavailable) {
		t.Fatalf("create = %v, want unavailable", err)
	}
	for _, want := range []string{"no account was added", "flatpak override --user --env=FROSTMAIL_SECRETS=file org.frostyard.Frostmail"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error lacks %q: %v", want, err)
		}
	}
	list, err := c.Account().List(ctx, nil)
	if err != nil || len(list) != 0 {
		t.Fatalf("list = %+v, %v; want no account", list, err)
	}
}

// With a store that works, create succeeds and the probe leaves no item.
func TestAccountCreateProbeLeavesNoSecret(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	if _, err := c.Account().Create(ctx, createParams("probe@mailtest.test")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := srv.Secrets.Get(ctx, secrets.ProbeKey); !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("the probe key is still in the store: %v", err)
	}
	if all, err := srv.Secrets.All(); err != nil || len(all) != 0 {
		t.Errorf("the store holds %d secrets, %v; want none", len(all), err)
	}
}
