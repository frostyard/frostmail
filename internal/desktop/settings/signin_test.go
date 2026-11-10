package settings_test

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/settings"
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
