package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/discover"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// Sign-in, discovery and the safety check (docs/design/accounts.md). The
// OAuth flow, discovery and verification arrive in M4 phase 3; until then
// they report unavailable.

var errM4 = api.Unavailable("this arrives later in M4")

func (a accounts) Discover(ctx context.Context, p *api.AccountDiscoverParams) (*api.Discovery, error) {
	r, err := discover.Discover(ctx, strings.TrimSpace(p.Email), a.Discovery)
	if err != nil {
		return nil, api.InvalidParams("%v", err)
	}
	out := &api.Discovery{Kind: r.Kind, Source: r.Source, Auth: r.Auth}
	conv := func(s *discover.Server) *api.ServerConfig {
		if s == nil {
			return nil
		}
		return &api.ServerConfig{Host: s.Host, Port: int64(s.Port), TLS: s.TLS, Username: s.Username}
	}
	out.IMAP, out.SMTP = conv(r.IMAP), conv(r.SMTP)
	return out, nil
}

func (a accounts) Authorize(ctx context.Context, p *api.AccountAuthorizeParams) (*api.AuthorizeResult, error) {
	if a.OAuth == nil {
		return nil, api.Unavailable("OAuth sign-in is not running")
	}
	u, err := a.OAuth.Authorize(ctx, p.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, api.NotFound("account %d does not exist", p.ID)
	case errors.Is(err, oauth.ErrNotOAuth):
		return nil, api.InvalidParams("account %d does not sign in with OAuth", p.ID)
	case errors.Is(err, oauth.ErrNoClient):
		return nil, api.Unavailable("no Google client is set: add one in Settings or with mailctl oauth set-client")
	case err != nil:
		return nil, err
	}
	return &api.AuthorizeResult{URL: u}, nil
}

func (accounts) Verify(context.Context, *api.AccountVerifyParams) (*api.VerifyReport, error) {
	return nil, errM4
}

// Oauth implements the oauth domain.
func (e *Engine) Oauth() api.OauthService { return oauthClients{e.d} }

type oauthClients struct{ Deps }

func (o oauthClients) SetClient(ctx context.Context, p *api.OauthSetClientParams) (*api.OAuthClient, error) {
	if !p.Provider.Valid() {
		return nil, api.InvalidParams("unknown provider %q", p.Provider)
	}
	id := strings.TrimSpace(p.ClientID)
	if id == "" || strings.ContainsAny(id, " \t\r\n") {
		return nil, api.InvalidParams("clientId %q is not a client ID", p.ClientID)
	}
	if p.ClientSecret != nil {
		if err := o.Secrets.Set(ctx, secrets.OAuthClientSecret(string(p.Provider)), *p.ClientSecret); err != nil {
			return nil, err
		}
	}
	if err := o.DB.Tx(ctx, func(tx *store.Tx) error { return tx.SetOAuthClientID(ctx, string(p.Provider), id) }); err != nil {
		return nil, err
	}
	return o.client(ctx, p.Provider)
}

func (o oauthClients) GetClient(ctx context.Context, p *api.OauthGetClientParams) (*api.OAuthClient, error) {
	if !p.Provider.Valid() {
		return nil, api.InvalidParams("unknown provider %q", p.Provider)
	}
	return o.client(ctx, p.Provider)
}

func (o oauthClients) client(ctx context.Context, provider api.OAuthProvider) (*api.OAuthClient, error) {
	id, err := o.DB.OAuthClientID(ctx, string(provider))
	if errors.Is(err, store.ErrNotFound) {
		return nil, api.NotFound("no %s client is set", provider)
	}
	if err != nil {
		return nil, fmt.Errorf("oauth client: %w", err)
	}
	_, err = o.Secrets.Get(ctx, secrets.OAuthClientSecret(string(provider)))
	return &api.OAuthClient{Provider: provider, ClientID: id, HasSecret: err == nil}, nil
}
