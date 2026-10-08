package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// Sign-in, discovery and the safety check (docs/design/accounts.md). The
// OAuth flow, discovery and verification arrive in M4 phase 3; until then
// they report unavailable.

var errM4 = api.Unavailable("this arrives later in M4")

func (accounts) Discover(context.Context, *api.AccountDiscoverParams) (*api.Discovery, error) {
	return nil, errM4
}

func (accounts) Authorize(context.Context, *api.AccountAuthorizeParams) (*api.AuthorizeResult, error) {
	return nil, errM4
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
