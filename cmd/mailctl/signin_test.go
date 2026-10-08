package main

// CONTRACT TEST for task card T-0053 (docs/tasks). Do not edit.

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/discover"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/secrets"
)

type noSRV struct{}

func (noSRV) LookupSRV(_ context.Context, _, _, name string) (string, []*net.SRV, error) {
	return "", nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

// signinServer runs maild with a fake Google token endpoint and no network
// for discovery.
func signinServer(t *testing.T) *rpctest.Server {
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
	return rpctest.StartWith(t, rpctest.Options{
		OAuthEndpoints: map[string]oauth.Endpoint{"google": {AuthURL: "https://accounts.test/auth", TokenURL: ts.URL, Scopes: oauth.Google.Scopes}},
		Discovery: discover.Deps{
			Get:      func(context.Context, string) ([]byte, error) { return nil, io.EOF },
			Resolver: noSRV{},
		},
	})
}

// browse plays the browser for an authorization URL: the provider sends it
// back to maild with the code.
func browse(t *testing.T, authURL string) {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Errorf("parse %q: %v", authURL, err)
		return
	}
	back := u.Query().Get("redirect_uri") + "?" + url.Values{"code": {"c0de"}, "state": {u.Query().Get("state")}}.Encode()
	resp, err := http.Get(back)
	if err != nil {
		t.Errorf("browse: %v", err)
		return
	}
	_ = resp.Body.Close()
}

// browser is an output writer that browses to the first https URL printed.
type browser struct {
	t    *testing.T
	mu   sync.Mutex
	buf  bytes.Buffer
	once sync.Once
}

func (b *browser) Write(p []byte) (int, error) {
	b.mu.Lock()
	n, _ := b.buf.Write(p)
	text := b.buf.String()
	b.mu.Unlock()
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "https://") && strings.HasSuffix(text, "\n") {
			b.once.Do(func() { go browse(b.t, line) })
		}
	}
	return n, nil
}

func (b *browser) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func runTo(t *testing.T, out io.Writer, stdin string, args ...string) error {
	t.Helper()
	clix.Stdout = out
	t.Cleanup(func() { clix.Stdout = nil })
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(out)
	root.SetIn(strings.NewReader(stdin))
	return (&clix.App{Version: "test"}).Run(root)
}

func TestOAuthClientCommands(t *testing.T) {
	srv := signinServer(t)
	sock := []string{"--socket", srv.Socket}
	if _, err := runWithStdin(t, "", append(sock, "oauth", "show", "google")...); err == nil || !strings.Contains(err.Error(), "no google client") {
		t.Errorf("show before set: %v", err)
	}
	out, err := runWithStdin(t, "s3cret\n", append(sock, "oauth", "set-client", "google", "--client-id", "cid", "--secret-stdin")...)
	if err != nil || out != "google client set with its secret\n" {
		t.Fatalf("set = %q, %v", out, err)
	}
	out, err = runWithStdin(t, "", append(sock, "oauth", "show", "google")...)
	if err != nil || out != "google client cid (secret stored)\n" {
		t.Fatalf("show = %q, %v", out, err)
	}
	out, err = runWithStdin(t, "", append(sock, "oauth", "set-client", "google", "--client-id", "cid2")...)
	if err != nil || out != "google client set\n" {
		t.Fatalf("set without secret = %q, %v", out, err)
	}
	out, err = runWithStdin(t, "", append(sock, "oauth", "show", "google")...)
	if err != nil || out != "google client cid2 (secret stored)\n" {
		t.Fatalf("show = %q, %v", out, err)
	}
	if _, err := runWithStdin(t, "", append(sock, "oauth", "set-client", "yahoo", "--client-id", "x")...); err == nil || !strings.Contains(err.Error(), "yahoo") {
		t.Errorf("unknown provider: %v", err)
	}
}

func TestConnectGmailWithOAuth(t *testing.T) {
	srv := signinServer(t)
	sock := []string{"--socket", srv.Socket}
	if _, err := runWithStdin(t, "cs", append(sock, "oauth", "set-client", "google", "--client-id", "cid", "--secret-stdin")...); err != nil {
		t.Fatal(err)
	}
	b := &browser{t: t}
	if err := runTo(t, b, "", append(sock, "account", "connect", "ann@gmail.com", "--oauth", "--name", "Ann")...); err != nil {
		t.Fatalf("connect: %v\n%s", err, b)
	}
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if len(lines) != 4 || lines[0] != "added account 1 (ann@gmail.com, gmail)" ||
		lines[1] != "open this address in your browser to sign in:" || !strings.HasPrefix(lines[2], "https://accounts.test/auth?") ||
		lines[3] != "signed in" {
		t.Fatalf("output:\n%s", b)
	}
	c := srv.Dial(t)
	a, err := c.Account().Get(t.Context(), &api.AccountGetParams{ID: 1})
	if err != nil || !a.SignedIn || a.Auth != api.AuthKindOAuth2 || a.Kind != api.AccountKindGmail ||
		a.IMAP.Host != "imap.gmail.com" || a.DisplayName != "Ann" || a.ReadOnly || !a.Notify {
		t.Fatalf("account = %+v, %v", a, err)
	}
}

