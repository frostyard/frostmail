package oauth

import (
	"encoding/json/v2"
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

// fakeMicrosoft is a token endpoint as Microsoft's behaves (probed
// 2026-10-15): a public client with no secret, every request naming its
// audience's scopes, and a new refresh token on every redemption.
type fakeMicrosoft struct {
	t         *testing.T
	mu        sync.Mutex
	challenge string
	refresh   string // the one that is current
	issued    int
}

func (p *fakeMicrosoft) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	p.mu.Lock()
	defer p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fail := func(code string) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"`+code+`"}`)
	}
	if r.Form.Get("client_id") != MicrosoftClientID || r.Form.Has("client_secret") {
		fail("invalid_client")
		return
	}
	if r.Form.Get("scope") != strings.Join(Microsoft.TokenScopes, " ") {
		p.t.Errorf("token request scope = %q", r.Form.Get("scope"))
		fail("invalid_scope")
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		if r.Form.Get("code") != "the-code" || Challenge(r.Form.Get("code_verifier")) != p.challenge ||
			!strings.HasPrefix(r.Form.Get("redirect_uri"), "http://localhost:") {
			fail("invalid_grant")
			return
		}
	case "refresh_token":
		if r.Form.Get("refresh_token") != p.refresh {
			fail("invalid_grant")
			return
		}
	default:
		fail("unsupported_grant_type")
		return
	}
	p.issued++
	p.refresh = "rt-" + string(rune('0'+p.issued))
	b, _ := json.Marshal(map[string]any{"access_token": "at-" + string(rune('0'+p.issued)), "expires_in": 3600,
		"refresh_token": p.refresh, "scope": strings.Join(Microsoft.Scopes[:2], " "), "token_type": "Bearer"})
	_, _ = w.Write(b)
}

func TestMicrosoftSignInWithTheBuiltInClient(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	sec := secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json"))
	p := &fakeMicrosoft{t: t}
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)
	ms := Microsoft
	ms.TokenURL = srv.URL
	clock := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	signedIn := make(chan int64, 1)
	m := &Manager{DB: db, Secrets: sec, Log: slog.New(slog.DiscardHandler),
		Endpoints: map[string]Endpoint{"microsoft": ms},
		SignedIn:  func(id int64) { signedIn <- id },
		Now:       func() time.Time { return clock }}
	var acct int64
	if err := db.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindMicrosoft, Email: "cy@hotmail.com", Auth: api.AuthKindOAuth2,
			IMAP: store.ServerConfig{Host: "outlook.office365.com", Port: 993, TLS: api.TLSModeTLS, Username: "cy@hotmail.com"},
			SMTP: store.ServerConfig{Host: "smtp-mail.outlook.com", Port: 587, TLS: api.TLSModeStartTLS, Username: "cy@hotmail.com"}})
		acct = a.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := m.Authorize(ctx, acct)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(raw)
	q := u.Query()
	if u.Host != "login.microsoftonline.com" || q.Get("client_id") != MicrosoftClientID || q.Get("prompt") != "select_account" ||
		q.Has("access_type") || q.Get("login_hint") != "cy@hotmail.com" ||
		q.Get("scope") != MicrosoftIMAPScope+" "+MicrosoftSMTPScope+" offline_access" {
		t.Errorf("authorization URL = %s", raw)
	}
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || redirect.Scheme != "http" || redirect.Hostname() != "localhost" || redirect.Path != "" {
		t.Fatalf("redirect = %q; Microsoft's client is registered with http://localhost", q.Get("redirect_uri"))
	}
	p.mu.Lock()
	p.challenge = q.Get("code_challenge")
	p.mu.Unlock()
	// The browser may reach localhost over IPv4 or IPv6.
	back := "http://127.0.0.1:" + redirect.Port() + "/?" + url.Values{"code": {"the-code"}, "state": {q.Get("state")}}.Encode()
	resp, err := http.Get(back)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "signed in to cy@hotmail.com") {
		t.Fatalf("redirect answered %d: %s", resp.StatusCode, body)
	}
	<-signedIn
	if rt, _ := sec.Get(ctx, secrets.RefreshToken(acct)); rt != "rt-1" {
		t.Fatalf("stored refresh token = %q", rt)
	}
	if tok, err := m.AccessToken(ctx, acct); err != nil || tok != "at-1" {
		t.Fatalf("AccessToken = %q, %v", tok, err)
	}

	// Each refresh rotates the refresh token; the next one must use it.
	for i, want := range []string{"at-2", "at-3"} {
		clock = clock.Add(time.Hour)
		tok, err := m.AccessToken(ctx, acct)
		if err != nil || tok != want {
			t.Fatalf("refresh %d: AccessToken = %q, %v", i+1, tok, err)
		}
		if rt, _ := sec.Get(ctx, secrets.RefreshToken(acct)); rt != "rt-"+want[3:] {
			t.Fatalf("refresh %d: stored refresh token = %q", i+1, rt)
		}
	}
}
