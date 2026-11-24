package main

import (
	"context"
	"fmt"
	"net/mail"
	"strconv"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostui"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func button(label string, press func()) ui.Button { return ui.Button{Text: label, OnPress: press} }
func field(label string, input core.Node) core.Node {
	return workspaceui.Stack(core.Text{Content: label, Style: ui.TypeCaption}, input)
}
func stringField(label string, value *string, masked bool) core.Node {
	return field(label, ui.Input{Label: label, DefaultValue: *value, Masked: masked, OnChange: func(s string) { *value = s }})
}
func choice[T comparable](label string, current T, options []ui.Option[T], change func(T), cx *core.Scope) core.Node {
	value := core.NewSignal(cx, current)
	return field(label, ui.Select[T]{Label: label, Options: options, Value: value.Get, OnChange: func(v T) { value.Set(v); change(v) }})
}
func named[T ~string](values ...T) []ui.Option[T] {
	var options []ui.Option[T]
	for _, v := range values {
		options = append(options, ui.Option[T]{Value: v, Label: string(v)})
	}
	return options
}
func (w *workspace) accountsPanel(cx *core.Scope) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		w.state.Get()
		nodes := []core.Node{button("Add account", func() { w.startAccount() })}
		for _, a := range w.store.State().Accounts {
			nodes = append(nodes, workspaceui.Card{Children: []core.Node{core.Text{Content: a.DisplayName, Style: ui.TypeTitle}, core.Text{Content: a.Email + " · " + string(a.Kind)}, workspaceui.Row(button("Manage "+a.Email, func() { w.accountMenu(a) }))}})
		}
		return workspaceui.Page(nodes...)
	})
}
func (w *workspace) startAccount() {
	w.show(func(*core.Scope) core.Node {
		var email string
		invalid := func() bool { a, err := mail.ParseAddress(email); return err != nil || a.Address != email }
		manual := func() api.Account {
			return api.Account{Email: email, Kind: api.AccountKindIMAP, Auth: api.AuthKindPassword, Notify: true, IMAP: api.ServerConfig{Port: 993, TLS: api.TLSModeTLS, Username: email}, SMTP: api.ServerConfig{Port: 465, TLS: api.TLSModeTLS, Username: email}}
		}
		return w.dialog("Add mail account", []core.Node{stringField("Email address", &email, false)}, button("Manual setup", func() {
			if invalid() {
				w.problem.Set("Enter an email address.")
				return
			}
			w.editAccount(manual())
		}), button("Find settings", func() {
			if invalid() {
				w.problem.Set("Enter an email address.")
				return
			}
			a := manual()
			perform(w, func(ctx context.Context, c *api.Client) (*api.Discovery, error) {
				return c.Account().Discover(ctx, &api.AccountDiscoverParams{Email: a.Email})
			}, func(d *api.Discovery, err error) {
				if err != nil {
					w.problem.Set(err.Error())
					return
				}
				a.Kind = d.Kind
				if len(d.Auth) > 0 {
					a.Auth = d.Auth[0]
				}
				if d.IMAP != nil {
					a.IMAP = *d.IMAP
				}
				if d.SMTP != nil {
					a.SMTP = *d.SMTP
				}
				w.editAccount(a)
			})
		}))
	})
}

