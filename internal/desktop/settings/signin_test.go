package settings_test

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/settings"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestSignInPaneStoresTheClient(t *testing.T) {
	f := newFixture(t, setup{})
	f.show(settings.PaneSignIn)
	f.sees("No client stored")
	if !f.disabled("Save") {
		t.Error("Save works with no client ID")
	}
	f.fill("Client ID:", " cid ")
	f.fill("Client Secret:", "test-secret")
	if a := f.control(ui.RoleTextField, "Client Secret:"); !a.Password {
		t.Error("the secret shows")
	}
	f.click("Save")
	f.sees("Client and secret stored")
	got, err := f.c.Oauth().GetClient(t.Context(), &api.OauthGetClientParams{Provider: api.OAuthProviderGoogle})
	if err != nil || got.ClientID != "cid" || !got.HasSecret {
		t.Fatalf("client = %+v, %v", got, err)
	}
	if f.value("Client Secret:") != "" {
		t.Error("the secret stays in the field")
	}
	if a := f.control(ui.RoleTextField, "Client Secret:"); a.Placeholder != "Stored" {
		t.Errorf("the placeholder is %q", a.Placeholder)
	}
	if !f.disabled("Save") {
		t.Error("Save works with nothing changed")
	}
	f.fill("Client ID:", "cid2")
	f.click("Save")
	f.until("the new ID, keeping the secret", func() bool {
		got, err := f.c.Oauth().GetClient(t.Context(), &api.OauthGetClientParams{Provider: api.OAuthProviderGoogle})
		return err == nil && got.ClientID == "cid2" && got.HasSecret
	})
}

func TestSignInPaneSaysWhenThereIsNoSecret(t *testing.T) {
	f := newFixture(t, setup{})
	if _, err := f.c.Oauth().SetClient(t.Context(), &api.OauthSetClientParams{Provider: api.OAuthProviderGoogle, ClientID: "cid"}); err != nil {
		t.Fatal(err)
	}
	f.show(settings.PaneSignIn)
	f.sees("Client stored without a secret")
	if f.value("Client ID:") != "cid" {
		t.Errorf("Client ID = %q", f.value("Client ID:"))
	}
}

// heldOauth holds oauth.setClient and oauth.getClient back while their gates
// are set: entered says one has arrived.
type heldOauth struct {
	api.OauthService
	set, get chan struct{}
	entered  chan struct{}
}

func (s heldOauth) SetClient(ctx context.Context, p *api.OauthSetClientParams) (*api.OAuthClient, error) {
	if s.set != nil {
		s.entered <- struct{}{}
		<-s.set
	}
	return s.OauthService.SetClient(ctx, p)
}

func (s heldOauth) GetClient(ctx context.Context, p *api.OauthGetClientParams) (*api.OAuthClient, error) {
	if s.get != nil {
		s.entered <- struct{}{}
		<-s.get
	}
	return s.OauthService.GetClient(ctx, p)
}

// An answer that comes while the user types must not take the field.
func TestALateSaveKeepsTheClientIDBeingTyped(t *testing.T) {
	gate, entered := make(chan struct{}), make(chan struct{}, 1)
	f := newFixture(t, setup{server: rpctest.Options{Services: func(s *api.Services) {
		s.Oauth = heldOauth{OauthService: s.Oauth, set: gate, entered: entered}
	}}})
	f.show(settings.PaneSignIn)
	f.sees("No client stored")
	f.fill("Client ID:", "first")
	f.click("Save")
	<-entered
	f.fill("Client ID:", "second")
	close(gate)
	f.sees("Client stored without a secret")
	if got := f.value("Client ID:"); got != "second" {
		t.Errorf("the field holds %q", got)
	}
	if f.disabled("Save") {
		t.Error("Save is off with an ID not stored")
	}
}

func TestALateReadKeepsTheClientIDBeingTyped(t *testing.T) {
	gate, entered := make(chan struct{}), make(chan struct{}, 1)
	f := newFixture(t, setup{
		first: func(f *fixture) {
			if _, err := f.c.Oauth().SetClient(t.Context(), &api.OauthSetClientParams{Provider: api.OAuthProviderGoogle, ClientID: "stored"}); err != nil {
				t.Fatal(err)
			}
		},
		server: rpctest.Options{Services: func(s *api.Services) {
			s.Oauth = heldOauth{OauthService: s.Oauth, get: gate, entered: entered}
		}},
	})
	f.show(settings.PaneSignIn)
	f.until("the read", func() bool {
		select {
		case <-entered:
			return true
		default:
			return false
		}
	})
	f.fill("Client ID:", "typed")
	close(gate)
	f.sees("Client stored without a secret")
	if got := f.value("Client ID:"); got != "typed" {
		t.Errorf("the field holds %q", got)
	}
}
