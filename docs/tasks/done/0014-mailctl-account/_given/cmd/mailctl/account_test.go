package main

// CONTRACT TEST for task card T-0014 (docs/tasks). Do not edit.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/secrets"
)

// runWithStdin runs mailctl like runMailctl (hello_test.go) with stdin set.
func runWithStdin(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	clix.Stdout = &out
	t.Cleanup(func() { clix.Stdout = nil })
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetIn(strings.NewReader(stdin))
	err := (&clix.App{Version: "test"}).Run(root)
	return out.String(), err
}

func TestParseServer(t *testing.T) {
	cases := []struct {
		in   string
		want api.ServerConfig
	}{
		{"imap.mailtest.test:993", api.ServerConfig{Host: "imap.mailtest.test", Port: 993, TLS: api.TLSModeTLS, Username: "me@mailtest.test"}},
		{"imap.mailtest.test:143", api.ServerConfig{Host: "imap.mailtest.test", Port: 143, TLS: api.TLSModeStartTLS, Username: "me@mailtest.test"}},
		{"smtp.mailtest.test:465", api.ServerConfig{Host: "smtp.mailtest.test", Port: 465, TLS: api.TLSModeTLS, Username: "me@mailtest.test"}},
		{"smtp.mailtest.test:587", api.ServerConfig{Host: "smtp.mailtest.test", Port: 587, TLS: api.TLSModeStartTLS, Username: "me@mailtest.test"}},
		{"10.0.0.5:2525/insecure", api.ServerConfig{Host: "10.0.0.5", Port: 2525, TLS: api.TLSModeInsecure, Username: "me@mailtest.test"}},
		{"mail.test:1993/tls", api.ServerConfig{Host: "mail.test", Port: 1993, TLS: api.TLSModeTLS, Username: "me@mailtest.test"}},
		{"[::1]:143/starttls", api.ServerConfig{Host: "::1", Port: 143, TLS: api.TLSModeStartTLS, Username: "me@mailtest.test"}},
	}
	for _, tc := range cases {
		got, err := parseServer(tc.in, "me@mailtest.test")
		if err != nil || got != tc.want {
			t.Errorf("parseServer(%q) = %+v, %v; want %+v", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"", "nohost", "host:0", "host:70000", "host:abc", "host:1234", "host:993/bogus"} {
		if _, err := parseServer(bad, "u"); err == nil {
			t.Errorf("parseServer(%q) succeeded; want an error", bad)
		}
	}
}

func TestAccountAddListRemove(t *testing.T) {
	srv := rpctest.Start(t)
	out, err := runWithStdin(t, "s3cret\n", "--socket", srv.Socket, "account", "add", "test1@mailtest.test",
		"--name", "Test One", "--imap", "imap.mailtest.test:993", "--smtp", "smtp.mailtest.test:587", "--password-stdin")
	if err != nil {
		t.Fatal(err)
	}
	if out != "added account 1 (test1@mailtest.test)\n" {
		t.Fatalf("add output = %q", out)
	}
	stored, err := srv.Secrets.Get(t.Context(), secrets.AccountPassword(1))
	if err != nil || stored != "s3cret" {
		t.Fatalf("password = %q, %v; want s3cret without the newline", stored, err)
	}

	out, err = runWithStdin(t, "", "--socket", srv.Socket, "account", "add", "two@mailtest.test",
		"--imap", "imap.two.test:143", "--smtp", "smtp.two.test:465", "--username", "two")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "added account 2 (two@mailtest.test)\n") {
		t.Fatalf("second add output = %q", out)
	}

	out, err = runWithStdin(t, "", "--socket", srv.Socket, "account", "list")
	if err != nil {
		t.Fatal(err)
	}
	want := "ID  EMAIL                IMAP                        SMTP\n" +
		"1   test1@mailtest.test  imap.mailtest.test:993/tls  smtp.mailtest.test:587/starttls\n" +
		"2   two@mailtest.test    imap.two.test:143/starttls  smtp.two.test:465/tls\n"
	if out != want {
		t.Fatalf("list output =\n%s\nwant\n%s", out, want)
	}

	out, err = runWithStdin(t, "", "--socket", srv.Socket, "--json", "account", "list")
	if err != nil {
		t.Fatal(err)
	}
	var accounts []api.Account
	if err := json.Unmarshal([]byte(out), &accounts); err != nil || len(accounts) != 2 || accounts[1].IMAP.Username != "two" || accounts[0].DisplayName != "Test One" {
		t.Fatalf("JSON list = %q (%v)", out, err)
	}

	out, err = runWithStdin(t, "", "--socket", srv.Socket, "account", "rm", "1")
	if err != nil || out != "removed account 1\n" {
		t.Fatalf("rm output = %q, %v", out, err)
	}
	if _, err := runWithStdin(t, "", "--socket", srv.Socket, "account", "rm", "1"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("second rm = %v; want a not-found error", err)
	}
	if _, err := runWithStdin(t, "", "--socket", srv.Socket, "account", "rm", "x"); err == nil {
		t.Fatal("rm x succeeded; want an error")
	}
}

func TestAccountAddNeedsServers(t *testing.T) {
	srv := rpctest.Start(t)
	if _, err := runWithStdin(t, "", "--socket", srv.Socket, "account", "add", "a@mailtest.test", "--imap", "imap.test:993"); err == nil {
		t.Fatal("add without --smtp succeeded")
	}
}
