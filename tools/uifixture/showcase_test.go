package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

const showcaseDir = "../../app/e2e/showcase"

// The showcase builds its accounts and mailboxes from the directory, takes
// flags and flag colors from file names, threads replies, and moves every
// date so the newest message arrived today.
func TestBuildShowcase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := Build(t.Context(), Options{Out: dir, Showcase: showcaseDir}); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(t.Context(), filepath.Join(dir, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	accounts, err := db.ListAccounts(t.Context())
	if err != nil || len(accounts) != 2 {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
	var work store.Account
	sec := secrets.NewFile(filepath.Join(dir, "secrets.json"))
	for _, a := range accounts {
		if a.Email == "ann@northwind.example" {
			work = a
		}
		if a.DisplayName != "Ann Lee" || a.IMAP.Host != "mail."+a.Email[strings.IndexByte(a.Email, '@')+1:] {
			t.Errorf("account %s is named %q on %s", a.Email, a.DisplayName, a.IMAP.Host)
		}
		if pw, err := sec.Get(t.Context(), secrets.AccountPassword(a.ID)); err != nil || pw == "" {
			t.Errorf("account %s has no password (%v), so it reads as signed out", a.Email, err)
		}
	}
	boxes := mailboxByPath(t, db, work.ID)
	if _, ok := boxes["Hostile"]; ok {
		t.Error("the showcase has the fixture's Hostile mailbox")
	}

	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		err := db.Tx(t.Context(), func(tx *store.Tx) error { return tx.QueryRowContext(t.Context(), query, args...).Scan(&n) })
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	for what, c := range map[string]struct {
		query string
		args  []any
		want  int
	}{
		"Projects, which holds no mail": {`SELECT COUNT(*) FROM mailboxes WHERE path = 'Projects' AND selectable = 0`, nil, 1},
		"work inbox":                    {`SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?`, []any{boxes["INBOX"].ID}, 8},
		"Projects/Aurora":               {`SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?`, []any{boxes["Projects/Aurora"].ID}, 2},
		"unread":                        {`SELECT COUNT(*) FROM messages WHERE seen = 0`, nil, 3},
		"red flags":                     {`SELECT COUNT(*) FROM messages WHERE flagged = 1 AND flag_color = 1`, nil, 1},
		"orange flags":                  {`SELECT COUNT(*) FROM messages WHERE flagged = 1 AND flag_color = 2`, nil, 1},
		"with attachments":              {`SELECT COUNT(*) FROM messages WHERE has_attachments = 1`, nil, 2},
		"the checklist chain":           {`SELECT COUNT(*) FROM messages WHERE thread_id = (SELECT thread_id FROM messages WHERE subject = 'Aurora launch checklist')`, nil, 3},
	} {
		if got := count(c.query, c.args...); got != c.want {
			t.Errorf("%s = %d, want %d", what, got, c.want)
		}
	}

	var newest string
	err = db.Tx(t.Context(), func(tx *store.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT MAX(internal_date) FROM messages`).Scan(&newest)
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.ParseTime(newest)
	if err != nil {
		t.Fatal(err)
	}
	if y, m, d := got.In(time.Local).Date(); time.Date(y, m, d, 0, 0, 0, 0, time.Local) != today() {
		t.Errorf("the newest message arrived %v, want today", got.In(time.Local))
	}
}

func today() time.Time {
	y, m, d := time.Now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}