func serverFields(cx *core.Scope, label string, s *api.ServerConfig, port *string) core.Node {
	return workspaceui.Card{Children: []core.Node{core.Text{Content: label, Style: ui.TypeTitle}, stringField(label+" host", &s.Host, false), workspaceui.Row(stringField(label+" port", port, false), choice(label+" security", s.TLS, named(api.TLSModeTLS, api.TLSModeStartTLS, api.TLSModeInsecure), func(t api.TLSMode) { s.TLS = t }, cx)), stringField(label+" username", &s.Username, false)}}
}
func (w *workspace) editAccount(original api.Account) {
	w.show(func(cx *core.Scope) core.Node {
		a := original
		ip, sp := strconv.FormatInt(a.IMAP.Port, 10), strconv.FormatInt(a.SMTP.Port, 10)
		days := strconv.FormatInt(a.SyncDays, 10)
		nodes := []core.Node{core.Text{Content: a.Email, Style: ui.TypeTitle}, stringField("Full name", &a.DisplayName, false)}
		if a.ID == 0 {
			nodes = append(nodes, workspaceui.Row(choice("Provider", a.Kind, named(api.AccountKindIMAP, api.AccountKindGmail, api.AccountKindMicrosoft, api.AccountKindICloud), func(v api.AccountKind) { a.Kind = v }, cx), choice("Sign-in method", a.Auth, named(api.AuthKindPassword, api.AuthKindOAuth2), func(v api.AuthKind) { a.Auth = v }, cx)))
		}
		readOnly, notify := core.NewSignal(cx, a.ReadOnly), core.NewSignal(cx, a.Notify)
		nodes = append(nodes, serverFields(cx, "IMAP", &a.IMAP, &ip), serverFields(cx, "SMTP", &a.SMTP, &sp), ui.Checkbox{Text: "Read only", Checked: readOnly.Get, OnChange: readOnly.Set}, ui.Checkbox{Text: "Notify for new mail", Checked: notify.Get, OnChange: notify.Set}, stringField("Days of mail to sync (0 for all)", &days, false))
		if a.ID > 0 {
			periods := []ui.Option[int64]{{Label: "Never", Value: 0}, {Label: "After one day", Value: 1}, {Label: "After one week", Value: 7}, {Label: "After 30 days", Value: 30}}
			nodes = append(nodes, choice("Erase Trash", settingValue(a.EraseTrashDays, 0), periods, func(v int64) { a.EraseTrashDays = &v }, cx),
				choice("Erase Junk", settingValue(a.EraseJunkDays, 0), periods, func(v int64) { a.EraseJunkDays = &v }, cx))
		}
		parse := func(raw string, max int64) (int64, error) {
			v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			if err != nil || v < 0 || v > max {
				return 0, fmt.Errorf("Enter a number from 0 to %d", max)
			}
			return v, nil
		}
		return w.dialog("Account settings", []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(440)}, Children: nodes}}, button("Cancel", w.closePanel), ui.Button{Text: "Save account", Disabled: w.acting.Get, OnPress: func() {
			var err error
			a.IMAP.Port, err = parse(ip, 65535)
			if err == nil {
				a.SMTP.Port, err = parse(sp, 65535)
			}
			if err == nil {
				a.SyncDays, err = parse(days, 36500)
			}
			if err != nil {
				w.problem.Set(err.Error())
				return
			}
			a.ReadOnly, a.Notify = readOnly.GetUntracked(), notify.GetUntracked()
			saved := a
			w.acting.Set(true)
			perform(w, func(ctx context.Context, c *api.Client) (*api.Account, error) {
				if saved.ID == 0 {
					return c.Account().Create(ctx, &api.AccountCreateParams{Email: saved.Email, DisplayName: saved.DisplayName, Kind: saved.Kind, Auth: saved.Auth, IMAP: &saved.IMAP, SMTP: &saved.SMTP, SyncDays: &saved.SyncDays, ReadOnly: &saved.ReadOnly, Notify: &saved.Notify})
				}
				return c.Account().Update(ctx, &api.AccountUpdateParams{ID: saved.ID, EraseTrashDays: saved.EraseTrashDays, EraseJunkDays: saved.EraseJunkDays, DisplayName: &saved.DisplayName, IMAP: &saved.IMAP, SMTP: &saved.SMTP, SyncDays: &saved.SyncDays, ReadOnly: &saved.ReadOnly, Notify: &saved.Notify})
			}, func(result *api.Account, err error) {
				w.acting.Set(false)
				if err != nil {
					w.problem.Set(err.Error())
					return
				}
				w.accountMenu(*result)
			})
		}})
	})
}
func (w *workspace) accountMenu(a api.Account) {
	w.show(func(*core.Scope) core.Node {
		auth := button("Set password", func() {
			w.show(func(*core.Scope) core.Node {
				var password string
				return w.dialog("Set password", []core.Node{stringField("Password", &password, true)}, button("Cancel", w.closePanel), button("Store password", func() {
					if password == "" {
						return
					}
					p := api.AccountSetPasswordParams{ID: a.ID, Password: password}
					w.change(func(ctx context.Context, c *api.Client) error { return c.Account().SetPassword(ctx, &p) }, w.closePanel)
				}))
			})
		})
		if a.Auth == api.AuthKindOAuth2 {
			auth = button("Sign in with provider", func() { w.authorizeAccount(a) })
		}
		return w.dialog(a.Email, []core.Node{button("Edit servers and preferences", func() { w.editAccount(a) }), auth, button("Services and collections", func() { w.services(a) }), button("Verify account", func() {
			perform(w, func(ctx context.Context, c *api.Client) (*api.VerifyReport, error) {
				return c.Account().Verify(ctx, &api.AccountVerifyParams{ID: a.ID})
			}, func(report *api.VerifyReport, err error) {
				if err != nil {
					w.problem.Set(err.Error())
					return
				}
				text := "Differences found"
				if report.Ok {
					text = "Account matches the server"
				}
				w.show(func(*core.Scope) core.Node {
					rows := []core.Node{core.Text{Content: text}}
					for _, m := range report.Mailboxes {
						rows = append(rows, core.Text{Content: fmt.Sprintf("%s · %d local · %d on server", m.Path, m.Local, m.Server)})
					}
					return w.dialog("Account verification", []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(420)}, Children: rows}})
				})
			})
		}), button("Remove account", func() {
			w.confirm("Remove account?", "Remove "+a.Email+" and its locally stored mail and settings?", "Remove", func() {
				w.change(func(ctx context.Context, c *api.Client) error {
					return c.Account().Delete(ctx, &api.AccountDeleteParams{ID: a.ID})
				}, nil)
			})
		})})
	})
}
func (w *workspace) authorizeAccount(a api.Account) {
	perform(w, func(ctx context.Context, c *api.Client) (*api.AuthorizeResult, error) {
		return c.Account().Authorize(ctx, &api.AccountAuthorizeParams{ID: a.ID})
	}, func(result *api.AuthorizeResult, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		url, err := (frostui.Opener{Schemes: []string{"https"}}).CheckURL(result.URL)
		if err != nil {
			w.problem.Set("The mail server returned an invalid provider sign-in URL.")
			return
		}
		w.show(func(*core.Scope) core.Node {
			var callback string
			return w.dialog("Sign in to "+a.Email, []core.Node{button("Open provider sign-in", func() { w.cx.OpenURL(url, nil) }), core.Text{Content: "If your browser cannot reach the return address, paste that address here to finish sign-in."}, stringField("Return address", &callback, true)}, button("Close", w.closePanel), button("Finish sign-in", func() {
				value := callback
				w.change(func(ctx context.Context, c *api.Client) error {
					_, err := c.Account().FinishSignIn(ctx, &api.AccountFinishSignInParams{URL: value})
					return err
				}, w.closePanel)
			}))
		})
	})
}
func (w *workspace) services(a api.Account) {
	type result struct {
		s  []api.ServiceSettings
		cs []api.Collection
	}
	perform(w, func(ctx context.Context, c *api.Client) (result, error) {
		s, err := c.Account().Services(ctx, &api.AccountServicesParams{ID: a.ID})
		if err != nil {
			return result{}, err
		}
		cs, err := c.Account().Collections(ctx, &api.AccountCollectionsParams{AccountID: &a.ID})
		return result{s, cs}, err
	}, func(r result, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.show(func(*core.Scope) core.Node {
			var nodes []core.Node
			reload := func() { w.services(a) }
			for _, s := range r.s {
				address := s.URL
				save := func(enabled bool) {
					url := changed(s.URL, address)
					w.change(func(ctx context.Context, c *api.Client) error {
						_, err := c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: a.ID, Service: s.Service, Enabled: enabled, URL: url})
						return err
					}, reload)
				}
				nodes = append(nodes, ui.Checkbox{Text: string(s.Service), Checked: func() bool { return s.Enabled }, Disabled: func() bool { return !s.Available || w.acting.Get() }, OnChange: save})
				if s.Available {
					nodes = append(nodes, stringField(string(s.Service)+" server", &address, false), button("Save "+string(s.Service)+" server", func() { save(s.Enabled) }))
				}
				if s.Error != nil {
					nodes = append(nodes, core.Text{Content: *s.Error})
				}
			}
			for _, col := range r.cs {
				nodes = append(nodes, workspaceui.Row(ui.Checkbox{Text: col.Name, Checked: func() bool { return col.Enabled }, OnChange: func(enabled bool) {
					w.change(func(ctx context.Context, c *api.Client) error {
						_, err := c.Account().SetCollection(ctx, &api.AccountSetCollectionParams{ID: col.ID, Enabled: &enabled})
						return err
					}, reload)
				}}, ui.Button{Text: "Use as default", Disabled: func() bool { return col.IsDefault }, OnPress: func() {
					v := true
					w.change(func(ctx context.Context, c *api.Client) error {
						_, err := c.Account().SetCollection(ctx, &api.AccountSetCollectionParams{ID: col.ID, IsDefault: &v})
						return err
					}, reload)
				}}))
			}
			return w.dialog("Services and collections", []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(440)}, Children: nodes}})
		})
	})
}
