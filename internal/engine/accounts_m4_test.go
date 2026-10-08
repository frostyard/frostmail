package engine_test

import (
	"errors"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func code(err error) api.ErrorCode {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

func TestProviderAccountsFillTheirServers(t *testing.T) {
	c := rpctest.Start(t).Dial(t)
	ctx := t.Context()
	g, err := c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindGmail, Email: "ann@gmail.com", DisplayName: "Ann", Auth: api.AuthKindOAuth2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if g.IMAP.Host != "imap.gmail.com" || g.IMAP.Username != "ann@gmail.com" || g.SMTP.Port != 465 || g.SMTP.TLS != api.TLSModeTLS {
		t.Errorf("gmail servers = %+v / %+v", g.IMAP, g.SMTP)
	}
	if !g.Notify || g.ReadOnly || g.SignedIn {
		t.Errorf("gmail defaults: notify %v, readOnly %v, signedIn %v", g.Notify, g.ReadOnly, g.SignedIn)
	}
	i, err := c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindICloud, Email: "bob@icloud.com", Auth: api.AuthKindPassword, ReadOnly: ptr(true), Notify: ptr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if i.IMAP.Host != "imap.mail.me.com" || i.SMTP.TLS != api.TLSModeStartTLS || !i.ReadOnly || i.Notify {
		t.Errorf("icloud = %+v", i)
	}
	bad := map[string]*api.AccountCreateParams{
		"generic without servers": {Kind: api.AccountKindIMAP, Email: "c@x.test", Auth: api.AuthKindPassword},
		"icloud with oauth":       {Kind: api.AccountKindICloud, Email: "d@icloud.com", Auth: api.AuthKindOAuth2},
		"generic with oauth": {
			Kind: api.AccountKindIMAP, Email: "e@x.test", Auth: api.AuthKindOAuth2,
			IMAP: &api.ServerConfig{Host: "imap.x.test", Port: 993, TLS: api.TLSModeTLS, Username: "e"},
			SMTP: &api.ServerConfig{Host: "smtp.x.test", Port: 465, TLS: api.TLSModeTLS, Username: "e"},
		},
	}
	for name, p := range bad {
		if _, err := c.Account().Create(ctx, p); code(err) != api.CodeInvalidParams {
			t.Errorf("%s: err = %v, want invalidParams", name, err)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestReadOnlyNotifyAndSignedIn(t *testing.T) {
	c := rpctest.Start(t).Dial(t)
	ctx := t.Context()
	a, err := c.Account().Create(ctx, createParams("ro@mailtest.test"))
	if err != nil {
		t.Fatal(err)
	}
	if a.SignedIn {
		t.Error("signed in before a password was set")
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	up, err := c.Account().Update(ctx, &api.AccountUpdateParams{ID: a.ID, ReadOnly: ptr(true), Notify: ptr(false)})
	if err != nil || !up.ReadOnly || up.Notify || !up.SignedIn {
		t.Fatalf("update = %+v, %v", up, err)
	}
	got, err := c.Account().Get(ctx, &api.AccountGetParams{ID: a.ID})
	if err != nil || !got.ReadOnly || got.Notify || !got.SignedIn {
		t.Fatalf("get = %+v, %v", got, err)
	}
}

func TestOAuthClients(t *testing.T) {
	c := rpctest.Start(t).Dial(t)
	ctx := t.Context()
	if _, err := c.Oauth().GetClient(ctx, &api.OauthGetClientParams{Provider: api.OAuthProviderGoogle}); code(err) != api.CodeNotFound {
		t.Fatalf("get before set = %v", err)
	}
	cl, err := c.Oauth().SetClient(ctx, &api.OauthSetClientParams{Provider: api.OAuthProviderGoogle, ClientID: " id-1.apps.googleusercontent.com ", ClientSecret: ptr("s3cret")})
	if err != nil || cl.ClientID != "id-1.apps.googleusercontent.com" || !cl.HasSecret {
		t.Fatalf("set = %+v, %v", cl, err)
	}
	cl, err = c.Oauth().SetClient(ctx, &api.OauthSetClientParams{Provider: api.OAuthProviderGoogle, ClientID: "id-2"})
	if err != nil || cl.ClientID != "id-2" || !cl.HasSecret {
		t.Fatalf("replace without a secret = %+v, %v", cl, err)
	}
	for _, id := range []string{"", "two words"} {
		if _, err := c.Oauth().SetClient(ctx, &api.OauthSetClientParams{Provider: api.OAuthProviderGoogle, ClientID: id}); code(err) != api.CodeInvalidParams {
			t.Errorf("set %q = %v", id, err)
		}
	}
}

func TestDiscoverOverTheAPI(t *testing.T) {
	c := rpctest.Start(t).Dial(t)
	d, err := c.Account().Discover(t.Context(), &api.AccountDiscoverParams{Email: " bob@icloud.com "})
	if err != nil || d.Kind != api.AccountKindICloud || d.Source != api.DiscoverySourceProfile ||
		d.IMAP == nil || d.IMAP.Host != "imap.mail.me.com" || d.SMTP.Username != "bob@icloud.com" {
		t.Fatalf("discover = %+v, %v", d, err)
	}
	if _, err := c.Account().Discover(t.Context(), &api.AccountDiscoverParams{Email: "nobody"}); code(err) != api.CodeInvalidParams {
		t.Errorf("no @: %v", err)
	}
}
