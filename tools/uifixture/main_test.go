package main

import (
	"path/filepath"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

func TestBuildPointsTheAccountAtAnSMTPServer(t *testing.T) {
	out := filepath.Join(t.TempDir(), "data")
	if err := Build(t.Context(), Options{Out: out, SMTP: "127.0.0.1:2525"}); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(t.Context(), filepath.Join(out, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	accts, err := db.ListAccounts(t.Context())
	if err != nil || len(accts) != 1 {
		t.Fatalf("accounts = %+v, %v", accts, err)
	}
	if s := accts[0].SMTP; s.Host != "127.0.0.1" || s.Port != 2525 || s.TLS != api.TLSModeInsecure {
		t.Errorf("SMTP = %+v", s)
	}
	pw, err := secrets.NewFile(filepath.Join(out, "secrets.json")).Get(t.Context(), secrets.AccountPassword(accts[0].ID))
	if err != nil || pw == "" {
		t.Errorf("password = %q, %v", pw, err)
	}
}
