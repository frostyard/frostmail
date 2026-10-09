package oauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

var (
	// ErrNotOAuth means the account does not sign in with OAuth.
	ErrNotOAuth = errors.New("oauth: the account does not sign in with OAuth")
	// ErrNoClient means no OAuth client is set for the account's provider.
	ErrNoClient = errors.New("oauth: no OAuth client is set")
	// ErrReauth means the provider refused the stored grant, or there is
	// none: the user must sign in again.
	ErrReauth = errors.New("oauth: sign in again")
)

// Manager signs accounts in (the authorization-code flow with PKCE and a
// loopback redirect) and keeps their access tokens fresh
// (docs/design/accounts.md, Sign-in). Access tokens live only in memory;
// refresh tokens are in the secret store.
type Manager struct {
	DB      *store.DB
	Secrets secrets.Store
	Log     *slog.Logger
	// HTTP talks to token endpoints; nil means a client with a 30 s
	// timeout.
	HTTP *http.Client
	// Endpoints by provider; nil means Google's.
	Endpoints map[string]Endpoint
	// SignedIn runs after a sign-in stored its token (maild reconnects the
	// account).
	SignedIn func(accountID int64)
	// FlowTimeout bounds a sign-in [5m].
	FlowTimeout time.Duration
	// Now and Rand are the clock and randomness; nil means the real ones.
	Now  func() time.Time
	Rand io.Reader

	mu         sync.Mutex
	tokens     map[int64]Token
	flows      map[int64]context.CancelFunc
	refreshing map[int64]*sync.Mutex
}

const refreshMargin = 5 * time.Minute

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) rand() io.Reader {
	if m.Rand != nil {
		return m.Rand
	}
	return rand.Reader
}

