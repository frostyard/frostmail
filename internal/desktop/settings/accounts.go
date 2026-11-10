package settings

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
)

// newAccount is selected while adding an account; 0 is nothing selected.
const newAccount int64 = -1

// signInWait is how long maild waits for a sign-in's return
// (internal/oauth's FlowTimeout).
const signInWait = 5 * time.Minute

// accountValue is what the account form edits.
type accountValue struct {
	Kind        api.AccountKind
	Email       string
	DisplayName string
	Auth        api.AuthKind
	IMAP, SMTP  api.ServerConfig
	ReadOnly    bool
	Notify      bool
	SyncDays    int64
	EraseTrash  int64
	EraseJunk   int64
	Password    string // a new password; "" keeps the stored one
	Services    []api.ServiceKind
}

// discoveryState is how far Find Settings got.
type discoveryState struct {
	finding bool
	found   bool
	source  api.DiscoverySource
	failed  string
}

// waitingSignIn is the sign-in this window started, while it waits: only
// it can finish from a pasted address (ADR-0043).
type waitingSignIn struct {
	id          int64
	wasSignedIn bool
	until       time.Time
}

// eraseNames are the names of an account's trash and junk mailboxes.
type eraseNames struct {
	trash, junk *string
}

// accountServices is the selected account's services.
type accountServices struct {
	id     int64
	list   []api.ServiceSettings
	load   loader
	busy   api.ServiceKind
	errors map[api.ServiceKind]string
	urls   map[api.ServiceKind]string
}

// accountsPane is the Accounts pane.
type accountsPane struct {
	request
	selected  int64
	shown     int64 // the account services and erase names are read for
	value     accountValue
	formNo    int // a new number makes a new form, which forgets its editing
	secretNo  int
	discovery discoveryState
	waiting   *waitingSignIn
	finish    string // the pasted address
	alerted   string // the error last scrolled into view
	erase     eraseNames
	eraseLoad *loader
	services  *accountServices
	listed    map[int64]bool
}

func newAccountsPane() *accountsPane {
	return &accountsPane{listed: map[int64]bool{}, value: emptyAccount()}
}

// emptyAccount is a new account's form: writable, since M5 (parity P-952).
func emptyAccount() accountValue {
	return accountValue{Kind: api.AccountKindIMAP, Auth: api.AuthKindPassword, Notify: true,
		IMAP: api.ServerConfig{Port: 993, TLS: api.TLSModeTLS}, SMTP: api.ServerConfig{Port: 465, TLS: api.TLSModeTLS}}
}

// eraseDays is a stored erase setting as the form offers it: anything but
// 1, 7 or 30 is Never (ADR-0051).
func eraseDays(days *int64) int64 {
	if days != nil && (*days == 1 || *days == 7 || *days == 30) {
		return *days
	}
	return 0
}

func valueOf(a api.Account) accountValue {
	return accountValue{Kind: a.Kind, Email: a.Email, DisplayName: a.DisplayName, Auth: a.Auth, IMAP: a.IMAP, SMTP: a.SMTP,
		ReadOnly: a.ReadOnly, Notify: a.Notify, SyncDays: a.SyncDays, EraseTrash: eraseDays(a.EraseTrashDays), EraseJunk: eraseDays(a.EraseJunkDays)}
}

// offeredServices are the services an account kind can turn on, in order.
func offeredServices(kind api.AccountKind) []api.ServiceKind {
	if kind == api.AccountKindICloud || kind == api.AccountKindMicrosoft {
		return []api.ServiceKind{api.ServiceKindContacts, api.ServiceKindCalendar}
	}
	return []api.ServiceKind{api.ServiceKindContacts, api.ServiceKindCalendar, api.ServiceKindTasks}
}

// defaultServices are the services a new account of a kind starts with
// checked.
func defaultServices(kind api.AccountKind) []api.ServiceKind {
	if kind == api.AccountKindIMAP {
		return nil
	}
	return offeredServices(kind)
}

// authFor is the sign-in a new kind starts with: Gmail keeps the one
// chosen, Microsoft signs in only with OAuth, the rest with a password.
func authFor(kind api.AccountKind, current api.AuthKind) api.AuthKind {
	switch kind {
	case api.AccountKindGmail:
		return current
	case api.AccountKindMicrosoft:
		return api.AuthKindOAuth2
	}
	return api.AuthKindPassword
}

