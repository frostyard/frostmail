package oauth

import (
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// fakeProvider is a token endpoint that checks PKCE and the client.
type fakeProvider struct {
	mu        sync.Mutex
	challenge string // the code_challenge the browser carried
	refreshes int
	revoked   bool
	srv       *httptest.Server
}

func (p *fakeProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	p.mu.Lock()
	defer p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Form.Get("client_id") != "cid" || r.Form.Get("client_secret") != "csecret" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		if r.Form.Get("code") != "the-code" || Challenge(r.Form.Get("code_verifier")) != p.challenge {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"bad code or verifier"}`)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"access-1","expires_in":3600,"refresh_token":"refresh-1","token_type":"Bearer"}`)
	case "refresh_token":
		if p.revoked || r.Form.Get("refresh_token") != "refresh-1" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`)
			return
		}
		p.refreshes++
		b, _ := json.Marshal(map[string]any{"access_token": "access-r" + string(rune('0'+p.refreshes)), "expires_in": 3600})
		_, _ = w.Write(b)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

type env struct {
	m        *Manager
	db       *store.DB
	sec      *secrets.File
	provider *fakeProvider
	acct     int64
	clock    time.Time
	signedIn chan int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	sec := secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json"))
	p := &fakeProvider{}
	p.srv = httptest.NewServer(p)
	t.Cleanup(p.srv.Close)
	e := &env{db: db, sec: sec, provider: p, clock: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC), signedIn: make(chan int64, 1)}
	e.m = &Manager{
		DB: db, Secrets: sec, Log: slog.New(slog.DiscardHandler),
		Endpoints: map[string]Endpoint{"google": {AuthURL: "https://accounts.test/auth", TokenURL: p.srv.URL, Scopes: Google.Scopes}},
		SignedIn:  func(id int64) { e.signedIn <- id },
		Now:       func() time.Time { return e.clock },
	}
	err = db.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{
			Kind: api.AccountKindGmail, Email: "ann@gmail.com", Auth: api.AuthKindOAuth2,
			IMAP: store.ServerConfig{Host: "imap.gmail.com", Port: 993, TLS: api.TLSModeTLS, Username: "ann@gmail.com"},
			SMTP: store.ServerConfig{Host: "smtp.gmail.com", Port: 465, TLS: api.TLSModeTLS, Username: "ann@gmail.com"},
		})
		e.acct = a.ID
		if err != nil {
			return err
		}
		return tx.SetOAuthClientID(ctx, "google", "cid")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sec.Set(ctx, secrets.OAuthClientSecret("google"), "csecret"); err != nil {
		t.Fatal(err)
	}
	return e
}

// browse follows an authorization URL back to maild as the provider would
// redirect the browser, with the given code and state.
func (e *env) browse(t *testing.T, authURL, code, state string) (int, string) {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	e.provider.mu.Lock()
	e.provider.challenge = q.Get("code_challenge")
	e.provider.mu.Unlock()
	if state == "" {
		state = q.Get("state")
	}
	back := q.Get("redirect_uri") + "?" + url.Values{"code": {code}, "state": {state}}.Encode()
	resp, err := http.Get(back)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestAuthorizeSignsIn(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	authURL, err := e.m.Authorize(ctx, e.acct)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	q := u.Query()
	if !strings.HasPrefix(authURL, "https://accounts.test/auth?") || q.Get("client_id") != "cid" ||
		q.Get("login_hint") != "ann@gmail.com" || !strings.HasPrefix(q.Get("redirect_uri"), "http://127.0.0.1:") {
		t.Fatalf("auth URL = %s", authURL)
	}

	// A request with another state is turned away and the flow keeps waiting.
	if status, _ := e.browse(t, authURL, "the-code", "forged"); status != http.StatusBadRequest {
		t.Fatalf("forged state: status %d", status)
	}
	status, body := e.browse(t, authURL, "the-code", "")
	if status != http.StatusOK || !strings.Contains(body, "signed in to ann@gmail.com") {
		t.Fatalf("return: %d %s", status, body)
	}
	select {
	case id := <-e.signedIn:
		if id != e.acct {
			t.Errorf("signed in %d", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SignedIn was not called")
	}
	if rt, err := e.sec.Get(ctx, secrets.RefreshToken(e.acct)); err != nil || rt != "refresh-1" {
		t.Fatalf("refresh token = %q, %v", rt, err)
	}
	if tok, err := e.m.AccessToken(ctx, e.acct); err != nil || tok != "access-1" {
		t.Fatalf("access token = %q, %v", tok, err)
	}
	if e.provider.refreshes != 0 {
		t.Error("a fresh token was refreshed")
	}
	// The flow ended: the port no longer answers.
	if _, err := http.Get(q.Get("redirect_uri")); err == nil {
		t.Error("the loopback listener is still open")
	}
}

func TestAuthorizeRefused(t *testing.T) {
	e := newEnv(t)
	authURL, err := e.m.Authorize(t.Context(), e.acct)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	back := u.Query().Get("redirect_uri") + "?" + url.Values{"error": {"access_denied"}, "state": {u.Query().Get("state")}}.Encode()
	resp, err := http.Get(back)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "access_denied") {
		t.Errorf("page = %s", body)
	}
	if _, err := e.sec.Get(t.Context(), secrets.RefreshToken(e.acct)); !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("a refused sign-in stored a token: %v", err)
	}
}

func TestAccessTokenRefreshesAndMarksRevokedGrants(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	if _, err := e.m.AccessToken(ctx, e.acct); !errors.Is(err, ErrReauth) {
		t.Fatalf("no grant = %v", err)
	}
	if acct, _ := e.db.GetAccount(ctx, e.acct); !acct.NeedsReauth {
		t.Error("the account was not marked")
	}
	if err := e.sec.Set(ctx, secrets.RefreshToken(e.acct), "refresh-1"); err != nil {
		t.Fatal(err)
	}
	tok, err := e.m.AccessToken(ctx, e.acct)
	if err != nil || tok != "access-r1" {
		t.Fatalf("refreshed = %q, %v", tok, err)
	}
	e.clock = e.clock.Add(50 * time.Minute) // 10 minutes left: still fine
	if tok, _ := e.m.AccessToken(ctx, e.acct); tok != "access-r1" {
		t.Errorf("token with 10 minutes left was refreshed: %q", tok)
	}
	e.clock = e.clock.Add(6 * time.Minute) // 4 minutes left: refresh
	if tok, _ := e.m.AccessToken(ctx, e.acct); tok != "access-r2" {
		t.Errorf("token with 4 minutes left = %q", tok)
	}
	e.m.Invalidate(e.acct)
	if tok, _ := e.m.AccessToken(ctx, e.acct); tok != "access-r3" {
		t.Errorf("after Invalidate = %q", tok)
	}

	e.provider.mu.Lock()
	e.provider.revoked = true
	e.provider.mu.Unlock()
	e.m.Invalidate(e.acct)
	_, err = e.m.AccessToken(ctx, e.acct)
	var te *TokenError
	if !errors.Is(err, ErrReauth) || !errors.As(err, &te) || te.Code != "invalid_grant" {
		t.Fatalf("revoked = %v", err)
	}
}

func TestAuthorizeNeedsAnOAuthAccountAndAClient(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	var pw int64
	err := e.db.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{
			Kind: api.AccountKindICloud, Email: "bob@icloud.com", Auth: api.AuthKindPassword,
			IMAP: store.ServerConfig{Host: "i", Port: 993, TLS: api.TLSModeTLS, Username: "b"},
			SMTP: store.ServerConfig{Host: "s", Port: 587, TLS: api.TLSModeStartTLS, Username: "b"},
		})
		pw = a.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.m.Authorize(ctx, pw); !errors.Is(err, ErrNotOAuth) {
		t.Errorf("password account = %v", err)
	}
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM oauth_clients`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.m.Authorize(ctx, e.acct); !errors.Is(err, ErrNoClient) {
		t.Errorf("no client = %v", err)
	}
}
