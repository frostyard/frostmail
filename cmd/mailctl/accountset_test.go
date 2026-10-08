package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestAccountSet(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	server := api.ServerConfig{Host: "imap.mailtest.test", Port: 993, TLS: api.TLSModeTLS, Username: "a@mailtest.test"}
	a, err := c.Account().Create(t.Context(), &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "a@mailtest.test", Auth: api.AuthKindPassword, IMAP: &server, SMTP: &server,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(a.ID, 10)
	set := func(args ...string) (string, error) {
		return runWithStdin(t, "", append([]string{"--socket", srv.Socket, "account", "set", id}, args...)...)
	}

	out, err := set("--sync-days", "365", "--read-only", "--no-notify")
	if err != nil || !strings.Contains(out, "keeps the last 365 days, read-only, does not notify") {
		t.Fatalf("set = %q, %v", out, err)
	}
	got, err := c.Account().Get(t.Context(), &api.AccountGetParams{ID: a.ID})
	if err != nil || got.SyncDays != 365 || !got.ReadOnly || got.Notify {
		t.Fatalf("account after set = %+v, %v", got, err)
	}
	// Only the flags given change.
	out, err = set("--read-write")
	if err != nil || !strings.Contains(out, "keeps the last 365 days, read-write, does not notify") {
		t.Errorf("set --read-write = %q, %v", out, err)
	}
	out, err = set("--sync-days", "0", "--notify")
	if err != nil || !strings.Contains(out, "keeps every message, read-write, notifies") {
		t.Errorf("set --sync-days 0 = %q, %v", out, err)
	}

	for _, bad := range [][]string{
		{},
		{"--read-only", "--read-write"},
		{"--notify", "--no-notify"},
		{"--sync-days", "-1"},
	} {
		if _, err := set(bad...); err == nil {
			t.Errorf("set %q succeeded", bad)
		}
	}
	if _, err := runWithStdin(t, "", "--socket", srv.Socket, "account", "set", "x", "--read-only"); err == nil {
		t.Error("set with a bad id succeeded")
	}
}