func providerOf(kind api.AccountKind) string {
	if kind == api.AccountKindMicrosoft {
		return "Microsoft"
	}
	return "Google"
}

func kindLabel(kind api.AccountKind) string {
	switch kind {
	case api.AccountKindGmail:
		return "Gmail"
	case api.AccountKindMicrosoft:
		return "Microsoft"
	case api.AccountKindICloud:
		return "iCloud"
	}
	return "IMAP"
}

func serviceName(s api.ServiceKind) string {
	switch s {
	case api.ServiceKindCalendar:
		return "Calendars"
	case api.ServiceKindTasks:
		return "Tasks"
	}
	return "Contacts"
}

func (w *Window) account(id int64) *api.Account {
	if i := slices.IndexFunc(w.data.accounts, func(a api.Account) bool { return a.ID == id }); i >= 0 {
		return &w.data.accounts[i]
	}
	return nil
}

// syncAccounts follows the account list. A selected account that leaves
// the list was removed, and the form moves to the first. One the list has
// not shown yet stays selected: a new account's create returns before the
// list reloads on maild's event.
func (w *Window) syncAccounts(a *accountsPane) {
	if a == nil || !w.data.loaded || w.cn == nil {
		return
	}
	for _, account := range w.data.accounts {
		a.listed[account.ID] = true
	}
	removed := a.selected > 0 && a.listed[a.selected] && w.account(a.selected) == nil
	if a.selected == 0 || removed {
		w.selectAccount(a, w.firstAccount())
	}
	w.accountShown(a)
	// A sign-in that finished elsewhere hides the field: an account that
	// was not signed in now says it is.
	if v := a.waiting; v != nil && !v.wasSignedIn {
		if account := w.account(v.id); account != nil && account.SignedIn {
			a.waiting = nil
		}
	}
}

func (w *Window) firstAccount() int64 {
	if len(w.data.accounts) > 0 {
		return w.data.accounts[0].ID
	}
	return newAccount
}

func (w *Window) selectAccount(a *accountsPane, next int64) {
	a.selected = next
	if a.waiting != nil && a.waiting.id != next {
		a.waiting = nil
	}
	a.err, a.discovery, a.finish = "", discoveryState{}, ""
	a.formNo++
	a.value = emptyAccount()
	if account := w.account(next); account != nil {
		a.value = valueOf(*account)
	}
	w.accountShown(a)
}

// accountShown reads what is the selected account's own, once it changed.
func (w *Window) accountShown(a *accountsPane) {
	if a.shown == a.selected {
		return
	}
	a.shown = a.selected
	a.erase, a.eraseLoad = eraseNames{}, &loader{}
	a.services = nil
	if a.selected > 0 {
		a.services = &accountServices{id: a.selected, errors: map[api.ServiceKind]string{}, urls: map[api.ServiceKind]string{}}
		w.loadServices(a, a.services)
		w.loadEraseNames(a)
	}
}

// accountChanged reads the changed account's services again.
func (w *Window) accountChanged(id int64) {
	if a := w.acct; a != nil && a.services != nil && a.services.id == id {
		w.loadServices(a, a.services)
	}
}

func (w *Window) loadServices(a *accountsPane, s *accountServices) {
	s.load.run(w, func(ctx context.Context, c *api.Client) func() {
		list, err := c.Account().Services(ctx, &api.AccountServicesParams{ID: s.id})
		return func() {
			if err != nil {
				a.err = message(err)
				return
			}
			s.list = list
		}
	})
}

// loadEraseNames reads the names of the mailboxes that the behaviors
// erase. Without them the form offers no behavior, so nothing is erased.
func (w *Window) loadEraseNames(a *accountsPane) {
	if a == nil || a.selected <= 0 || a.eraseLoad == nil {
		return
	}
	id, l := a.selected, a.eraseLoad
	l.run(w, func(ctx context.Context, c *api.Client) func() {
		list, err := c.Mailbox().List(ctx, &api.MailboxListParams{AccountID: &id})
		return func() {
			if a.eraseLoad != l {
				return
			}
			a.erase = eraseNames{}
			if err != nil {
				return
			}
			for i := range list {
				if list[i].AccountID != id {
					continue
				}
				switch {
				case list[i].Role == api.MailboxRoleTrash && a.erase.trash == nil:
					a.erase.trash = &list[i].Name
				case list[i].Role == api.MailboxRoleJunk && a.erase.junk == nil:
					a.erase.junk = &list[i].Name
				}
			}
		}
	})
}

