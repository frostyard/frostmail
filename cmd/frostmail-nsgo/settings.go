package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/nsgo-dev/nsgo/core"
)

func (w *workspace) oauthClient(provider api.OAuthProvider) {
	perform(w, func(ctx context.Context, c *api.Client) (*api.OAuthClient, error) {
		return c.Oauth().GetClient(ctx, &api.OauthGetClientParams{Provider: provider})
	}, func(info *api.OAuthClient, err error) {
		if errors.Is(err, api.ErrNotFound) {
			info = &api.OAuthClient{Provider: provider}
			err = nil
		}
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.show(func(*core.Scope) core.Node {
			id, secret := info.ClientID, ""
			return w.dialog("OAuth client · "+string(provider), []core.Node{stringField("Client ID", &id, false), stringField("New client secret", &secret, true)}, button("Cancel", w.closePanel), primary("Save", func() {
				p := api.OauthSetClientParams{Provider: provider, ClientID: id, ClientSecret: changed("", secret)}
				w.change(func(ctx context.Context, c *api.Client) error { _, err := c.Oauth().SetClient(ctx, &p); return err }, w.closePanel)
			}))
		})
	})
}
func settingValue[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

// addIdentity asks which account a new sender identity is of, then for it.
func (w *workspace) addIdentity(load func()) {
	w.show(func(*core.Scope) core.Node {
		var rows []core.Node
		for _, a := range w.store.State().Accounts {
			rows = append(rows, action(a.Email, func() {
				w.form("Add sender identity", []editField{{"Name", a.DisplayName, false}, {"Email address", "", false}}, func(v []string) error {
					w.change(func(ctx context.Context, c *api.Client) error {
						_, err := c.Identity().Create(ctx, &api.IdentityCreateParams{AccountID: a.ID, Name: &v[0], Email: v[1]})
						return err
					}, func() { w.closePanel(); load() })
					return nil
				})
			}))
		}
		return w.dialog("Choose account", []core.Node{actionList(rows...)})
	})
}

func (w *workspace) editIdentity(i api.Identity, load func()) {
	w.form("Sender identity", []editField{{"Name", i.Name, false}, {"Reply to", i.ReplyTo, false}, {"Signature HTML", i.SignatureHTML, true}}, func(v []string) error {
		w.change(func(ctx context.Context, c *api.Client) error {
			_, err := c.Identity().Update(ctx, &api.IdentityUpdateParams{ID: i.ID, Name: changed(i.Name, v[0]), ReplyTo: changed(i.ReplyTo, v[1]), SignatureHTML: changed(i.SignatureHTML, v[2])})
			return err
		}, func() { w.closePanel(); load() })
		return nil
	})
}

func (w *workspace) deleteIdentity(i api.Identity, load func()) {
	w.destroy("Delete sender identity?", i.Email, "Delete", func() {
		w.change(func(ctx context.Context, c *api.Client) error {
			return c.Identity().Delete(ctx, &api.IdentityDeleteParams{ID: i.ID})
		}, load)
	})
}

func (w *workspace) editSnippet(s api.Snippet, load func()) {
	w.form("Message snippet", []editField{{"Snippet name", s.Name, false}, {"Snippet HTML", s.HTML, true}}, func(v []string) error {
		if strings.TrimSpace(v[0]) == "" {
			return fmt.Errorf("Enter a snippet name")
		}
		w.change(func(ctx context.Context, c *api.Client) error {
			if s.ID == 0 {
				_, err := c.Snippet().Create(ctx, &api.SnippetCreateParams{Name: v[0], HTML: v[1]})
				return err
			}
			_, err := c.Snippet().Update(ctx, &api.SnippetUpdateParams{ID: s.ID, Name: changed(s.Name, v[0]), HTML: changed(s.HTML, v[1])})
			return err
		}, func() { w.closePanel(); load() })
		return nil
	})
}

func (w *workspace) deleteSnippet(s api.Snippet, load func()) {
	w.destroy("Delete snippet?", s.Name, "Delete", func() {
		w.change(func(ctx context.Context, c *api.Client) error {
			return c.Snippet().Delete(ctx, &api.SnippetDeleteParams{ID: s.ID})
		}, load)
	})
}
