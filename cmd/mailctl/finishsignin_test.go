package main

import (
	"net/url"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
)

// TestFinishSignInCommand finishes a waiting sign-in with the address the
// browser could not open (ADR-0043), once only.
func TestFinishSignInCommand(t *testing.T) {
	srv := signinServer(t)
	sock := []string{"--socket", srv.Socket}
	if _, err := runWithStdin(t, "cs", append(sock, "oauth", "set-client", "google", "--client-id", "cid", "--secret-stdin")...); err != nil {
		t.Fatal(err)
	}
	out, err := runWithStdin(t, "", append(sock, "account", "connect", "ann@gmail.com", "--oauth", "--no-wait")...)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	authURL, err := url.Parse(strings.TrimSpace(strings.Split(out, "\n")[2]))
	if err != nil {
		t.Fatalf("auth URL in %q: %v", out, err)
	}
	back := authURL.Query().Get("redirect_uri") + "?" + url.Values{"code": {"c0de"}, "state": {authURL.Query().Get("state")}}.Encode()
	out, err = runWithStdin(t, "", append(sock, "account", "finish-signin", back)...)
	if err != nil || out != "account 1 (ann@gmail.com) signed in\n" {
		t.Fatalf("finish-signin = %q, %v", out, err)
	}
	a, err := srv.Dial(t).Account().Get(t.Context(), &api.AccountGetParams{ID: 1})
	if err != nil || !a.SignedIn {
		t.Fatalf("account = %+v, %v", a, err)
	}
	if _, err := runWithStdin(t, "", append(sock, "account", "finish-signin", back)...); err == nil ||
		!strings.Contains(err.Error(), "no sign-in waits") {
		t.Errorf("finish-signin twice: %v", err)
	}
	if _, err := runWithStdin(t, "", append(sock, "account", "finish-signin")...); err == nil {
		t.Error("finish-signin without an address succeeded")
	}
}