// expireSignIn ends a sign-in maild has stopped waiting for.
func (w *Window) expireSignIn(a *accountsPane) {
	if a != nil && a.waiting != nil && w.o.Now().After(a.waiting.until) {
		a.waiting = nil
	}
}

// startSignIn asks maild for the sign-in's page and opens it in the
// browser; it runs on a worker.
func (w *Window) startSignIn(ctx context.Context, c *api.Client, a *accountsPane, id int64, wasSignedIn bool) error {
	until := w.o.Now().Add(signInWait)
	w.o.Post(func() { a.waiting = &waitingSignIn{id: id, wasSignedIn: wasSignedIn, until: until} })
	if w.o.Wake != nil {
		time.AfterFunc(signInWait, w.o.Wake)
	}
	err := func() error {
		r, err := c.Account().Authorize(ctx, &api.AccountAuthorizeParams{ID: id})
		if err != nil {
			return err
		}
		if w.o.OpenURL == nil {
			return errors.New("cannot open a browser here")
		}
		return w.o.OpenURL(r.URL)
	}()
	if err != nil {
		w.o.Post(func() { a.waiting = nil })
	}
	return err
}

func (w *Window) authorize(a *accountsPane) {
	id := a.selected
	if id <= 0 {
		return
	}
	account := w.account(id)
	wasSignedIn := account != nil && account.SignedIn
	w.run(&a.request, func(ctx context.Context, c *api.Client) error {
		return w.startSignIn(ctx, c, a, id, wasSignedIn)
	}, noClient)
}

func (w *Window) finishSignIn(a *accountsPane) {
	url := strings.TrimSpace(a.finish)
	if url == "" {
		return
	}
	w.run(&a.request, func(ctx context.Context, c *api.Client) error {
		if _, err := c.Account().FinishSignIn(ctx, &api.AccountFinishSignInParams{URL: url}); err != nil {
			return err
		}
		w.o.Post(func() { a.waiting, a.finish = nil, "" })
		return nil
	}, nil)
}

func (w *Window) discover(a *accountsPane) {
	email := strings.TrimSpace(a.value.Email)
	a.discovery = discoveryState{finding: true}
	w.run(&a.request, func(ctx context.Context, c *api.Client) error {
		d, err := c.Account().Discover(ctx, &api.AccountDiscoverParams{Email: email})
		w.o.Post(func() {
			if err != nil {
				a.discovery = discoveryState{failed: message(err)}
				return
			}
			a.discovery = discoveryState{found: true, source: d.Source}
			a.value.Kind, a.value.Services, a.value.Auth = d.Kind, defaultServices(d.Kind), api.AuthKindPassword
			if len(d.Auth) > 0 {
				a.value.Auth = d.Auth[0]
			}
			if d.IMAP != nil {
				a.value.IMAP = withUser(*d.IMAP, email)
			}
			if d.SMTP != nil {
				a.value.SMTP = withUser(*d.SMTP, email)
			}
		})
		return nil
	}, nil)
}

func withUser(s api.ServerConfig, email string) api.ServerConfig {
	if s.Username == "" {
		s.Username = email
	}
	return s
}

func (w *Window) submitAccount(a *accountsPane) {
	if a.busy {
		return
	}
	if a.selected == newAccount {
		w.createAccount(a)
		return
	}
	if account := w.account(a.selected); account != nil {
		w.updateAccount(a, *account)
	}
}

func (w *Window) createAccount(a *accountsPane) {
	v := a.value
	w.run(&a.request, func(ctx context.Context, c *api.Client) error {
		readOnly, notify := v.ReadOnly, v.Notify
		p := &api.AccountCreateParams{Kind: v.Kind, Email: strings.TrimSpace(v.Email), DisplayName: v.DisplayName, Auth: v.Auth, ReadOnly: &readOnly, Notify: &notify}
		// Servers with an empty host are left out, so a Gmail or iCloud
		// profile fills them.
		if strings.TrimSpace(v.IMAP.Host) != "" {
			p.IMAP = &v.IMAP
		}
		if strings.TrimSpace(v.SMTP.Host) != "" {
			p.SMTP = &v.SMTP
		}
		created, err := c.Account().Create(ctx, p)
		if err != nil {
			return err
		}
		w.o.Post(func() {
			a.selected, a.value = created.ID, valueOf(*created)
			a.formNo++
			w.accountShown(a)
		})
		if v.Auth == api.AuthKindPassword && v.Password != "" {
			if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: created.ID, Password: v.Password}); err != nil {
				return err
			}
		}
		failed := enableServices(ctx, c, created.ID, v.Services)
		// Only the sign-in step can mean a missing OAuth client. A create or
		// password failure keeps maild's own message.
		if v.Auth == api.AuthKindOAuth2 {
			if err := w.startSignIn(ctx, c, a, created.ID, false); err != nil {
				if text := noClient(err); text != "" {
					return errors.New(text)
				}
				return err
			}
		}
		if failed != "" {
			return errors.New(failed)
		}
		return nil
	}, nil)
}