func (m *Manager) httpClient() *http.Client {
	if m.HTTP != nil {
		return m.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (m *Manager) endpoint(provider string) (Endpoint, bool) {
	if m.Endpoints != nil {
		e, ok := m.Endpoints[provider]
		return e, ok
	}
	if provider == "google" {
		return Google, true
	}
	return Endpoint{}, false
}

// creds are what a flow or a refresh needs about an account.
type creds struct {
	acct     store.Account
	endpoint Endpoint
	clientID string
	secret   string
}

func (m *Manager) creds(ctx context.Context, accountID int64) (creds, error) {
	acct, err := m.DB.GetAccount(ctx, accountID)
	if err != nil {
		return creds{}, err
	}
	if acct.Auth != api.AuthKindOAuth2 {
		return creds{}, ErrNotOAuth
	}
	provider := providers.ForKind(acct.Kind).OAuth
	e, ok := m.endpoint(provider)
	if !ok {
		return creds{}, fmt.Errorf("%w: %s accounts have no OAuth provider", ErrNoClient, acct.Kind)
	}
	id, err := m.DB.OAuthClientID(ctx, provider)
	if errors.Is(err, store.ErrNotFound) {
		return creds{}, fmt.Errorf("%w for %s", ErrNoClient, provider)
	}
	if err != nil {
		return creds{}, err
	}
	secret, err := m.Secrets.Get(ctx, secrets.OAuthClientSecret(provider))
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return creds{}, err
	}
	return creds{acct: acct, endpoint: e, clientID: id, secret: secret}, nil
}

// Authorize starts a sign-in and returns the URL for the browser. maild
// listens on a random loopback port for the browser's return, for one
// request with the right state, until FlowTimeout; a new sign-in for the
// same account cancels the previous one.
func (m *Manager) Authorize(ctx context.Context, accountID int64) (string, error) {
	c, err := m.creds(ctx, accountID)
	if err != nil {
		return "", err
	}
	scopes, err := m.scopes(ctx, c)
	if err != nil {
		return "", err
	}
	verifier, err := NewVerifier(m.rand())
	if err != nil {
		return "", err
	}
	stateBytes := make([]byte, 16)
	if _, err := io.ReadFull(m.rand(), stateBytes); err != nil {
		return "", fmt.Errorf("oauth: state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("oauth: listen for the redirect: %w", err)
	}
	redirect := fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)

	timeout := m.FlowTimeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	m.mu.Lock()
	if m.flows == nil {
		m.flows = map[int64]context.CancelFunc{}
	}
	if old := m.flows[accountID]; old != nil {
		old()
	}
	m.flows[accountID] = cancel
	m.mu.Unlock()

	f := &flow{m: m, c: c, state: state, verifier: verifier, redirect: redirect, scopes: scopes, done: make(chan struct{})}
	srv := &http.Server{Handler: f, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	go func() {
		select {
		case <-fctx.Done():
			_ = srv.Close()
		case <-f.done:
			// Shut down gracefully: Close could drop the connection before
			// the browser has the page that says the sign-in worked.
			sctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			_ = srv.Shutdown(sctx)
			stop()
		}
		cancel()
	}()
	e := c.endpoint
	e.Scopes = scopes
	return AuthURL(e, c.clientID, redirect, state, Challenge(verifier), c.acct.Email), nil
}

// scopes are what a sign-in asks for: the endpoint's (mail), and for a
// Google account the scopes of its services that are on (ADR-0017).
func (m *Manager) scopes(ctx context.Context, c creds) ([]string, error) {
	out := slices.Clone(c.endpoint.Scopes)
	if providers.ForKind(c.acct.Kind).OAuth != "google" {
		return out, nil
	}
	services, err := m.DB.Services(ctx, c.acct.ID)
	if err != nil {
		return nil, err
	}
	for _, s := range services {
		if scope := GoogleScope(s.Service); s.Enabled && scope != "" {
			out = append(out, scope)
		}
	}
	return out, nil
}

// GoogleScope is the Google scope a service needs.
func GoogleScope(service api.ServiceKind) string {
	switch service {
	case api.ServiceKindContacts:
		return GoogleContactsScope
	case api.ServiceKindCalendar:
		return GoogleCalendarScope
	case api.ServiceKindTasks:
		return GoogleTasksScope
	}
	return ""
}

// flow is one sign-in waiting for the browser.
type flow struct {
	m                         *Manager
	c                         creds
	state, verifier, redirect string
	scopes                    []string // what the sign-in asked for
	once                      sync.Once
	done                      chan struct{}
}

func (f *flow) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.URL.Path != "/" || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(f.state)) != 1 {
		http.Error(w, "This is not the sign-in Frostmail is waiting for.", http.StatusBadRequest)
		return
	}
	finished := false
	f.once.Do(func() {
		finished = true
		defer close(f.done)
		if e := q.Get("error"); e != "" {
			f.m.Log.Warn("sign-in refused", "account", f.c.acct.ID, "error", e)
			page(w, "Sign-in was not completed ("+e+"). You can close this tab and try again from Frostmail.")
			return
		}
		if err := f.finish(r.Context(), q.Get("code")); err != nil {
			f.m.Log.Warn("sign-in failed", "account", f.c.acct.ID, "err", err)
			page(w, "Sign-in failed: "+err.Error())
			return
		}
		page(w, "Frostmail is signed in to "+f.c.acct.Email+". You can close this tab.")
	})
	if !finished {
		http.Error(w, "This sign-in is already finished.", http.StatusGone)
	}
}

func page(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	_, _ = io.WriteString(w, "<!DOCTYPE html><title>Frostmail</title><p style=\"font: 15px sans-serif; margin: 3em\">"+
		html.EscapeString(text)+"</p>")
}

