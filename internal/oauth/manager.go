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
	// ErrNoScope means the grant covers no scope of the audience asked
	// for: the user signs in again with the service on.
	ErrNoScope = errors.New("oauth: the sign-in covers no scope for this service")
	// ErrNoSignIn means no waiting sign-in has the state of the address
	// given to Finish: it is unknown, finished or expired.
	ErrNoSignIn = errors.New("oauth: no sign-in waits for this address")
	// ErrRefused means the provider refused the sign-in: the address
	// carries its error value.
	ErrRefused = errors.New("oauth: the provider refused the sign-in")
	// ErrNoCode means the address carries neither a code nor an error.
	ErrNoCode = errors.New("oauth: the browser returned no code")

	// errFlowFinished means the sign-in ended before this request.
	errFlowFinished = errors.New("oauth: this sign-in is already finished")
)

// tokenKey names a cached access token: an account's, for an audience
// ("" for mail's).
type tokenKey struct {
	account  int64
	audience string
}

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
	// Endpoints by provider; nil means Google's and Microsoft's.
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
	tokens     map[tokenKey]Token
	flows      map[int64]*flow  // the waiting sign-in of each account
	byState    map[string]*flow // the same waiting sign-ins, by state
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
	switch provider {
	case "google":
		return Google, true
	case "microsoft":
		return Microsoft, true
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
	if errors.Is(err, store.ErrNotFound) && e.ClientID != "" {
		id, err = e.ClientID, nil // the provider's built-in client
	}
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
// same account cancels the previous one. A sign-in also ends through
// Finish, when its browser cannot reach maild's loopback (ADR-0043).
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
	lns, redirect, err := listenLoopback(c.endpoint.Loopback)
	if err != nil {
		return "", err
	}

	timeout := m.FlowTimeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	f := &flow{m: m, c: c, lns: lns, state: state, verifier: verifier, redirect: redirect, scopes: scopes,
		ctx: fctx, cancel: cancel, done: make(chan struct{})}
	m.mu.Lock()
	if m.flows == nil {
		m.flows = map[int64]*flow{}
	}
	if m.byState == nil {
		m.byState = map[string]*flow{}
	}
	if old := m.flows[accountID]; old != nil {
		old.cancel()
	}
	m.flows[accountID] = f
	m.byState[state] = f
	m.mu.Unlock()
	srv := &http.Server{Handler: f, ReadHeaderTimeout: 10 * time.Second}
	for _, ln := range lns {
		go func() { _ = srv.Serve(ln) }()
	}
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
		m.removeFlow(f)
	}()
	e := c.endpoint
	e.Scopes = scopes
	return AuthURL(e, c.clientID, redirect, state, Challenge(verifier), c.acct.Email), nil
}

// listenLoopback listens for a sign-in's redirect on a random loopback
// port and returns the redirect URI. "localhost" (Microsoft's
// registration) listens on IPv4 and, when it can, on the same port over
// IPv6, since a browser may try either; anything else is 127.0.0.1.
func listenLoopback(host string) ([]net.Listener, string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("oauth: listen for the redirect: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if host != "localhost" {
		return []net.Listener{ln}, fmt.Sprintf("http://127.0.0.1:%d/", port), nil
	}
	lns := []net.Listener{ln}
	if ln6, err := net.Listen("tcp", fmt.Sprintf("[::1]:%d", port)); err == nil {
		lns = append(lns, ln6)
	}
	return lns, fmt.Sprintf("http://localhost:%d", port), nil
}

// scopes are what a sign-in asks for: the endpoint's (mail), and the
// scopes of the account's services that are on (Google's, ADR-0017, and
// Microsoft Graph's, ADR-0039).
func (m *Manager) scopes(ctx context.Context, c creds) ([]string, error) {
	out := slices.Clone(c.endpoint.Scopes)
	provider := providers.ForKind(c.acct.Kind).OAuth
	if provider != "google" && provider != "microsoft" {
		return out, nil
	}
	services, err := m.DB.Services(ctx, c.acct.ID)
	if err != nil {
		return nil, err
	}
	for _, s := range services {
		if scope := ServiceScope(provider, s.Service); s.Enabled && scope != "" {
			out = append(out, scope)
		}
	}
	return out, nil
}