// enableServices turns on the form's services, in order. One that fails
// does not stop the others; the answer names the first.
func enableServices(ctx context.Context, c *api.Client, id int64, services []api.ServiceKind) string {
	failed := ""
	for _, s := range services {
		if _, err := c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: id, Service: s, Enabled: true}); err != nil && failed == "" {
			failed = "Could not turn on " + serviceName(s) + ": " + message(err)
		}
	}
	return failed
}

func (w *Window) updateAccount(a *accountsPane, account api.Account) {
	v := a.value
	p := &api.AccountUpdateParams{ID: account.ID, DisplayName: &v.DisplayName, IMAP: &v.IMAP, SMTP: &v.SMTP, ReadOnly: &v.ReadOnly, Notify: &v.Notify, SyncDays: &v.SyncDays}
	// A behavior whose mailbox the account has not got, or whose name has
	// not loaded, shows Never in a select the user cannot use: leave its
	// setting out, so this Save keeps the stored value.
	if a.erase.trash != nil {
		p.EraseTrashDays = &v.EraseTrash
	}
	if a.erase.junk != nil {
		p.EraseJunkDays = &v.EraseJunk
	}
	w.run(&a.request, func(ctx context.Context, c *api.Client) error {
		updated, err := c.Account().Update(ctx, p)
		if err != nil {
			return err
		}
		if v.Password != "" {
			if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: updated.ID, Password: v.Password}); err != nil {
				return err
			}
		}
		w.o.Post(func() {
			a.value = valueOf(*updated)
			a.secretNo++
		})
		return nil
	}, nil)
}

func (w *Window) removeAccount(a *accountsPane, id int64) {
	account := w.account(id)
	if account == nil {
		return
	}
	email := account.Email
	w.ask(frostui.Confirm{Title: "Remove Account?", Body: "Remove " + email + "? Its mail is deleted from this computer, not from the server.",
		Action: "Remove", Destructive: true, OnConfirm: func() {
			w.run(&a.request, func(ctx context.Context, c *api.Client) error {
				return c.Account().Delete(ctx, &api.AccountDeleteParams{ID: id})
			}, nil)
		}})
}

func (w *Window) toggleService(a *accountsPane, s *accountServices, service api.ServiceKind, enabled bool, url string) {
	cn := w.cn
	if cn == nil {
		return
	}
	s.busy = service
	go func() {
		ctx, cancel := context.WithTimeout(cn.ctx, callTimeout)
		defer cancel()
		p := &api.AccountSetServiceParams{ID: s.id, Service: service, Enabled: enabled}
		if url != "" {
			p.URL = &url
		}
		_, err := cn.client.Account().SetService(ctx, p)
		w.o.Post(func() {
			if s.busy == service {
				s.busy = ""
			}
			if err != nil {
				s.errors[service] = message(err)
				return
			}
			delete(s.errors, service)
			delete(s.urls, service)
			w.loadServices(a, s)
		})
	}()
}

// accountsBody draws the list over the selected account's form: beside the
// settings window's own list there is no room for both across.
func (w *Window) accountsBody(c *ui.Context) {
	a := w.acct
	if a == nil {
		return
	}
	ui.Column(c).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Children(func() {
		w.accountList(c, a)
		if a.selected != 0 {
			ui.Column(c.Key(a.formNo)).Gap(frostui.SpaceL).AlignItems(ui.Stretch).Children(func() { w.accountForm(c, a) })
		}
	})
}

