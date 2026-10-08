package main

// CONTRACT TEST for task card T-0036 (docs/tasks). Do not edit.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/store"
)

const hostileDir = "../../internal/render/testdata/hostile"

func build(t *testing.T, n int) (*store.DB, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	if err := Build(t.Context(), Options{Out: dir, N: n, Seed: 1, Hostile: hostileDir}); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(t.Context(), filepath.Join(dir, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, dir
}

func mailboxByPath(t *testing.T, db *store.DB, accountID int64) map[string]store.Mailbox {
	t.Helper()
	list, err := db.ListMailboxes(t.Context(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]store.Mailbox{}
	for _, mb := range list {
		out[mb.Path] = mb
	}
	return out
}

func TestBuildAccountAndMailboxes(t *testing.T) {
	db, _ := build(t, 50)
	accts, err := db.ListAccounts(t.Context())
	if err != nil || len(accts) != 1 {
		t.Fatalf("accounts = %+v, %v", accts, err)
	}
	a := accts[0]
	if a.Email != "test1@mailtest.test" || a.IMAP.Host != "127.0.0.1" || a.IMAP.Port != 1 || a.Kind != api.AccountKindIMAP {
		t.Fatalf("account = %+v", a)
	}
	mbs := mailboxByPath(t, db, a.ID)
	want := map[string]api.MailboxRole{
		"INBOX": api.MailboxRoleInbox, "Drafts": api.MailboxRoleDrafts, "Sent": api.MailboxRoleSent,
		"Junk": api.MailboxRoleJunk, "Trash": api.MailboxRoleTrash, "Archive": api.MailboxRoleArchive,
		"Hostile": api.MailboxRoleNone,
	}
	for path, role := range want {
		if mb, ok := mbs[path]; !ok || mb.Role != role {
			t.Errorf("mailbox %s = %+v, want role %s", path, mb, role)
		}
	}
}

func TestBuildMessages(t *testing.T) {
	db, dir := build(t, 300)
	ctx := t.Context()
	accts, _ := db.ListAccounts(ctx)
	mbs := mailboxByPath(t, db, accts[0].ID)
	inbox, err := db.ViewIDs(ctx, store.ViewFilter{MailboxID: mbs["INBOX"].ID})
	if err != nil || len(inbox) != 300 {
		t.Fatalf("INBOX = %d messages, %v; want 300", len(inbox), err)
	}
	threads, err := db.ViewIDs(ctx, store.ViewFilter{MailboxID: mbs["INBOX"].ID, Threads: true})
	if err != nil || len(threads) >= 300 || len(threads) < 150 {
		t.Fatalf("INBOX threads = %d, %v; want replies grouped into threads", len(threads), err)
	}
	found, err := db.ViewIDs(ctx, store.ViewFilter{Match: `"ledger"*`})
	if err != nil || len(found) == 0 {
		t.Fatalf("search for ledger = %d, %v; want the search index filled", len(found), err)
	}
	rows, err := db.Summaries(ctx, inbox)
	if err != nil {
		t.Fatal(err)
	}
	var seen, unseen, flagged, attached int
	for _, r := range rows {
		if r.Flags.Seen {
			seen++
		} else {
			unseen++
		}
		if r.Flags.Flagged && r.Flags.Color == 1 {
			flagged++
		}
		if r.HasAttachments {
			attached++
		}
		if r.Subject == "" || r.From.Addr == "" || r.Preview == "" || r.Date.IsZero() {
			t.Fatalf("summary %+v lacks subject, sender, preview or date", r)
		}
	}
	if seen == 0 || unseen == 0 || flagged == 0 || attached == 0 {
		t.Fatalf("seen %d unseen %d flagged %d with attachments %d; want each", seen, unseen, flagged, attached)
	}
	blobs := blob.New(filepath.Join(dir, "blobs"))
	for _, id := range inbox[:20] {
		m, err := db.GetMessage(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := blobs.Has(m.BlobID); !ok || err != nil {
			t.Fatalf("message %d body %q not stored: %v", id, m.BlobID, err)
		}
		if !strings.HasPrefix(m.MessageID, "gen-1-") || !strings.HasSuffix(m.MessageID, "@mailgen.test") {
			t.Fatalf("message %d Message-ID %q", id, m.MessageID)
		}
		if m.HasAttachments && !slices.ContainsFunc(m.Parts, func(p store.Part) bool { return strings.HasSuffix(p.Filename, ".csv") }) {
			t.Fatalf("message %d has attachments but parts %+v", id, m.Parts)
		}
	}
}

func TestBuildHostileMailbox(t *testing.T) {
	db, _ := build(t, 10)
	ctx := t.Context()
	accts, _ := db.ListAccounts(ctx)
	mbs := mailboxByPath(t, db, accts[0].ID)
	files, _ := filepath.Glob(filepath.Join(hostileDir, "*.html"))
	ids, err := db.ViewIDs(ctx, store.ViewFilter{MailboxID: mbs["Hostile"].ID})
	if err != nil || len(ids) != len(files) || len(files) < 40 {
		t.Fatalf("Hostile = %d messages, %v; want %d", len(ids), err, len(files))
	}
	rows, err := db.Summaries(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	subjects := map[string]bool{}
	for _, r := range rows {
		subjects[r.Subject] = true
	}
	if !subjects["Hostile: 01-script-inline"] {
		t.Fatalf("subjects = %v", subjects)
	}
}

func TestRunFlags(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := run(t.Context(), []string{"-out", dir, "-n", "5"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "frostmail.db")); err != nil {
		t.Fatal(err)
	}
	if err := run(t.Context(), []string{"-n", "5"}); err == nil {
		t.Fatal("missing -out accepted")
	}
	if err := run(t.Context(), []string{"-out", dir, "-n", "-1"}); err == nil {
		t.Fatal("negative -n accepted")
	}
	if err := run(t.Context(), []string{"-out", dir, "-n", "5"}); err == nil {
		t.Fatal("an existing -out was overwritten")
	}
}