func TestConnectICloudWithPassword(t *testing.T) {
	srv := signinServer(t)
	out, err := runWithStdin(t, "app-pw\n", "--socket", srv.Socket, "account", "connect", "bob@icloud.com", "--password-stdin", "--read-only", "--no-notify")
	if err != nil || out != "added account 1 (bob@icloud.com, icloud)\npassword stored\n" {
		t.Fatalf("connect = %q, %v", out, err)
	}
	a, err := srv.Dial(t).Account().Get(t.Context(), &api.AccountGetParams{ID: 1})
	if err != nil || !a.SignedIn || !a.ReadOnly || a.Notify || a.SMTP.Host != "smtp.mail.me.com" || a.Auth != api.AuthKindPassword {
		t.Fatalf("account = %+v, %v", a, err)
	}
}

func TestConnectChecksItsArguments(t *testing.T) {
	srv := signinServer(t)
	sock := []string{"--socket", srv.Socket}
	if _, err := runWithStdin(t, "", append(sock, "account", "connect", "ann@nowhere.test", "--password-stdin")...); err == nil ||
		!strings.Contains(err.Error(), "no servers found for nowhere.test") || !strings.Contains(err.Error(), "mailctl account add") {
		t.Errorf("unknown domain: %v", err)
	}
	if _, err := runWithStdin(t, "", append(sock, "account", "connect", "ann@gmail.com")...); err == nil ||
		!strings.Contains(err.Error(), "--password-stdin or --oauth") {
		t.Errorf("no sign-in: %v", err)
	}
	if _, err := runWithStdin(t, "", append(sock, "account", "connect", "bob@icloud.com", "--oauth")...); err == nil {
		t.Error("iCloud with --oauth was accepted")
	}
	list, err := srv.Dial(t).Account().List(t.Context(), nil)
	if err != nil || len(list) != 0 {
		t.Errorf("accounts after refused connects = %+v, %v", list, err)
	}
}

func TestAuthorizeWithoutWaiting(t *testing.T) {
	srv := signinServer(t)
	sock := []string{"--socket", srv.Socket}
	if _, err := runWithStdin(t, "cs", append(sock, "oauth", "set-client", "google", "--client-id", "cid", "--secret-stdin")...); err != nil {
		t.Fatal(err)
	}
	out, err := runWithStdin(t, "", append(sock, "account", "connect", "ann@gmail.com", "--oauth", "--no-wait")...)
	if err != nil || !strings.HasPrefix(out, "added account 1 (ann@gmail.com, gmail)\nopen this address in your browser to sign in:\nhttps://") ||
		strings.Contains(out, "signed in") {
		t.Fatalf("connect --no-wait = %q, %v", out, err)
	}
	out, err = runWithStdin(t, "", append(sock, "account", "authorize", "1", "--no-wait")...)
	if err != nil || !strings.HasPrefix(out, "open this address in your browser to sign in:\nhttps://") {
		t.Fatalf("authorize --no-wait = %q, %v", out, err)
	}
	browse(t, strings.TrimSpace(strings.Split(out, "\n")[1]))
	c := srv.Dial(t)
	a, err := c.Account().Get(t.Context(), &api.AccountGetParams{ID: 1})
	if err != nil || !a.SignedIn {
		t.Fatalf("account = %+v, %v", a, err)
	}
	if _, err := runWithStdin(t, "pw", append(sock, "account", "connect", "bob@icloud.com", "--password-stdin")...); err != nil {
		t.Fatal(err)
	}
	if _, err := runWithStdin(t, "", append(sock, "account", "authorize", "2")...); err == nil || !strings.Contains(err.Error(), "OAuth") {
		t.Errorf("authorize a password account: %v", err)
	}
	if _, err := runWithStdin(t, "", append(sock, "account", "authorize", "x")...); err == nil || !strings.Contains(err.Error(), `invalid account id "x"`) {
		t.Errorf("authorize x: %v", err)
	}
}

func TestAccountPassword(t *testing.T) {
	srv := signinServer(t)
	sock := []string{"--socket", srv.Socket}
	if _, err := runWithStdin(t, "first\n", append(sock, "account", "connect", "bob@icloud.com", "--password-stdin")...); err != nil {
		t.Fatal(err)
	}
	out, err := runWithStdin(t, "second-pw\n", append(sock, "account", "password", "1", "--password-stdin")...)
	if err != nil || out != "password stored for account 1\n" {
		t.Fatalf("password = %q, %v", out, err)
	}
	if got, err := srv.Secrets.Get(t.Context(), secrets.AccountPassword(1)); err != nil || got != "second-pw" {
		t.Errorf("stored = %q, %v", got, err)
	}
	if _, err := runWithStdin(t, "x\n", append(sock, "account", "password", "1")...); err == nil {
		t.Error("password without --password-stdin succeeded")
	}
	if _, err := runWithStdin(t, "x\n", append(sock, "account", "password", "9", "--password-stdin")...); err == nil {
		t.Error("password for a missing account succeeded")
	}
}
