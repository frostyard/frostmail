package settings_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/settings"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// spy records the calls of account.* in order, and can fake some.
type spy struct {
	api.AccountService
	mu       sync.Mutex
	calls    []string
	update   *api.AccountUpdateParams
	password string
	failSvc  map[api.ServiceKind]error
	services []api.ServiceSettings
	discover error
	setURL   string
}

func (s *spy) log(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

func (s *spy) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

func (s *spy) Create(ctx context.Context, p *api.AccountCreateParams) (*api.Account, error) {
	s.log("create")
	return s.AccountService.Create(ctx, p)
}

func (s *spy) Update(ctx context.Context, p *api.AccountUpdateParams) (*api.Account, error) {
	s.log("update")
	s.mu.Lock()
	s.update = p
	s.mu.Unlock()
	return s.AccountService.Update(ctx, p)
}

func (s *spy) SetPassword(ctx context.Context, p *api.AccountSetPasswordParams) error {
	s.log("setPassword")
	s.mu.Lock()
	s.password = p.Password
	s.mu.Unlock()
	return s.AccountService.SetPassword(ctx, p)
}

func (s *spy) SetService(ctx context.Context, p *api.AccountSetServiceParams) (*api.ServiceSettings, error) {
	s.log("setService " + string(p.Service))
	s.mu.Lock()
	err := s.failSvc[p.Service]
	if p.URL != nil {
		s.setURL = *p.URL
	}
	faked := s.services != nil
	if err == nil && faked {
		for i := range s.services {
			if s.services[i].Service == p.Service {
				s.services[i].Enabled = p.Enabled
			}
		}
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if faked {
		return &api.ServiceSettings{Service: p.Service, Enabled: p.Enabled}, nil
	}
	return s.AccountService.SetService(ctx, p)
}

func (s *spy) Services(ctx context.Context, p *api.AccountServicesParams) ([]api.ServiceSettings, error) {
	s.mu.Lock()
	faked := slices.Clone(s.services)
	s.mu.Unlock()
	if faked != nil {
		return faked, nil
	}
	return s.AccountService.Services(ctx, p)
}

func (s *spy) Authorize(ctx context.Context, p *api.AccountAuthorizeParams) (*api.AuthorizeResult, error) {
	s.log("authorize")
	return s.AccountService.Authorize(ctx, p)
}

func (s *spy) Discover(ctx context.Context, p *api.AccountDiscoverParams) (*api.Discovery, error) {
	s.mu.Lock()
	err := s.discover
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return s.AccountService.Discover(ctx, p)
}

// accountsFixture runs the window with a spy on account.*, and a fake token
// endpoint for Google.
func accountsFixture(t *testing.T, opts func(*settings.Options)) (*fixture, *spy) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("code") == "c0de" {
			_, _ = io.WriteString(w, `{"access_token":"tok","expires_in":3600,"refresh_token":"ref"}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
	}))
	t.Cleanup(ts.Close)
	s := &spy{failSvc: map[api.ServiceKind]error{}}
	f := newFixture(t, setup{options: opts, server: rpctest.Options{
		OAuthEndpoints: map[string]oauth.Endpoint{"google": {AuthURL: "https://accounts.test/auth", TokenURL: ts.URL, Scopes: oauth.Google.Scopes}},
		Services:       func(sv *api.Services) { s.AccountService = sv.Account; sv.Account = s },
	}})
	return f, s
}

func (f *fixture) googleClient() {
	f.t.Helper()
	secret := "test-secret"
	if _, err := f.c.Oauth().SetClient(f.t.Context(), &api.OauthSetClientParams{Provider: api.OAuthProviderGoogle, ClientID: "cid", ClientSecret: &secret}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) inLodge() {
	f.t.Helper()
	l, err := conn.ParseLodge("https://lodge.example.ts.net")
	if err != nil {
		f.t.Fatal(err)
	}
	f.record(func() { f.state = conn.State{Target: conn.Target{Lodge: &l}, Connected: true} })
}

func (f *fixture) listed() []api.Account {
	f.t.Helper()
	list, err := f.c.Account().List(f.t.Context(), &api.AccountListParams{})
	if err != nil {
		f.t.Fatal(err)
	}
	return list
}

func TestNoAccountStartsAdding(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.sees("New Account")
	if !f.disabled("Add Account") || !f.disabled("Remove Account") {
		t.Error("the + and − buttons work while adding with no account")
	}
	for _, want := range []string{"Email Address:", "Full Name:", "Account Type:", "Sign In:", "Password:", "Incoming Mail (IMAP)", "Outgoing Mail (SMTP)",
		"Read only: never change anything on the server", "Notify me about new mail", "Contacts, Calendars and Tasks", "Cancel"} {
		if !f.tt.HasText(want) {
			t.Errorf("no %q in %q", want, f.tt.Texts())
		}
	}
	for _, not := range []string{"Keep mail from:", "Mailbox Behaviors"} {
		f.notSeen(not)
	}
	if f.checked("Read only: never change anything on the server") || !f.checked("Notify me about new mail") {
		t.Error("a new account is not writable with notifications on")
	}
	if f.value("IMAP Port") != "993" || f.value("SMTP Port") != "465" {
		t.Errorf("default ports %q, %q", f.value("IMAP Port"), f.value("SMTP Port"))
	}
}

func TestAddAccountWithPassword(t *testing.T) {
	f, s := accountsFixture(t, nil)
	f.sees("New Account")
	if !f.disabled("Find Settings") {
		t.Error("Find Settings works with no address")
	}
	f.fill("Email Address:", "ann@a.test")
	if f.disabled("Find Settings") {
		t.Error("Find Settings is off for a valid address")
	}
	f.fill("Full Name:", "Ann Example")
	f.fill("Password:", "test-password")
	if a := f.control(ui.RoleTextField, "Password:"); !a.Password || a.Value == "test-password" {
		t.Errorf("the password field shows its text: %+v", a)
	}
	f.fill("IMAP Server", "imap.a.test")
	f.fill("IMAP User Name", "ann@a.test")
	f.fill("SMTP Server", "smtp.a.test")
	f.fill("SMTP User Name", "ann@a.test")
	f.check("Contacts")
	f.click("Add This Account")
	f.until("the new account's form", func() bool { return f.tt.HasText("Keep mail from:") })
	list := f.listed()
	if len(list) != 1 || list[0].Email != "ann@a.test" || list[0].DisplayName != "Ann Example" || list[0].IMAP.Host != "imap.a.test" || list[0].ReadOnly || !list[0].Notify {
		t.Fatalf("account = %+v", list)
	}
	f.until("the calls", func() bool { return len(s.seen()) == 3 })
	if got := s.seen(); !slices.Equal(got, []string{"create", "setPassword", "setService contacts"}) {
		t.Errorf("calls = %q", got)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.password != "test-password" {
		t.Error("account.setPassword did not get what was typed")
	}
}

func TestAddAccountLeavesOutEmptyServers(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.sees("New Account")
	f.fill("Email Address:", "ann@gmail.com")
	f.choose("Account Type", "Gmail")
	f.check("Contacts") // on by default: off
	f.check("Calendars")
	f.check("Tasks")
	f.click("Add This Account")
	f.until("the account", func() bool { return f.tt.HasText("Keep mail from:") })
	list := f.listed()
	if len(list) != 1 || list[0].IMAP.Host != "imap.gmail.com" {
		t.Errorf("a Gmail profile did not fill the servers: %+v", list)
	}
}

func TestAddAccountKeepsTheFormOnAnError(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.sees("New Account")
	f.fill("Email Address:", "not an address")
	f.fill("Full Name:", "Ann")
	f.click("Add This Account")
	f.until("maild's error", func() bool {
		a, ok := f.tt.AccessByRole(ui.RoleAlert, "")
		return ok && a.Bounds.W > 0
	})
	f.sees("New Account")
	if f.value("Full Name:") != "Ann" {
		t.Errorf("the form lost its name: %q", f.value("Full Name:"))
	}
}

func TestAnErrorBelowTheFoldScrollsIntoView(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.sees("New Account")
	f.tt.SetSize(settings.Width, settings.Height)
	f.fill("Email Address:", "not an address")
	f.tt.Key(0, ui.KeyEnter)
	f.until("the error in view", func() bool {
		a, ok := f.tt.AccessByRole(ui.RoleAlert, "")
		return ok && a.Bounds.H > 0 && a.Bounds.Y+a.Bounds.H <= settings.Height
	})
}

func TestFindSettingsFillsTheForm(t *testing.T) {
	f, s := accountsFixture(t, nil)
	f.sees("New Account")
	f.fill("Email Address:", "ann@gmail.com")
	f.click("Find Settings")
	f.sees("Found settings for this address.")
	if f.choice("Account Type") != "Gmail" || f.choice("Sign In") != "Google sign-in" {
		t.Errorf("kind %q, auth %q", f.choice("Account Type"), f.choice("Sign In"))
	}
	if f.value("IMAP Server") != "imap.gmail.com" || f.value("IMAP User Name") != "ann@gmail.com" {
		t.Errorf("servers: %q, %q", f.value("IMAP Server"), f.value("IMAP User Name"))
	}
	for _, name := range []string{"Contacts", "Calendars", "Tasks"} {
		if !f.checked(name) {
			t.Errorf("%s is off for a found Gmail", name)
		}
	}
	f.sees("After adding the account, sign in with Google in your browser, once for mail and the services checked below.")
	f.sees("Google asks once for mail and the services checked here.")

	f.fill("Email Address:", "ann@nothing.test")
	f.click("Find Settings")
	f.sees("No settings found; enter the servers below.")

	s.mu.Lock()
	s.discover = errors.New("the lookup failed")
	s.mu.Unlock()
	f.click("Find Settings")
	f.sees("the lookup failed")
}

func TestChangingTheKindSetsSignInAndServices(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.sees("New Account")
	f.choose("Account Type", "Microsoft")
	if f.choice("Sign In") != "Microsoft sign-in" {
		t.Errorf("Microsoft's sign-in is %q", f.choice("Sign In"))
	}
	f.sees("Microsoft:")
	f.notSeen("Password:")
	f.sees("Microsoft asks once for mail and the services checked here.")
	if _, ok := f.tt.AccessByRole(ui.RoleCheckBox, "Tasks"); ok {
		t.Error("Microsoft offers Tasks")
	}
	f.choose("Account Type", "iCloud")
	if f.choice("Sign In") != "Password" {
		t.Errorf("iCloud's sign-in is %q", f.choice("Sign In"))
	}
	f.sees("Use an app-specific password from appleid.apple.com.")
	f.sees("Frostmail finds them with the account's password.")
	f.choose("Account Type", "Gmail")
	f.sees("Use an app password from your Google account.")
	f.choose("Account Type", "IMAP")
	for _, name := range []string{"Contacts", "Calendars", "Tasks"} {
		if f.checked(name) {
			t.Errorf("%s is on for IMAP", name)
		}
	}
}

func TestSecurityMovesADefaultPort(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.sees("New Account")
	f.choose("IMAP Security", "STARTTLS")
	if f.value("IMAP Port") != "143" {
		t.Errorf("IMAP over STARTTLS is on port %q", f.value("IMAP Port"))
	}
	f.choose("SMTP Security", "STARTTLS")
	if f.value("SMTP Port") != "587" {
		t.Errorf("SMTP over STARTTLS is on port %q", f.value("SMTP Port"))
	}
	f.choose("SMTP Security", "None (insecure)")
	if f.value("SMTP Port") != "25" {
		t.Errorf("insecure SMTP is on port %q", f.value("SMTP Port"))
	}
	f.fill("IMAP Port", "1143")
	f.choose("IMAP Security", "TLS")
	if f.value("IMAP Port") != "1143" {
		t.Errorf("a port of the user's moved: %q", f.value("IMAP Port"))
	}
}

func TestAccountListSaysHowEachStands(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.addIMAP("plain@a.test")
	b := f.addIMAP("named@a.test")
	if err := f.c.Account().SetPassword(t.Context(), &api.AccountSetPasswordParams{ID: b.ID, Password: "test-password"}); err != nil {
		t.Fatal(err)
	}
	name, read := "Ann Example", true
	if _, err := f.c.Account().Update(t.Context(), &api.AccountUpdateParams{ID: b.ID, DisplayName: &name, ReadOnly: &read}); err != nil {
		t.Fatal(err)
	}
	f.sees("Ann Example")
	f.sees("plain@a.test")
	f.sees("Sign in again")
	f.sees("IMAP · Read-only")
	f.click("plain@a.test")
	if a := f.control(ui.RoleButton, "plain@a.test"); a.Current != ui.CurrentItem {
		t.Errorf("the selected row's current = %v", a.Current)
	}
}

func TestEditingSavesEveryFieldAndANewPassword(t *testing.T) {
	f, s := accountsFixture(t, nil)
	a := f.addIMAP("me@a.test")
	f.open("me@a.test")
	f.until("the form", func() bool { return f.tt.HasText("Keep mail from:") })
	email := f.control(ui.RoleTextField, "Email Address:")
	if !email.ReadOnly {
		t.Error("the address can be edited")
	}
	if !f.control(ui.RolePopUpButton, "Account Type").Disabled || !f.control(ui.RolePopUpButton, "Sign In").Disabled {
		t.Error("the kind or the sign-in can be changed")
	}
	if p := f.control(ui.RoleTextField, "Password:"); p.Placeholder != "Unchanged" {
		t.Errorf("the password's placeholder is %q", p.Placeholder)
	}
	f.fill("Full Name:", "Me Myself")
	f.fill("Password:", "new-test-password")
	f.fill("IMAP Server", "imap2.a.test")
	f.check("Notify me about new mail")
	f.choose("Keep mail from", "Last year")
	f.click("Save")
	f.until("the saved name", func() bool {
		got, err := f.c.Account().Get(t.Context(), &api.AccountGetParams{ID: a.ID})
		return err == nil && got.DisplayName == "Me Myself" && got.IMAP.Host == "imap2.a.test" && !got.Notify && got.SyncDays == 365
	})
	f.until("the password's call", func() bool { return slices.Contains(s.seen(), "setPassword") })
	f.until("the emptied password", func() bool { return f.value("Password:") == "" })
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.password != "new-test-password" {
		t.Error("account.setPassword did not get what was typed")
	}
}

func TestKeepMailShowsAWindowSetElsewhere(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	days := int64(45)
	if _, err := f.c.Account().Create(t.Context(), &api.AccountCreateParams{Kind: api.AccountKindIMAP, Email: "me@a.test", Auth: api.AuthKindPassword, SyncDays: &days,
		IMAP: &api.ServerConfig{Host: "imap.x.test", Port: 993, TLS: api.TLSModeTLS, Username: "me"}, SMTP: &api.ServerConfig{Host: "smtp.x.test", Port: 465, TLS: api.TLSModeTLS, Username: "me"}}); err != nil {
		t.Fatal(err)
	}
	f.open("me@a.test")
	f.until("the form", func() bool { return f.tt.HasText("Keep mail from:") })
	if f.choice("Keep mail from") != "Last 45 days" {
		t.Errorf("Keep mail from = %q", f.choice("Keep mail from"))
	}
}

func (f *fixture) mailboxes(a int64, boxes ...store.ServerMailbox) {
	f.t.Helper()
	ctx := f.t.Context()
	if err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.ReplaceMailboxes(ctx, a, boxes)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
}

func TestMailboxBehaviorsNameTheMailboxesAndLeaveOutTheRest(t *testing.T) {
	f, s := accountsFixture(t, nil)
	a := f.addIMAP("me@a.test")
	f.mailboxes(a.ID, store.ServerMailbox{Path: "Bin", Role: api.MailboxRoleTrash, Selectable: true})
	f.open("me@a.test")
	f.until("the form", func() bool { return f.tt.HasText("Keep mail from:") })
	f.until("the note", func() bool {
		return f.tt.HasText("Messages are erased from the server after they have been in “Bin” that long")
	})
	if f.control(ui.RolePopUpButton, "Erase deleted messages").Disabled {
		t.Error("the Trash behavior is off")
	}
	if !f.control(ui.RolePopUpButton, "Erase junk messages").Disabled || f.choice("Erase junk messages") != "Never" {
		t.Error("the Junk behavior is on with no Junk mailbox")
	}
	f.choose("Erase deleted messages", "After One Week")
	f.click("Save")
	f.until("the update", func() bool { return slices.Contains(s.seen(), "update") })
	s.mu.Lock()
	p := s.update
	s.mu.Unlock()
	if p.EraseTrashDays == nil || *p.EraseTrashDays != 7 {
		t.Errorf("eraseTrashDays = %v", p.EraseTrashDays)
	}
	if p.EraseJunkDays != nil {
		t.Errorf("eraseJunkDays sent with no Junk mailbox: %v", *p.EraseJunkDays)
	}
	// The save's answer resets the form, so check until the box stays.
	f.until("the behaviors off", func() bool {
		if !f.checked("Read only: never change anything on the server") {
			f.check("Read only: never change anything on the server")
		}
		return f.control(ui.RolePopUpButton, "Erase deleted messages").Disabled
	})
}

func TestAnAnswerAfterAChangeOfConnectionRemovesNothing(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.addIMAP("me@a.test")
	f.open("me@a.test")
	f.until("the form", func() bool { return f.tt.HasText("Keep mail from:") })
	f.click("Remove Account")
	f.sees("Remove me@a.test? Its mail is deleted from this computer, not from the server.")
	f.win.Disconnect()
	f.connect()
	f.until("the new connection", func() bool { return f.tt.HasText("me@a.test") })
	f.press(f.control(ui.RoleButton, "Remove"))
	f.tt.Frame()
	if list := f.listed(); len(list) != 1 {
		t.Errorf("accounts = %+v", list)
	}
}

func TestMailboxBehaviorsFollowMailboxChanged(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	a := f.addIMAP("me@a.test")
	f.open("me@a.test")
	f.until("the form", func() bool { return f.tt.HasText("Keep mail from:") })
	if !f.control(ui.RolePopUpButton, "Erase junk messages").Disabled {
		t.Error("the Junk behavior is on with no Junk mailbox")
	}
	ctx := t.Context()
	if err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		if _, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{{Path: "Spam", Role: api.MailboxRoleJunk, Selectable: true}}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.MailboxChanged{AccountID: a.ID})
	}); err != nil {
		t.Fatal(err)
	}
	f.until("the Junk behavior", func() bool { return !f.control(ui.RolePopUpButton, "Erase junk messages").Disabled })
	f.sees("“Spam”")
}

func TestRemovingAsksFirstAndMovesToTheFirst(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	first := f.addIMAP("first@a.test")
	second := f.addIMAP("second@a.test")
	f.sees("second@a.test")
	f.click("second@a.test")
	f.until("its form", func() bool { return f.value("Email Address:") == "second@a.test" })
	f.click("Remove Account")
	f.sees("Remove second@a.test? Its mail is deleted from this computer, not from the server.")
	if list := f.listed(); len(list) != 2 {
		t.Fatalf("removed before the answer: %+v", list)
	}
	f.press(f.control(ui.RoleButton, "Remove"))
	f.until("the first account's form", func() bool { return f.value("Email Address:") == "first@a.test" })
	if list := f.listed(); len(list) != 1 || list[0].ID != first.ID {
		t.Errorf("accounts = %+v (removed %d)", list, second.ID)
	}
	// Removed elsewhere too.
	if err := f.c.Account().Delete(t.Context(), &api.AccountDeleteParams{ID: first.ID}); err != nil {
		t.Fatal(err)
	}
	f.sees("New Account")
}

func TestAddingAnOAuthAccountSignsInOnceAfterTheServices(t *testing.T) {
	f, s := accountsFixture(t, nil)
	f.googleClient()
	f.sees("New Account")
	f.fill("Email Address:", "ann@gmail.com")
	f.click("Find Settings")
	f.sees("Found settings for this address.")
	f.click("Add This Account")
	f.until("the browser", func() bool { return len(f.wantOpened()) == 1 })
	if got, want := s.seen(), []string{"create", "setService contacts", "setService calendar", "setService tasks", "authorize"}; !slices.Equal(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
	if u := f.wantOpened()[0]; !strings.HasPrefix(u, "https://accounts.test/auth") {
		t.Errorf("opened %q", u)
	}
	f.sees("Not signed in")
}

func TestASignInWithNoClientSaysToSetOneUp(t *testing.T) {
	f, s := accountsFixture(t, nil)
	f.sees("New Account")
	f.fill("Email Address:", "ann@gmail.com")
	f.click("Find Settings")
	f.sees("Found settings for this address.")
	f.click("Add This Account")
	f.sees("Set up your Google client under Sign-In first.")
	if !slices.Contains(s.seen(), "authorize") || len(f.wantOpened()) != 0 {
		t.Errorf("calls %q, opened %q", s.seen(), f.wantOpened())
	}
}

func TestAServiceThatFailsDoesNotStopTheOthers(t *testing.T) {
	f, s := accountsFixture(t, nil)
	s.mu.Lock()
	s.failSvc[api.ServiceKindCalendar] = api.Errorf(api.CodeUnavailable, "no calendar here")
	s.services = []api.ServiceSettings{svc(api.ServiceKindContacts, nil), svc(api.ServiceKindCalendar, nil), svc(api.ServiceKindTasks, nil)}
	s.mu.Unlock()
	f.sees("New Account")
	f.fill("Email Address:", "ann@a.test")
	f.fill("IMAP Server", "imap.a.test")
	f.fill("IMAP User Name", "ann@a.test")
	f.fill("SMTP Server", "smtp.a.test")
	f.fill("SMTP User Name", "ann@a.test")
	f.check("Contacts")
	f.check("Calendars")
	f.check("Tasks")
	f.click("Add This Account")
	f.sees("Could not turn on Calendars: no calendar here")
	if got, want := s.seen(), []string{"create", "setService contacts", "setService calendar", "setService tasks"}; !slices.Equal(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
	f.sees("Keep mail from:") // it stays selected, in edit mode
}

// oauthAccount makes a Google account, not signed in.
func (f *fixture) oauthAccount(email string) *api.Account {
	f.t.Helper()
	a, err := f.c.Account().Create(f.t.Context(), &api.AccountCreateParams{Kind: api.AccountKindGmail, Email: email, Auth: api.AuthKindOAuth2})
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}

func back(t *testing.T, authURL string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("redirect_uri") + "?" + url.Values{"code": {"c0de"}, "state": {u.Query().Get("state")}}.Encode()
}

func TestFinishingASignInFromAPastedAddress(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.inLodge()
	f.googleClient()
	f.oauthAccount("ann@gmail.com")
	f.open("ann@gmail.com")
	f.sees("Not signed in")
	f.notSeen("Finish Sign-In")
	f.click("Sign In…")
	f.sees("Finish Sign-In")
	// The row shows before maild answers: wait for the page to open.
	f.until("the browser", func() bool { return len(f.wantOpened()) == 1 })
	f.sees("If your browser cannot open the page after you sign in, copy that page's address and paste it here.")
	if !f.disabled("Finish Sign-In") {
		t.Error("Finish Sign-In works with no address")
	}

	f.fill("Address", "https://nowhere.test/?code=wrong&state=wrong")
	f.click("Finish Sign-In")
	f.until("the error", func() bool { _, ok := f.tt.AccessByRole(ui.RoleAlert, ""); return ok })
	if f.value("Address") == "" {
		t.Error("the address went with the error")
	}

	f.fill("Address", back(t, f.wantOpened()[0]))
	f.click("Finish Sign-In")
	f.sees("Signed in")
	f.sees("Sign In Again…")
	f.until("the row's end", func() bool { return !f.tt.HasText("Finish Sign-In") })
}

func TestOnThisComputerASignInHasNoPasteField(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.googleClient()
	f.oauthAccount("ann@gmail.com")
	f.open("ann@gmail.com")
	f.sees("Not signed in")
	f.click("Sign In…")
	f.until("the browser", func() bool { return len(f.wantOpened()) == 1 })
	f.notSeen("Finish Sign-In")
}

// clock is a time a test moves.
type clock struct{ now atomic.Pointer[time.Time] }

func newClock(at time.Time) *clock {
	c := &clock{}
	c.now.Store(&at)
	return c
}

func (c *clock) Now() time.Time { return *c.now.Load() }

func (c *clock) Add(d time.Duration) {
	next := c.Now().Add(d)
	c.now.Store(&next)
}

func TestAWaitingSignInEnds(t *testing.T) {
	at := newClock(time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local))
	f, _ := accountsFixture(t, func(o *settings.Options) { o.Now = at.Now })
	f.inLodge()
	f.googleClient()
	a := f.oauthAccount("ann@gmail.com")
	f.addIMAP("other@a.test")
	f.sees("other@a.test")
	f.open("ann@gmail.com")
	f.until("the first account", func() bool { return f.tt.HasText("Not signed in") })

	// The row shows before maild answers, and a click waits for an answer
	// still out: each sign-in goes on once its page has opened.
	signIn := func() {
		t.Helper()
		before := len(f.wantOpened())
		f.click("Sign In…")
		f.sees("Finish Sign-In")
		f.until("the browser", func() bool { return len(f.wantOpened()) > before })
	}

	// After maild's limit.
	signIn()
	at.Add(5*time.Minute + time.Second)
	f.until("the limit", func() bool { return !f.tt.HasText("Finish Sign-In") })

	// When another account is selected.
	signIn()
	f.click("other@a.test")
	f.until("the other account", func() bool { return f.value("Email Address:") == "other@a.test" })
	f.click("ann@gmail.com")
	f.until("the first account", func() bool { return f.value("Email Address:") == "ann@gmail.com" })
	f.notSeen("Finish Sign-In")

	// When it finishes elsewhere.
	signIn()
	opened := f.wantOpened()
	if got, err := f.c.Account().FinishSignIn(t.Context(), &api.AccountFinishSignInParams{URL: back(t, opened[len(opened)-1])}); err != nil || !got.SignedIn || got.ID != a.ID {
		t.Fatalf("finishSignIn = %+v, %v", got, err)
	}
	f.until("the row's end", func() bool { return f.tt.HasText("Signed in") && !f.tt.HasText("Finish Sign-In") })
}

func TestASignInThatFailsToStartDoesNotWait(t *testing.T) {
	f, _ := accountsFixture(t, nil)
	f.inLodge()
	f.googleClient()
	f.oauthAccount("ann@gmail.com")
	f.open("ann@gmail.com")
	f.sees("Not signed in")
	f.record(func() { f.openErr = errors.New("no browser") })
	f.click("Sign In…")
	f.sees("no browser")
	f.notSeen("Finish Sign-In")
}

func svc(kind api.ServiceKind, change func(*api.ServiceSettings)) api.ServiceSettings {
	s := api.ServiceSettings{Service: kind, Available: true, SignedIn: true}
	if change != nil {
		change(&s)
	}
	return s
}

func TestServicesSayHowEachStands(t *testing.T) {
	at := newClock(time.Date(2026, 11, 10, 15, 4, 0, 0, time.Local))
	f, s := accountsFixture(t, func(o *settings.Options) { o.Now = at.Now })
	today, earlier := at.Now().Add(-time.Hour), at.Now().Add(-48*time.Hour)
	failure := "the server said no"
	s.services = []api.ServiceSettings{
		svc(api.ServiceKindContacts, func(v *api.ServiceSettings) { v.Enabled, v.LastSyncAt = true, &today }),
		svc(api.ServiceKindCalendar, func(v *api.ServiceSettings) { v.Enabled, v.LastSyncAt = true, &earlier }),
		svc(api.ServiceKindTasks, func(v *api.ServiceSettings) { v.Enabled, v.Error = true, &failure }),
	}
	f.addIMAP("me@a.test")
	f.open("me@a.test")
	f.sees("Synced at 2:04 PM")
	f.sees("Synced 11/8/26")
	f.sees(failure)

	s.mu.Lock()
	s.services = []api.ServiceSettings{
		svc(api.ServiceKindContacts, func(v *api.ServiceSettings) { v.Enabled = true }),
		svc(api.ServiceKindCalendar, func(v *api.ServiceSettings) { v.Enabled, v.SignedIn = true, false }),
		svc(api.ServiceKindTasks, func(v *api.ServiceSettings) { v.Enabled, v.SignedIn = true, false }),
	}
	s.mu.Unlock()
	name := "Ann"
	if _, err := f.c.Account().Update(t.Context(), &api.AccountUpdateParams{ID: f.listed()[0].ID, DisplayName: &name}); err != nil {
		t.Fatal(err)
	}
	f.sees("Waiting for the first sync")
	f.sees("Waiting for Google sign-in")
	f.sees("Sign in to Google to allow Calendars and Tasks.")
}

func TestServicesUnavailableSaysWhy(t *testing.T) {
	f, s := accountsFixture(t, nil)
	reason := "Apple Reminders is out of reach."
	s.services = []api.ServiceSettings{
		{Service: api.ServiceKindContacts}, {Service: api.ServiceKindCalendar}, {Service: api.ServiceKindTasks, Reason: &reason},
	}
	f.addIMAP("me@a.test")
	f.open("me@a.test")
	f.sees("This account has no contacts, calendars or tasks Frostmail can reach.")
	f.sees(reason)
}

func TestServiceToggleAndConnectWithAURL(t *testing.T) {
	f, s := accountsFixture(t, nil)
	s.services = []api.ServiceSettings{svc(api.ServiceKindContacts, nil)}
	s.failSvc[api.ServiceKindContacts] = api.Errorf(api.CodeUnavailable, "give the server's address")
	f.addIMAP("me@a.test")
	f.open("me@a.test")
	f.until("the row", func() bool { _, ok := f.tt.AccessByRole(ui.RoleCheckBox, "Contacts"); return ok })
	f.check("Contacts")
	f.sees("give the server's address")
	if !f.disabled("Connect") {
		t.Error("Connect works with no address")
	}
	if a := f.control(ui.RoleTextField, "Contacts Server"); a.Placeholder != "https://dav.example.com/" {
		t.Errorf("the placeholder is %q", a.Placeholder)
	}
	s.mu.Lock()
	delete(s.failSvc, api.ServiceKindContacts)
	s.mu.Unlock()
	f.fill("Contacts Server", " https://dav.x.test/ ")
	f.click("Connect")
	f.until("the service on", func() bool { return f.checked("Contacts") })
	f.notSeen("give the server's address")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setURL != "https://dav.x.test/" {
		t.Errorf("setService url = %q", s.setURL)
	}
}

func TestServicesSignInGoesToTheBrowser(t *testing.T) {
	f, s := accountsFixture(t, nil)
	s.services = []api.ServiceSettings{svc(api.ServiceKindContacts, func(v *api.ServiceSettings) { v.Enabled, v.SignedIn = true, false })}
	f.googleClient()
	f.oauthAccount("ann@gmail.com")
	f.open("ann@gmail.com")
	f.sees("Sign in to Google to allow Contacts.")
	f.press(signInForServices(f))
	f.until("the browser", func() bool { return len(f.wantOpened()) == 1 })
}

// signInForServices is the services' Sign In…, the last of its name.
func signInForServices(f *fixture) ui.AccessInfo {
	f.t.Helper()
	a := f.control(ui.RoleButton, "Sign In…")
	box, ok := f.tt.Find("Sign in to Google to allow Contacts.")
	if !ok {
		f.t.Fatal("no prompt")
	}
	a.Bounds.Y = box.Y // the button is on the prompt's row
	a.Bounds.H = box.H
	a.Bounds.X = box.X + box.W + 12
	a.Bounds.W = 40
	return a
}
