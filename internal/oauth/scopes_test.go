package oauth

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// TestAuthorizeAsksForServiceScopes: a Google sign-in asks for mail and the
// scopes of the services that are on, and records what was granted.
func TestAuthorizeAsksForServiceScopes(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	err := e.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetService(ctx, e.acct, api.ServiceKindContacts, true, "https://dav.test/"); err != nil {
			return err
		}
		if err := tx.SetService(ctx, e.acct, api.ServiceKindTasks, true, "https://tasks.test/"); err != nil {
			return err
		}
		return tx.SetService(ctx, e.acct, api.ServiceKindCalendar, false, "https://dav.test/")
	})
	if err != nil {
		t.Fatal(err)
	}
	authURL, err := e.m.Authorize(ctx, e.acct)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	want := strings.Join([]string{Google.Scopes[0], GoogleContactsScope, GoogleTasksScope}, " ")
	if got := u.Query().Get("scope"); got != want {
		t.Errorf("scope = %q, want %q", got, want)
	}
	if status, _ := e.browse(t, authURL, "the-code", ""); status != http.StatusOK {
		t.Fatalf("return: %d", status)
	}
	select {
	case <-e.signedIn:
	case <-time.After(5 * time.Second):
		t.Fatal("SignedIn was not called")
	}
	acct, _ := e.db.GetAccount(ctx, e.acct)
	if !acct.HasScope(GoogleContactsScope) || !acct.HasScope(GoogleTasksScope) || acct.HasScope(GoogleCalendarScope) {
		t.Errorf("granted scopes = %q", acct.GrantedScopes)
	}
}