// ServiceScope is the scope a provider's service needs ("" for none).
func ServiceScope(provider string, service api.ServiceKind) string {
	switch provider {
	case "google":
		return GoogleScope(service)
	case "microsoft":
		switch service {
		case api.ServiceKindContacts:
			return MicrosoftContactsScope
		case api.ServiceKindCalendar:
			return MicrosoftCalendarScope
		}
	}
	return ""
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
	lns                       []net.Listener // the redirect's port
	state, verifier, redirect string
	scopes                    []string // what the sign-in asked for
	ctx                       context.Context
	cancel                    context.CancelFunc
	once                      sync.Once
	done                      chan struct{}
}

func (f *flow) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.URL.Path != "/" || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(f.state)) != 1 {
		http.Error(w, "This is not the sign-in Frostmail is waiting for.", http.StatusBadRequest)
		return
	}
	w.Header().Set("Connection", "close")
	switch err := f.complete(r.Context(), q); {
	case errors.Is(err, errFlowFinished):
		http.Error(w, "This sign-in is already finished.", http.StatusGone)
	case errors.Is(err, ErrRefused):
		// The flow ends with this answer: the port stops answering before
		// the browser has the page, and this connection closes after it.
		page(w, "Sign-in was not completed ("+q.Get("error")+"). You can close this tab and try again from Frostmail.")
	case err != nil:
		page(w, "Sign-in failed: "+err.Error())
	default:
		page(w, "Frostmail is signed in to "+f.c.acct.Email+". You can close this tab.")
	}
}

// complete ends the sign-in once: it closes the listeners, refuses it when
// the query holds the provider's error, or exchanges the code and stores
// the grant. A second call returns errFlowFinished.
func (f *flow) complete(ctx context.Context, q url.Values) error {
	finished := false
	var err error
	f.once.Do(func() {
		finished = true
		defer close(f.done)
		for _, ln := range f.lns {
			_ = ln.Close()
		}
		f.m.removeFlow(f)
		if e := q.Get("error"); e != "" {
			f.m.Log.Warn("sign-in refused", "account", f.c.acct.ID, "error", e)
			err = fmt.Errorf("%w: %s", ErrRefused, e)
			return
		}
		err = f.finish(ctx, q.Get("code"))
		if err != nil {
			f.m.Log.Warn("sign-in failed", "account", f.c.acct.ID, "err", err)
		}
	})
	if !finished {
		return errFlowFinished
	}
	return err
}

// removeFlow forgets a flow that ended, or that a new sign-in replaced.
func (m *Manager) removeFlow(f *flow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.flows[f.c.acct.ID] == f {
		delete(m.flows, f.c.acct.ID)
	}
	if m.byState[f.state] == f {
		delete(m.byState, f.state)
	}
}

// Finish ends the waiting sign-in whose state is in the address that the
// provider sent the browser to, and stores its token as the loopback
// listener would (ADR-0043). The address must have the waiting sign-in's
// redirect as its scheme, host, port and path. Finish never logs the code
// or the address.
func (m *Manager) Finish(ctx context.Context, rawURL string) (int64, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return 0, ErrNoSignIn
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, ErrNoSignIn
	}
	state := u.Query().Get("state")
	if state == "" {
		return 0, ErrNoSignIn
	}
	m.mu.Lock()
	var f *flow
	for _, candidate := range m.byState {
		if subtle.ConstantTimeCompare([]byte(state), []byte(candidate.state)) == 1 {
			f = candidate
			break
		}
	}
	m.mu.Unlock()
	if f == nil || f.ctx.Err() != nil || !sameRedirect(u, f.redirect) {
		return 0, ErrNoSignIn
	}
	err = f.complete(ctx, u.Query())
	if errors.Is(err, errFlowFinished) {
		return 0, ErrNoSignIn
	}
	if err != nil {
		return 0, err
	}
	return f.c.acct.ID, nil
}