func (w *Window) accountList(c *ui.Context, a *accountsPane) {
	ui.Column(c).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Children(func() {
		listBox(c, "Accounts", func() {
			for _, account := range w.data.accounts {
				title := account.DisplayName
				if strings.TrimSpace(title) == "" {
					title = account.Email
				}
				status := "Sign in again"
				if account.SignedIn {
					status = kindLabel(account.Kind)
					if account.ReadOnly {
						status += " · Read-only"
					}
				}
				if listRow(c.Key(account.ID), title, status, a.selected == account.ID, !account.SignedIn).Clicked() {
					w.selectAccount(a, account.ID)
				}
			}
			if a.selected == newAccount {
				listRow(c.Key("new"), "New Account", "", true, false)
			}
		})
		add, remove := addRemove(c, "Add Account", "Remove Account", a.selected == newAccount, a.selected <= 0)
		if add {
			w.selectAccount(a, newAccount)
		}
		if remove {
			w.removeAccount(a, a.selected)
		}
	})
}

func (w *Window) accountForm(c *ui.Context, a *accountsPane) {
	v := &a.value
	editing := a.selected != newAccount
	title := "Add Account"
	if editing {
		title = v.DisplayName
		if strings.TrimSpace(title) == "" {
			title = v.Email
		}
	}
	submit := func() { w.submitAccount(a) }
	frostui.Heading(c, title).Role(ui.RoleHeading).Level(2)
	ui.Form(c, func() {
		ui.Field(c.Key("email"), "Email Address:", func() {
			if ui.TextInput(c, &v.Email).ReadOnly(editing).Submitted() {
				submit()
			}
			if !editing {
				w.findSettings(c, a)
			}
		})
		ui.Field(c.Key("name"), "Full Name:", func() {
			if ui.TextInput(c, &v.DisplayName).Submitted() {
				submit()
			}
		})
		ui.Field(c.Key("kind"), "Account Type:", func() {
			choose(c, "Account Type", v.Kind, kindOptions(), editing, func(kind api.AccountKind) {
				v.Kind, v.Auth = kind, authFor(kind, v.Auth)
				if !editing {
					v.Services = defaultServices(kind)
				}
			})
		})
		ui.Field(c.Key("auth"), "Sign In:", func() {
			choose(c, "Sign In", v.Auth, authOptions(v.Kind), editing, func(auth api.AuthKind) { v.Auth = auth })
		})
		if v.Auth == api.AuthKindPassword {
			w.passwordRow(c, a, editing, submit)
		} else {
			w.signInRow(c, a, editing)
		}
	})
	serverFields(c.Key("imap"), "imap", "Incoming Mail (IMAP)", &v.IMAP, a.busy)
	serverFields(c.Key("smtp"), "smtp", "Outgoing Mail (SMTP)", &v.SMTP, a.busy)
	ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Start).Children(func() {
		ui.Checkbox(c, &v.ReadOnly, "Read only: never change anything on the server")
		ui.Checkbox(c, &v.Notify, "Notify me about new mail")
	})
	if editing {
		w.syncWindow(c, a)
		w.mailboxBehaviors(c, a)
		if a.services != nil {
			w.servicesSection(c, a, a.services)
		}
	} else {
		w.addServices(c, a)
	}
	w.formAlert(c, a)
	buttons(c, func() {
		if !editing && ui.Button(c, "Cancel").Clicked() {
			w.selectAccount(a, w.firstAccount())
		}
		// Its name differs from the + button's, which adds a form, not an
		// account.
		label, name := "Add Account", "Add This Account"
		if editing {
			label, name = "Save", "Save"
		}
		if ui.PrimaryButton(c, label).Label(name).Disabled(a.busy).Clicked() {
			submit()
		}
	})
}

// formAlert shows the last request's error and scrolls it into view: it
// sits below the services, out of sight when the window is shorter than
// the form, where a failed Sign In… would seem to do nothing.
func (w *Window) formAlert(c *ui.Context, a *accountsPane) {
	if a.err == "" {
		a.alerted = ""
		return
	}
	e := frostui.Problem(c, a.err).Role(ui.RoleAlert)
	if a.err != a.alerted {
		a.alerted = a.err
		e.ScrollIntoView()
	}
}

func kindOptions() []option[api.AccountKind] {
	out := []option[api.AccountKind]{}
	for _, k := range []api.AccountKind{api.AccountKindIMAP, api.AccountKindGmail, api.AccountKindMicrosoft, api.AccountKindICloud} {
		out = append(out, option[api.AccountKind]{k, kindLabel(k)})
	}
	return out
}

func authOptions(kind api.AccountKind) []option[api.AuthKind] {
	switch kind {
	case api.AccountKindMicrosoft:
		return []option[api.AuthKind]{{api.AuthKindOAuth2, "Microsoft sign-in"}}
	case api.AccountKindGmail:
		return []option[api.AuthKind]{{api.AuthKindPassword, "Password"}, {api.AuthKindOAuth2, "Google sign-in"}}
	}
	return []option[api.AuthKind]{{api.AuthKindPassword, "Password"}}
}

