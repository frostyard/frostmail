package mailsync_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/smtpx/smtpxtest"
)

// tokenServer grants "tok-1" for the authorization code "c0de" and keeps
// granting it on refresh.
func tokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Form.Get("grant_type") == "authorization_code" && r.Form.Get("code") == "c0de":
			_, _ = io.WriteString(w, `{"access_token":"tok-1","expires_in":3600,"refresh_token":"ref-1"}`)
		case r.Form.Get("grant_type") == "refresh_token" && r.Form.Get("refresh_token") == "ref-1":
			_, _ = io.WriteString(w, `{"access_token":"tok-1","expires_in":3600}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestOAuthSignInSyncsAndSends signs a Gmail-kind account in through
// account.authorize, then syncs over IMAP and sends over SMTP with XOAUTH2.
// The memory server has no Gmail extensions, so the generic sync path runs.
func TestOAuthSignInSyncsAndSends(t *testing.T) {
	mem := imapxtest.StartMemFullOAuth(t, "tok-1")
	sm := smtpxtest.Start(t, "unused")
	sm.Token = "tok-1"
	ts := tokenServer(t)
	cfg := testConfig
	srv := rpctest.StartWith(t, rpctest.Options{
		Sync:           &cfg,
		OAuthEndpoints: map[string]oauth.Endpoint{"google": {AuthURL: "https://accounts.test/auth", TokenURL: ts.URL, Scopes: oauth.Google.Scopes}},
	})
	c := srv.Dial(t)
	ctx := t.Context()
	if _, err := c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Oauth().SetClient(ctx, &api.OauthSetClientParams{Provider: api.OAuthProviderGoogle, ClientID: "cid", ClientSecret: ptr("cs")}); err != nil {
		t.Fatal(err)
	}
	o := mem.DialOptions()
	a, err := c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindGmail, Email: "ann@gmail.com", Auth: api.AuthKindOAuth2,
		IMAP: &api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: imapxtest.Username},
		SMTP: sm.Config(imapxtest.Username),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := &sendHarness{harness: &harness{t: t, srv: srv, c: c, acct: a.ID}, mem: mem, smtp: sm}
	h.waitPhase(api.SyncPhaseUnauthorized) // no grant yet

	res, err := c.Account().Authorize(ctx, &api.AccountAuthorizeParams{ID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.URL)
	back := u.Query().Get("redirect_uri") + "?" + url.Values{"code": {"c0de"}, "state": {u.Query().Get("state")}}.Encode()
	resp, err := http.Get(back)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	h.waitPhase(api.SyncPhaseIdle)
	got, err := c.Account().Get(ctx, &api.AccountGetParams{ID: a.ID})
	if err != nil || !got.SignedIn {
		t.Fatalf("account = %+v, %v", got, err)
	}

	d := h.draft("Over OAuth", []api.Address{bob}, nil, nil)
	item, err := c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitFor(10*time.Second, "sent", func(_ api.EventEnvelope, ev api.Event) bool {
		o, ok := ev.(api.OutboxChanged)
		return ok && o.ID == item.ID && o.State == api.OutboxStateSent
	})
	if msgs := sm.Messages(); len(msgs) != 1 || !slices.Equal(msgs[0].To, []string{"bob@x.test"}) {
		t.Fatalf("sent = %+v", msgs)
	}
}
