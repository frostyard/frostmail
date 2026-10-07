package main

// CONTRACT TEST for task card T-0020 (docs/tasks). Do not edit.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// syncFixture runs maild with sync against an in-memory IMAP server holding
// the five seed messages, with one account (ID 1) whose password is set when
// withPassword is true.
func syncFixture(t *testing.T, withPassword bool) *rpctest.Server {
	t.Helper()
	mem := imapxtest.StartMem(t)
	files, _ := filepath.Glob("../../dev/incus/seed/*.eml")
	slices.Sort(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := mem.User.Append("INBOX", bytes.NewReader(raw), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := mailsync.Config{PollInterval: time.Hour, MinBackoff: 50 * time.Millisecond}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg})
	o := mem.DialOptions()
	server := api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: o.Username}
	c := srv.Dial(t)
	if _, err := c.Account().Create(t.Context(), &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "test@mailtest.test", Auth: api.AuthKindPassword, IMAP: server, SMTP: server,
	}); err != nil {
		t.Fatal(err)
	}
	if withPassword {
		if err := c.Account().SetPassword(t.Context(), &api.AccountSetPasswordParams{ID: 1, Password: imapxtest.Password}); err != nil {
			t.Fatal(err)
		}
	}
	return srv
}

// messageID finds a message by subject through a search view.
func messageID(t *testing.T, srv *rpctest.Server, subject string) int64 {
	t.Helper()
	out, err := runMailctl(t, "--socket", srv.Socket, "--json", "search", subject)
	if err != nil {
		t.Fatal(err)
	}
	var rows []api.MessageSummary
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) == 0 {
		t.Fatalf("search %q = %q (%v)", subject, out, err)
	}
	return rows[0].ID
}

func TestSyncWait(t *testing.T) {
	srv := syncFixture(t, true)
	out, err := runMailctl(t, "--socket", srv.Socket, "sync", "1", "--wait")
	if err != nil {
		t.Fatal(err)
	}
	if out != "account 1: up to date\n" {
		t.Fatalf("sync output = %q", out)
	}
	out, err = runMailctl(t, "--socket", srv.Socket, "sync", "--wait")
	if err != nil || out != "account 1: up to date\n" {
		t.Fatalf("sync of every account = %q, %v", out, err)
	}
	if _, err := runMailctl(t, "--socket", srv.Socket, "sync", "7"); err == nil {
		t.Fatal("sync of a missing account succeeded")
	}
}

func TestSyncWaitReportsBadPassword(t *testing.T) {
	srv := syncFixture(t, false)
	_, err := runMailctl(t, "--socket", srv.Socket, "sync", "1", "--wait", "--timeout", "10s")
	if err == nil || !strings.Contains(err.Error(), "account 1: unauthorized") {
		t.Fatalf("sync without a password = %v; want an unauthorized error", err)
	}
}

func TestShow(t *testing.T) {
	srv := syncFixture(t, true)
	if _, err := runMailctl(t, "--socket", srv.Socket, "sync", "1", "--wait"); err != nil {
		t.Fatal(err)
	}
	id := messageID(t, srv, "numbers")
	out, err := runMailctl(t, "--socket", srv.Socket, "show", strconv.FormatInt(id, 10))
	if err != nil {
		t.Fatal(err)
	}
	want := "From: Carol Accountant <carol@mailtest.test>\n" +
		"To: Test One <test1@mailtest.test>\n" +
		"Date: " + time.Date(2026, 10, 7, 12, 45, 0, 0, time.UTC).Local().Format("2006-01-02 15:04") + "\n" +
		"Subject: Q3 numbers\n" +
		"Attachments: q3.csv\n" +
		"\n" +
		"Numbers attached.\n"
	if out != want {
		t.Fatalf("show =\n%s\nwant\n%s", out, want)
	}

	reply := messageID(t, srv, "noon")
	out, err = runMailctl(t, "--socket", srv.Socket, "show", strconv.FormatInt(reply, 10))
	if err != nil || !strings.HasPrefix(out, "From: Test One <test1@mailtest.test>\nTo: Bob Builder <bob@mailtest.test>\nDate: ") ||
		strings.Contains(out, "Attachments:") || !strings.HasSuffix(out, "\n\nThursday works. Noon?\n\n> Are you free for lunch on Thursday?\n") {
		t.Fatalf("show reply = %q, %v", out, err)
	}

	out, err = runMailctl(t, "--socket", srv.Socket, "--json", "show", strconv.FormatInt(id, 10))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Message api.Message `json:"message"`
		Body    api.Body    `json:"body"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Message.Summary.Subject != "Q3 numbers" || got.Body.Text != "Numbers attached." {
		t.Fatalf("JSON show = %q (%v)", out, err)
	}
	if _, err := runMailctl(t, "--socket", srv.Socket, "show", "999"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("show 999 = %v", err)
	}
}