func (w *Window) findSettings(c *ui.Context, a *accountsPane) {
	d := a.discovery
	off := a.busy || d.finding || !validEmail(strings.TrimSpace(a.value.Email))
	ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, "Find Settings").Disabled(off).Clicked() {
			w.discover(a)
		}
		switch {
		case d.finding:
			frostui.Caption(c, "Looking up servers…").Wrap().Shrink(1)
		case d.failed != "":
			frostui.Caption(c, d.failed).TextColor(c.Theme().Danger).Wrap().Shrink(1)
		case d.found && d.source == api.DiscoverySourceNone:
			frostui.Caption(c, "No settings found; enter the servers below.").Wrap().Shrink(1)
		case d.found:
			frostui.Caption(c, "Found settings for this address.").Wrap().Shrink(1)
		}
	})
}

func (w *Window) passwordRow(c *ui.Context, a *accountsPane, editing bool, submit func()) {
	field := ui.Field(c.Key("password"), "Password:", func() {
		in := ui.TextInput(c.Key(a.secretNo), &a.value.Password).Password()
		if editing {
			in.Placeholder("Unchanged")
		}
		if in.Submitted() {
			submit()
		}
	})
	switch a.value.Kind {
	case api.AccountKindGmail:
		field.Description("Use an app password from your Google account.")
	case api.AccountKindICloud:
		field.Description("Use an app-specific password from appleid.apple.com.")
	}
}

func (w *Window) signInRow(c *ui.Context, a *accountsPane, editing bool) {
	provider := providerOf(a.value.Kind)
	ui.Field(c.Key("signin"), provider+":", func() {
		if !editing {
			covers := ""
			if len(offeredServices(a.value.Kind)) > 0 {
				covers = ", once for mail and the services checked below"
			}
			note(c, "After adding the account, sign in with "+provider+" in your browser"+covers+".")
			return
		}
		account := w.account(a.selected)
		signedIn := account != nil && account.SignedIn
		ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
			label, button := "Not signed in", "Sign In…"
			if signedIn {
				label, button = "Signed in", "Sign In Again…"
			}
			status := frostui.Body(c, label)
			if !signedIn {
				status.TextColor(c.Theme().Danger)
			}
			if ui.Button(c, button).Disabled(a.busy).Clicked() {
				w.authorize(a)
			}
		})
		if a.waiting != nil && a.waiting.id == a.selected && !w.onThisComputer() {
			w.finishRow(c, a)
		}
	})
}

func (w *Window) finishRow(c *ui.Context, a *accountsPane) {
	note(c, "If your browser cannot open the page after you sign in, copy that page's address and paste it here.")
	ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		frostui.Muted(c, "Address").SingleLine()
		ui.TextInput(c, &a.finish).Label("Address").Disabled(a.busy).Grow(1).MinWidth(0)
		if ui.Button(c, "Finish Sign-In").Disabled(a.busy || strings.TrimSpace(a.finish) == "").Clicked() {
			w.finishSignIn(a)
		}
	})
}

// defaultPort is the usual port of a server kind over a security mode.
func defaultPort(id string, tls api.TLSMode) int64 {
	switch {
	case id == "imap" && tls == api.TLSModeTLS:
		return 993
	case id == "imap":
		return 143
	case tls == api.TLSModeTLS:
		return 465
	case tls == api.TLSModeStartTLS:
		return 587
	}
	return 25
}

// withTLS changes the security mode, moving a port that was the old
// mode's default along with it.
func withTLS(id string, s api.ServerConfig, tls api.TLSMode) api.ServerConfig {
	if s.Port == 0 || s.Port == defaultPort(id, s.TLS) {
		s.Port = defaultPort(id, tls)
	}
	s.TLS = tls
	return s
}