// sameRedirect reports whether u has the scheme, host, port and path of a
// redirect URI. A path of "" and "/" are equal, as the browser shows them.
func sameRedirect(u *url.URL, redirect string) bool {
	r, err := url.Parse(redirect)
	if err != nil {
		return false
	}
	path := func(p string) string {
		if p == "" {
			return "/"
		}
		return p
	}
	return strings.EqualFold(u.Scheme, r.Scheme) &&
		strings.EqualFold(u.Hostname(), r.Hostname()) &&
		u.Port() == r.Port() &&
		path(u.Path) == path(r.Path)
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
		return ErrNoCode
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
	m.remember(tokenKey{account: c.acct.ID}, tok)
	granted := tok.Scope
	if granted == "" || len(c.endpoint.Audiences) > 0 {
		// The provider granted what was asked; Microsoft names only the
		// first audience's scopes, but the consent covered them all.
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
	if len(c.endpoint.TokenScopes) > 0 && !form.Has("scope") {
		form.Set("scope", strings.Join(c.endpoint.TokenScopes, " "))
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
func (m *Manager) remember(key tokenKey, tok Token) {
	if tok.Expiry.IsZero() {
		tok.Expiry = m.now().Add(50 * time.Minute)
	}
	tok.Refresh = ""
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tokens == nil {
		m.tokens = map[tokenKey]Token{}
	}
	m.tokens[key] = tok
}

// AccessToken returns a token for mail valid for at least 5 more minutes,
// refreshing it when needed. It returns ErrReauth when there is no grant
// or the provider refuses it (and marks the account so).
func (m *Manager) AccessToken(ctx context.Context, accountID int64) (string, error) {
	return m.AccessTokenFor(ctx, accountID, "")
}

// AccessTokenFor is AccessToken for one of the provider's audiences besides
// mail's (AudienceGraph), from the same refresh token; "" is mail's. A
// provider with one audience (Google) gives its token for any. The lock
// is the account's, so two audiences never redeem a rotating refresh
// token at once. ErrNoScope means the grant has no scope of the audience.
func (m *Manager) AccessTokenFor(ctx context.Context, accountID int64, audience string) (string, error) {
	if tok, ok := m.cached(tokenKey{account: accountID, audience: audience}); ok {
		return tok, nil
	}
	c, err := m.creds(ctx, accountID)
	if err != nil {
		return "", err
	}
	prefix, multi := c.endpoint.Audiences[audience]
	if !multi {
		audience = ""
	}
	key := tokenKey{account: accountID, audience: audience}
	if tok, ok := m.cached(key); ok {
		return tok, nil
	}
	lock := m.refreshLock(accountID)
	lock.Lock()
	defer lock.Unlock()
	if tok, ok := m.cached(key); ok { // refreshed while we waited
		return tok, nil
	}
	form := url.Values{"grant_type": {"refresh_token"}}
	if multi {
		var scopes []string
		for _, s := range strings.Fields(c.acct.GrantedScopes) {
			if strings.HasPrefix(s, prefix) {
				scopes = append(scopes, s)
			}
		}
		if len(scopes) == 0 {
			return "", fmt.Errorf("%w (%s)", ErrNoScope, audience)
		}
		form.Set("scope", strings.Join(append(scopes, "offline_access"), " "))
	}
	refresh, err := m.Secrets.Get(ctx, secrets.RefreshToken(accountID))
	if errors.Is(err, secrets.ErrNotFound) {
		return "", m.needsReauth(ctx, accountID, ErrReauth)
	}
	if err != nil {
		return "", err
	}
	form.Set("refresh_token", refresh)
	tok, err := m.exchange(ctx, c, form)
	var te *TokenError
	if multi && errors.As(err, &te) && strings.Contains(te.Description, "AADSTS65001") {
		// No consent for this audience's scopes: the service waits for a
		// sign-in, and mail goes on.
		return "", fmt.Errorf("%w (%s): %w", ErrNoScope, audience, err)
	}
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
	m.remember(key, tok)
	return tok.Access, nil
}

func (m *Manager) cached(key tokenKey) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tok, ok := m.tokens[key]
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

// Invalidate drops an account's cached access tokens (a server refused
// one), so the next AccessToken or AccessTokenFor refreshes.
func (m *Manager) Invalidate(accountID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.tokens {
		if key.account == accountID {
			delete(m.tokens, key)
		}
	}
}
