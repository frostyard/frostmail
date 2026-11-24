package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workspace) settings(cx *core.Scope) core.Node {
	tab := core.NewSignal(cx, "Accounts")
	panes := named("Accounts", "General", "Identities", "Snippets", "Filters", "Mailboxes", "Sign-in")
	if desktopServer {
		panes = append(panes, ui.Option[string]{Value: "Server", Label: "Server"})
	}
	return core.View{Style: ui.FillRest, Children: []core.Node{
		ui.Tabs[string]{Label: "Settings panes", Options: panes, Value: tab.Get, OnChange: tab.Set},
		core.Dynamic(func(cx *core.Scope) core.Node {
			switch tab.Get() {
			case "Server":
				return w.serverPanel(cx)
			case "Filters":
				return w.filters(cx)
			case "Mailboxes":
				return w.mailboxes(cx)
			case "General":
				return w.general(cx)
			case "Identities":
				return w.identities(cx)
			case "Snippets":
				return w.snippets(cx)
			case "Sign-in":
				return workspaceui.Page(button("Google sign-in client", func() { w.oauthClient(api.OAuthProviderGoogle) }), button("Microsoft sign-in client", func() { w.oauthClient(api.OAuthProviderMicrosoft) }))
			default:
				return w.accountsPanel(cx)
			}
		})}}
}
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
			return w.dialog("OAuth client · "+string(provider), []core.Node{stringField("Client ID", &id, false), stringField("New client secret", &secret, true)}, button("Cancel", w.closePanel), button("Save", func() {
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
func (w *workspace) general(cx *core.Scope) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		w.state.Get()
		st := w.store.State()
		s := st.Settings
		if s == nil {
			return core.Text{Content: "Loading preferences…"}
		}
		save := func(p api.SettingsSetParams) {
			w.change(func(ctx context.Context, c *api.Client) error { _, err := c.Settings().Set(ctx, &p); return err }, nil)
		}
		scope := string(s.NotifyScope)
		scopeOpts := named("inbox", "vips", "contacts", "all")
		for _, box := range st.Smart {
			key := fmt.Sprintf("smart:%d", box.ID)
			scopeOpts = append(scopeOpts, ui.Option[string]{Value: key, Label: box.Name})
			if s.NotifySmartID != nil && *s.NotifySmartID == box.ID && s.NotifyScope == api.NotifyScopeSmart {
				scope = key
			}
		}
		var lines, delay []ui.Option[int64]
		for i := int64(0); i <= 5; i++ {
			lines = append(lines, ui.Option[int64]{Value: i, Label: strconv.FormatInt(i, 10)})
		}
		for _, i := range []int64{0, 10, 20, 30} {
			delay = append(delay, ui.Option[int64]{Value: i, Label: fmt.Sprintf("%d seconds", i)})
		}
		nodes := []core.Node{choice("New message notifications", scope, scopeOpts, func(v string) {
			scope := api.NotifyScope(v)
			p := api.SettingsSetParams{NotifyScope: &scope}
			if raw, ok := strings.CutPrefix(v, "smart:"); ok {
				id, err := strconv.ParseInt(raw, 10, 64)
				if err != nil {
					return
				}
				scope = api.NotifyScopeSmart
				p.NotifySmartID = &id
			}
			save(p)
		}, cx),
			choice("Undo send delay", s.UndoDelay, delay, func(v int64) { save(api.SettingsSetParams{UndoDelay: &v}) }, cx),
			choice("List preview lines", settingValue(s.PreviewLines, int64(2)), lines, func(v int64) { save(api.SettingsSetParams{PreviewLines: &v}) }, cx),
			choice("Blocked mail", settingValue(s.BlockAction, api.BlockActionMark), named(api.BlockActionMark, api.BlockActionTrash), func(v api.BlockAction) { save(api.SettingsSetParams{BlockAction: &v}) }, cx),
			button("Edit flag names", func() {
				var fields []editField
				for i, n := range []string{"Red", "Orange", "Yellow", "Green", "Blue", "Purple", "Gray"} {
					name := ""
					if i < len(s.FlagNames) {
						name = s.FlagNames[i]
					}
					fields = append(fields, editField{n, name, false})
				}
				w.form("Flag names", fields, func(v []string) error {
					p := api.SettingsSetParams{FlagNames: v}
					w.change(func(ctx context.Context, c *api.Client) error { _, err := c.Settings().Set(ctx, &p); return err }, w.closePanel)
					return nil
				})
			})}
		if desktopServer {
			sizes := []ui.Option[api.TextSize]{{Value: api.TextSizeSmaller, Label: "Smaller"}, {Value: api.TextSizeDefault, Label: "Default"}, {Value: api.TextSizeLarger, Label: "Larger"}, {Value: api.TextSizeLargest, Label: "Largest"}}
			nodes = append(nodes, choice("Message text size", settingValue(s.TextSize, api.TextSizeDefault), sizes, func(v api.TextSize) { save(api.SettingsSetParams{TextSize: &v}) }, cx))
		}
		nodes = append(nodes, w.notificationSettings(cx)...)
		if opt := w.traySetting(); opt != nil {
			nodes = append(nodes, opt)
		}
		for _, b := range st.Blocked {
			nodes = append(nodes, button("Unblock "+b.Address, func() {
				w.change(func(ctx context.Context, c *api.Client) error {
					return c.Block().Remove(ctx, &api.BlockRemoveParams{Addresses: []string{b.Address}})
				}, nil)
			}))
		}
		return workspaceui.Page(nodes...)
	})
}
func (w *workspace) identities(cx *core.Scope) core.Node {
	rows, load := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.AccountChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Identity, error) {
		return func(ctx context.Context, c *api.Client) ([]api.Identity, error) {
			return c.Identity().List(ctx, &api.IdentityListParams{})
		}
	})
	add := func() {
		w.show(func(*core.Scope) core.Node {
			var buttons []core.Node
			for _, a := range w.store.State().Accounts {
				buttons = append(buttons, button(a.Email, func() {
					w.form("Add sender identity", []editField{{"Name", a.DisplayName, false}, {"Email address", "", false}}, func(v []string) error {
						w.change(func(ctx context.Context, c *api.Client) error {
							_, err := c.Identity().Create(ctx, &api.IdentityCreateParams{AccountID: a.ID, Name: &v[0], Email: v[1]})
							return err
						}, func() { w.closePanel(); load() })
						return nil
					})
				}))
			}
			return w.dialog("Choose account", buttons)
		})
	}
	return core.View{Style: ui.FillRest, Children: []core.Node{button("Add sender identity", add), collectionList(rows, "Sender identities", func(i api.Identity) string { return i.Email }, func(i api.Identity) string { return i.Name }, func(i api.Identity) {
		w.show(func(*core.Scope) core.Node {
			nodes := []core.Node{button("Edit identity and signature", func() {
				w.form("Sender identity", []editField{{"Name", i.Name, false}, {"Reply to", i.ReplyTo, false}, {"Signature HTML", i.SignatureHTML, true}}, func(v []string) error {
					w.change(func(ctx context.Context, c *api.Client) error {
						_, err := c.Identity().Update(ctx, &api.IdentityUpdateParams{ID: i.ID, Name: changed(i.Name, v[0]), ReplyTo: changed(i.ReplyTo, v[1]), SignatureHTML: changed(i.SignatureHTML, v[2])})
						return err
					}, func() { w.closePanel(); load() })
					return nil
				})
			})}
			if !i.IsDefault {
				nodes = append(nodes, button("Delete identity", func() {
					w.confirm("Delete sender identity?", i.Email, "Delete", func() {
						w.change(func(ctx context.Context, c *api.Client) error {
							return c.Identity().Delete(ctx, &api.IdentityDeleteParams{ID: i.ID})
						}, load)
					})
				}))
			}
			return w.dialog(i.Email, nodes)
		})
	})}}
}
func (w *workspace) snippets(cx *core.Scope) core.Node {
	rows, load := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.SnippetChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Snippet, error) {
		return func(ctx context.Context, c *api.Client) ([]api.Snippet, error) {
			return c.Snippet().List(ctx, &api.SnippetListParams{})
		}
	})
	edit := func(s api.Snippet) {
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
	return core.View{Style: ui.FillRest, Children: []core.Node{button("Add snippet", func() { edit(api.Snippet{}) }), collectionList(rows, "Message snippets", func(s api.Snippet) string { return s.Name }, func(s api.Snippet) string { return s.UpdatedAt.Local().Format("Jan 2") }, func(s api.Snippet) {
		w.show(func(*core.Scope) core.Node {
			return w.dialog(s.Name, []core.Node{button("Edit snippet", func() { edit(s) }), button("Delete snippet", func() {
				w.confirm("Delete snippet?", s.Name, "Delete", func() {
					w.change(func(ctx context.Context, c *api.Client) error {
						return c.Snippet().Delete(ctx, &api.SnippetDeleteParams{ID: s.ID})
					}, load)
				})
			})})
		})
	})}}
}