// parsePort reads a port as Number.parseInt does: its leading digits, or 0.
func parsePort(text string) int64 {
	text = strings.TrimSpace(text)
	end := 0
	for end < len(text) && text[end] >= '0' && text[end] <= '9' {
		end++
	}
	n, err := strconv.ParseInt(text[:end], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func tlsOptions() []option[api.TLSMode] {
	return []option[api.TLSMode]{{api.TLSModeTLS, "TLS"}, {api.TLSModeStartTLS, "STARTTLS"}, {api.TLSModeInsecure, "None (insecure)"}}
}

// serverFields edits one server's host, port, security and user name. id
// names its fields for assistive technology, as "imap Server".
func serverFields(c *ui.Context, id, legend string, v *api.ServerConfig, off bool) {
	name := strings.ToUpper(id)
	ui.Form(c, func() {
		ui.Fieldset(c, legend, func() {
			ui.Field(c.Key("host"), "Server:", func() { ui.TextInput(c, &v.Host).Label(name + " Server") })
			ui.Field(c.Key("port"), "Port:", func() {
				text := ""
				if v.Port != 0 {
					text = strconv.FormatInt(v.Port, 10)
				}
				if ui.TextInput(c, &text).Label(name + " Port").Width(96).Changed() {
					v.Port = parsePort(text)
				}
			})
			ui.Field(c.Key("tls"), "Security:", func() {
				choose(c, name+" Security", v.TLS, tlsOptions(), off, func(tls api.TLSMode) { *v = withTLS(id, *v, tls) })
			})
			ui.Field(c.Key("user"), "User Name:", func() { ui.TextInput(c, &v.Username).Label(name + " User Name") })
		}).Disabled(off)
	})
}

// syncWindows are the "Keep mail from" choices, in the order offered
// (ADR-0016); a stored value outside them keeps an option of its own.
func syncWindows(days int64) []option[int64] {
	out := []option[int64]{{0, "All mail"}, {30, "Last 30 days"}, {90, "Last 90 days"}, {183, "Last 6 months"}, {365, "Last year"}, {730, "Last 2 years"}}
	if !slices.ContainsFunc(out, func(o option[int64]) bool { return o.value == days }) {
		out = append(out, option[int64]{days, fmt.Sprintf("Last %d days", days)})
	}
	return out
}

func (w *Window) syncWindow(c *ui.Context, a *accountsPane) {
	v := &a.value
	ui.Form(c, func() {
		ui.Field(c.Key("sync"), "Keep mail from:", func() {
			choose(c, "Keep mail from", v.SyncDays, syncWindows(v.SyncDays), a.busy, func(days int64) { v.SyncDays = days })
		}).Description("A longer time downloads older mail from the server. A shorter time removes older mail from this computer only; it stays on the server.")
	})
}

func eraseOptions() []option[int64] {
	return []option[int64]{{0, "Never"}, {1, "After One Day"}, {7, "After One Week"}, {30, "After One Month"}}
}

func (w *Window) mailboxBehaviors(c *ui.Context, a *accountsPane) {
	v := &a.value
	row := func(key, label string, mailbox *string, stored *int64) {
		ui.Field(c.Key(key), label+":", func() {
			shown := *stored
			if mailbox == nil {
				shown = 0
			}
			choose(c, label, shown, eraseOptions(), a.busy || v.ReadOnly || mailbox == nil, func(days int64) { *stored = days })
		})
	}
	ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Role(ui.RoleGroup).Label("Mailbox Behaviors").Children(func() {
		frostui.Strong(c, "Mailbox Behaviors").Role(ui.RoleHeading).Level(3)
		ui.Form(c, func() {
			row("trash", "Erase deleted messages", a.erase.trash, &v.EraseTrash)
			row("junk", "Erase junk messages", a.erase.junk, &v.EraseJunk)
		})
		var names []string
		for _, name := range []*string{a.erase.trash, a.erase.junk} {
			if name != nil && *name != "" {
				names = append(names, "“"+*name+"”")
			}
		}
		if len(names) > 0 {
			note(c, "Messages are erased from the server after they have been in "+strings.Join(names, " or ")+" that long, and cannot be recovered. Mail older than this account's sync window is left on the server.")
		}
	})
}

// addServices are the services to turn on with a new account.
func (w *Window) addServices(c *ui.Context, a *accountsPane) {
	v := &a.value
	offered := offeredServices(v.Kind)
	if len(offered) == 0 {
		return
	}
	ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Start).Role(ui.RoleGroup).Label("Contacts, Calendars and Tasks").Children(func() {
		frostui.Strong(c, "Contacts, Calendars and Tasks").Role(ui.RoleHeading).Level(3)
		for _, s := range offered {
			on := slices.Contains(v.Services, s)
			if ui.Checkbox(c.Key(s), &on, serviceName(s)).Disabled(a.busy).Changed() {
				var next []api.ServiceKind
				for _, k := range offered {
					if k == s && on || k != s && slices.Contains(v.Services, k) {
						next = append(next, k)
					}
				}
				v.Services = next
			}
		}
		if v.Auth == api.AuthKindOAuth2 {
			note(c, providerOf(v.Kind)+" asks once for mail and the services checked here.")
		} else {
			note(c, "Frostmail finds them with the account's password.")
		}
	})
}

// joinNames is a list of names as a sentence says it.
func joinNames(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// servicesSection is an existing account's services (ADR-0017).
func (w *Window) servicesSection(c *ui.Context, a *accountsPane, s *accountServices) {
	var available []api.ServiceSettings
	var reasons []string
	for _, svc := range s.list {
		if svc.Available {
			available = append(available, svc)
		} else if svc.Reason != nil && *svc.Reason != "" {
			reasons = append(reasons, *svc.Reason)
		}
	}
	account := w.account(s.id)
	provider := "Google"
	if account != nil {
		provider = providerOf(account.Kind)
	}
	ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Role(ui.RoleGroup).Label("Contacts, Calendars and Tasks").Children(func() {
		if len(available) == 0 {
			note(c, "This account has no contacts, calendars or tasks Frostmail can reach.")
		} else {
			frostui.Strong(c, "Contacts, Calendars and Tasks").Role(ui.RoleHeading).Level(3)
			for _, svc := range available {
				w.serviceRow(c.Key(svc.Service), a, s, svc, provider)
			}
			var waiting []string
			for _, svc := range available {
				if svc.Enabled && !svc.SignedIn {
					waiting = append(waiting, serviceName(svc.Service))
				}
			}
			if len(waiting) > 0 {
				ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
					frostui.Caption(c, "Sign in to "+provider+" to allow "+joinNames(waiting)+".").TextColor(c.Theme().Danger).Wrap().Shrink(1)
					if ui.Button(c.Key("signin-services"), "Sign In…").Clicked() {
						w.authorize(a)
					}
				})
			}
		}
		for _, reason := range reasons {
			note(c, reason)
		}
	})
}