// finish exchanges the code and stores the grant.
func (f *flow) finish(ctx context.Context, code string) error {
	if code == "" {
		return errors.New("the browser returned no code")
	}
	m, c := f.m, f.c
	tok, err := m.exchange(ctx, c, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {f.redirect},
		"code_verifier": {f.verifier},
	})
	if err != nil {
		return err
	}
	if tok.Refresh == "" {
		return errors.New("the provider granted no refresh token")
	}
	if err := m.Secrets.Set(ctx, secrets.RefreshToken(c.acct.ID), tok.Refresh); err != nil {
		return err
	}
	m.remember(c.acct.ID, tok)
	granted := tok.Scope
	if granted == "" { // the provider granted what was asked
		granted = strings.Join(f.scopes, " ")
	}
	err = m.DB.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetNeedsReauth(ctx, c.acct.ID, false); err != nil {
			return err
		}
		return tx.SetGrantedScopes(ctx, c.acct.ID, granted)
	})
	if err != nil {
		return err
	}
	if m.SignedIn != nil {
		m.SignedIn(c.acct.ID)
	}
	return nil
}

// exchange posts a token request with the client's credentials.
func (m *Manager) exchange(ctx context.Context, c creds, form url.Values) (Token, error) {
	form.Set("client_id", c.clientID)
	if c.secret != "" {
		form.Set("client_secret", c.secret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := m.httpClient().Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("oauth: token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Token{}, fmt.Errorf("oauth: token response: %w", err)
	}
	return ParseToken(body, resp.StatusCode, m.now())
}

// remember caches an access token; a token without a lifetime is kept for
// 50 minutes.
func (m *Manager) remember(accountID int64, tok Token) {
	if tok.Expiry.IsZero() {
		tok.Expiry = m.now().Add(50 * time.Minute)
	}
	tok.Refresh = ""
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tokens == nil {
		m.tokens = map[int64]Token{}
	}
	m.tokens[accountID] = tok
}

// AccessToken returns a token valid for at least 5 more minutes,
// refreshing it when needed. It returns ErrReauth when there is no grant
// or the provider refuses it (and marks the account so).
func (m *Manager) AccessToken(ctx context.Context, accountID int64) (string, error) {
	if tok, ok := m.cached(accountID); ok {
		return tok, nil
	}
	lock := m.refreshLock(accountID)
	lock.Lock()
	defer lock.Unlock()
	if tok, ok := m.cached(accountID); ok { // refreshed while we waited
		return tok, nil
	}
	c, err := m.creds(ctx, accountID)
	if err != nil {
		return "", err
	}
	refresh, err := m.Secrets.Get(ctx, secrets.RefreshToken(accountID))
	if errors.Is(err, secrets.ErrNotFound) {
		return "", m.needsReauth(ctx, accountID, ErrReauth)
	}
	if err != nil {
		return "", err
	}
	tok, err := m.exchange(ctx, c, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	var te *TokenError
	if errors.As(err, &te) && te.Code == "invalid_grant" {
		return "", m.needsReauth(ctx, accountID, fmt.Errorf("%w: %w", ErrReauth, err))
	}
	if err != nil {
		return "", err
	}
	if tok.Refresh != "" && tok.Refresh != refresh {
		if err := m.Secrets.Set(ctx, secrets.RefreshToken(accountID), tok.Refresh); err != nil {
			return "", err
		}
	}
	m.remember(accountID, tok)
	return tok.Access, nil
}

func (m *Manager) cached(accountID int64) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tok, ok := m.tokens[accountID]
	if !ok || tok.Expiry.Sub(m.now()) < refreshMargin {
		return "", false
	}
	return tok.Access, true
}

func (m *Manager) refreshLock(accountID int64) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.refreshing == nil {
		m.refreshing = map[int64]*sync.Mutex{}
	}
	l := m.refreshing[accountID]
	if l == nil {
		l = &sync.Mutex{}
		m.refreshing[accountID] = l
	}
	return l
}

// needsReauth marks the account and returns err.
func (m *Manager) needsReauth(ctx context.Context, accountID int64, err error) error {
	if txErr := m.DB.Tx(ctx, func(tx *store.Tx) error { return tx.SetNeedsReauth(ctx, accountID, true) }); txErr != nil {
		return errors.Join(err, txErr)
	}
	return err
}

// Invalidate drops an account's cached access token (the server refused
// it), so the next AccessToken refreshes.
func (m *Manager) Invalidate(accountID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, accountID)
}
