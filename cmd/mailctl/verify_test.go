package main

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestVerify(t *testing.T) {
	srv := syncFixture(t, true)
	if _, err := runMailctl(t, "--socket", srv.Socket, "sync", "1", "--wait"); err != nil {
		t.Fatal(err)
	}
	out, err := runMailctl(t, "--socket", srv.Socket, "verify", "1")
	if err != nil {
		t.Fatalf("verify after a sync = %q, %v", out, err)
	}
	if !strings.HasPrefix(out, "account 1: ok\n") || !strings.Contains(out, "  INBOX: 5 on the server, 5 here\n") {
		t.Errorf("verify output = %q", out)
	}
	if _, err := runMailctl(t, "--socket", srv.Socket, "verify", "7"); err == nil || !strings.Contains(err.Error(), "account 7") {
		t.Errorf("verify of a missing account = %v", err)
	}
}

func TestPrintReport(t *testing.T) {
	var b strings.Builder
	printReport(&b, &api.VerifyReport{AccountID: 2, Mailboxes: []api.MailboxCheck{
		{Path: "[Gmail]/All Mail", Server: 10, Local: 9, MissingLocally: []int64{4, 7}, FlagDiffs: 1, LabelDiffs: 3},
		{Path: "[Gmail]/Trash", Server: 1, Local: 2, MissingOnServer: []int64{12}, MissingLocally: []int64{}},
	}})
	want := "account 2: DIFFERS\n" +
		"  [Gmail]/All Mail: 10 on the server, 9 here\n" +
		"    missing here: UID 4, 7\n    flags differ: 1 message\n    labels differ: 3 messages\n" +
		"  [Gmail]/Trash: 1 on the server, 2 here\n    missing on the server: UID 12\n"
	if b.String() != want {
		t.Errorf("report = %q, want %q", b.String(), want)
	}
}