func (w *Window) serviceRow(c *ui.Context, a *accountsPane, s *accountServices, svc api.ServiceSettings, provider string) {
	ui.Column(c).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
			on := svc.Enabled
			if ui.Checkbox(c, &on, serviceName(svc.Service)).Disabled(s.busy == svc.Service).Changed() {
				w.toggleService(a, s, svc.Service, on, "")
			}
			text, failed := serviceStatus(svc, s, provider, w.o.Now())
			if text != "" {
				line := frostui.Caption(c, text).Wrap().Shrink(1)
				if failed {
					line.TextColor(c.Theme().Danger)
				}
			}
		})
		if failure := s.errors[svc.Service]; !svc.Enabled && failure != "" {
			alert(c, failure)
			w.serviceURLRow(c, a, s, svc.Service)
		}
	})
}

// serviceStatus says how a service stands, and whether that is a failure.
func serviceStatus(svc api.ServiceSettings, s *accountServices, provider string, now time.Time) (string, bool) {
	switch {
	case s.busy == svc.Service:
		return "Connecting…", false
	case !svc.Enabled:
		return "", false
	case !svc.SignedIn:
		return "Waiting for " + provider + " sign-in", false
	}
	if failure := s.errors[svc.Service]; failure != "" {
		return failure, true
	}
	if svc.Error != nil && *svc.Error != "" {
		return *svc.Error, true
	}
	if svc.LastSyncAt == nil {
		return "Waiting for the first sync", false
	}
	synced := svc.LastSyncAt.In(now.Location())
	if y, m, d := now.Date(); synced.Year() == y && synced.Month() == m && synced.Day() == d {
		return "Synced at " + synced.Format("3:04 PM"), false
	}
	return "Synced " + synced.Format("1/2/06"), false
}

func (w *Window) serviceURLRow(c *ui.Context, a *accountsPane, s *accountServices, service api.ServiceKind) {
	url := s.urls[service]
	ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
		frostui.Muted(c, "Server:").SingleLine()
		if ui.TextInput(c, &url).Label(serviceName(service) + " Server").Placeholder("https://dav.example.com/").Grow(1).MinWidth(0).Changed() {
			s.urls[service] = url
		}
		if ui.Button(c, "Connect").Disabled(strings.TrimSpace(url) == "" || s.busy == service).Clicked() {
			w.toggleService(a, s, service, true, strings.TrimSpace(url))
		}
	})
}
