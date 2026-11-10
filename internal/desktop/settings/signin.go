package settings

import (
	"context"
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
)

// signInPane is the Sign-In pane: the Google OAuth client.
type signInPane struct {
	request
	client   *api.OAuthClient
	load     loader
	id       string
	synced   string // the stored ID the field last took
	secret   string
	secretNo int // a new number makes a new field, which forgets the secret
}

func (w *Window) loadClient() {
	p := w.auth
	if p == nil {
		return
	}
	p.load.run(w, func(ctx context.Context, c *api.Client) func() {
		client, err := c.Oauth().GetClient(ctx, &api.OauthGetClientParams{Provider: api.OAuthProviderGoogle})
		return func() {
			if err != nil {
				if !errors.Is(err, api.ErrNotFound) {
					p.err = message(err)
				}
				return
			}
			p.setClient(client)
		}
	})
}

func (p *signInPane) setClient(client *api.OAuthClient) {
	p.client = client
	if client.ClientID != p.synced {
		p.synced, p.id = client.ClientID, client.ClientID
	}
}

func (p *signInPane) status() string {
	switch {
	case p.client == nil:
		return "No client stored"
	case p.client.HasSecret:
		return "Client and secret stored"
	}
	return "Client stored without a secret"
}

func (w *Window) saveClient(p *signInPane) {
	id, secret := strings.TrimSpace(p.id), p.secret
	p.secret = ""
	p.secretNo++
	w.run(&p.request, func(ctx context.Context, c *api.Client) error {
		params := &api.OauthSetClientParams{Provider: api.OAuthProviderGoogle, ClientID: id}
		if secret != "" {
			params.ClientSecret = &secret
		}
		stored, err := c.Oauth().SetClient(ctx, params)
		if err != nil {
			return err
		}
		w.o.Post(func() { p.setClient(stored) })
		return nil
	}, nil)
}

func (w *Window) signInBody(c *ui.Context) {
	p := w.auth
	if p == nil {
		return
	}
	stored := ""
	if p.client != nil {
		stored = p.client.ClientID
	}
	trimmed := strings.TrimSpace(p.id)
	off := p.busy || trimmed == "" || trimmed == stored && p.secret == ""
	save := func() {
		if !off {
			w.saveClient(p)
		}
	}
	ui.Column(c).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Role(ui.RoleGroup).Label("Google client").Children(func() {
		frostui.Heading(c, "Google Client").Role(ui.RoleHeading).Level(2)
		note(c, "Gmail sign-in uses your own Google OAuth client until Frostmail has a verified one. Create a Desktop app client in Google Cloud and enter it here.")
		ui.Form(c, func() {
			ui.Field(c.Key("id"), "Client ID:", func() {
				if ui.TextInput(c, &p.id).Submitted() {
					save()
				}
			})
			ui.Field(c.Key("secret"), "Client Secret:", func() {
				in := ui.TextInput(c.Key(p.secretNo), &p.secret).Password()
				if p.client != nil && p.client.HasSecret {
					in.Placeholder("Stored")
				}
				if in.Submitted() {
					save()
				}
			})
		})
		note(c, p.status())
		alert(c, p.err)
		buttons(c, func() {
			if ui.PrimaryButton(c, "Save").Disabled(off).Clicked() {
				save()
			}
		})
	})
}
