//go:build linux && integration

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Each test owns its maild and generated accounts. No host config, keyring,
// mail server or installed app participates in the test.
func fixtureWorkspace(t *testing.T) *nsgotest.App {
	t.Helper()
	dir, err := os.MkdirTemp("", "fm-ui-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	settings := map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "config"), "XDG_STATE_HOME": filepath.Join(dir, "state"), "FROSTMAIL_SOCKET": filepath.Join(dir, "maild.sock"), "FROSTMAIL_DATA_DIR": filepath.Join(dir, "data"), "FROSTMAIL_CACHE_DIR": filepath.Join(dir, "cache"), "FROSTMAIL_SYNC": "off", "FROSTMAIL_SECRETS": "file"}
	for k, v := range settings {
		t.Setenv(k, v)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "go", args...)
		cmd.Dir = "../.."
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, out)
		}
	}
	run("run", "./tools/uifixture", "-out", settings["FROSTMAIL_DATA_DIR"], "-showcase", "tools/uifixture/showcase")
	bin := filepath.Join(dir, "maild")
	run("build", "-o", bin, "./cmd/maild")
	daemon := exec.CommandContext(t.Context(), bin)
	daemon.Stderr = os.Stderr
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = daemon.Process.Kill(); _ = daemon.Wait() })
	a := nsgotest.Start(t, root, nsgotest.Size(1280, 820))
	a.WaitFor("text:Connected · Local", 20*time.Second)
	return a
}

func TestMailboxReadsFixture(t *testing.T) {
	a := fixtureWorkspace(t)
	const subject = "Your trip to Lisbon: itinerary"
	a.WaitFor("label:"+subject, 8*time.Second)
	a.Press("label:" + subject)
	a.WaitFor("label:Message body", 5*time.Second)
}

func TestDraftRoundTrip(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Compose")
	a.WaitFor("label:Subject", 5*time.Second)
	a.Type("label:To", "fixture@example.test")
	a.Type("label:Subject", "NSGo draft round trip")
	a.Press("label:Save and close")
	a.WaitUntil(5*time.Second, func() bool { return !a.Exists("label:Subject") })
	a.Press("label:Open local drafts")
	a.WaitFor("label:NSGo draft round trip", 5*time.Second)
	a.Press("label:NSGo draft round trip")
	a.WaitFor("label:Subject", 5*time.Second)
	if got := a.Find("label:Subject").Text; got != "NSGo draft round trip" {
		t.Fatalf("reopened subject: %q", got)
	}
}

func TestPersonalOrganizerRoundTrip(t *testing.T) {
	a := fixtureWorkspace(t)
	for _, item := range []struct{ module, add, field, title string }{
		{"Tasks", "New task", "Title", "Book fixture appointment"},
		{"People", "New contact", "Given name", "Fixtureperson"},
		{"Calendar", "New event", "Title", "Fixture appointment"},
	} {
		a.Press("label:" + item.module)
		a.WaitFor("label:"+item.add, 5*time.Second)
		a.Press("label:" + item.add)
		a.Type("label:"+item.field, item.title)
		a.Press("label:Save")
		if item.module == "Calendar" {
			a.WaitUntil(5*time.Second, func() bool { return !a.Exists("label:Save") })
			a.Type("label:Search events", item.title)
			a.Submit("label:Search events")
		}
		a.WaitFor("label:"+item.title, 5*time.Second)
		a.Press("label:" + item.title)
		a.WaitFor("label:Close", 5*time.Second)
		a.Press("label:Close")
	}
}

func TestAccountAndSnippetSettings(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Settings")
	a.WaitFor("label:Add account", 5*time.Second)
	a.Press("label:Add account")
	a.Type("label:Email address", "offline@example.test")
	a.Press("label:Manual setup")
	a.Type("label:Full name", "Offline fixture")
	a.Type("label:IMAP host", "imap.example.test")
	a.Type("label:SMTP host", "smtp.example.test")
	a.Press("label:Save account")
	a.WaitFor("label:Set password", 5*time.Second)
	a.Press("label:Close")
	a.WaitFor("label:Manage offline@example.test", 5*time.Second)
	a.Press("label:Snippets")
	a.Press("label:Add snippet")
	a.Type("label:Snippet name", "Fixture signoff")
	a.Type("label:Snippet HTML", "<p>Good evening</p>")
	a.Press("label:Save")
	a.WaitFor("label:Fixture signoff", 5*time.Second)
}

func TestOrganizeRuleAndSmartMailbox(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Settings")
	a.WaitFor("label:Add account", 5*time.Second)
	a.Press("label:Filters")
	for _, item := range []struct{ open, title string }{{"New rule", "Fixture rule"}, {"New smart mailbox", "Fixture smart mailbox"}} {
		a.Press("label:" + item.open)
		a.Type("label:Name", item.title)
		a.Type("label:Condition value", "example.test")
		a.Press("label:Save")
		a.WaitFor("label:Edit "+item.title, 5*time.Second)
	}
}

func TestMessageHeaders(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Your trip to Lisbon: itinerary")
	a.WaitFor("label:Message actions", 5*time.Second)
	a.Press("label:Message actions")
	a.Press("label:Show headers")
	a.WaitFor("text:All headers", 5*time.Second)
	a.WaitFor("label:Save message as", 5*time.Second)
}

func TestScheduledSendReturnsToDraft(t *testing.T) {
	a := fixtureWorkspace(t)
	a.WaitFor("label:Your trip to Lisbon: itinerary", 8*time.Second)
	a.Press("label:Compose")
	a.WaitFor("label:Subject", 5*time.Second)
	a.Type("label:To", "fixture@example.test")
	a.Type("label:Subject", "Scheduled fixture")
	a.Press("label:Send later")
	a.Press("label:Send 8:00 AM Tomorrow")
	a.WaitFor("label:Undo send", 5*time.Second)
	a.Press("label:Undo send")
	a.WaitFor("label:Subject", 5*time.Second)
	if got := a.Find("label:Subject").Text; got != "Scheduled fixture" {
		t.Fatalf("Reopened subject %q", got)
	}
}

func TestConversationOpensRelatedMessages(t *testing.T) {
	a := fixtureWorkspace(t)
	const subject = "Re: Aurora launch checklist"
	a.WaitFor("label:"+subject, 8*time.Second)
	a.Press("label:" + subject)
	a.WaitFor("label:View conversation (3 messages)", 5*time.Second)
	a.Press("label:View conversation (3 messages)")
	a.WaitFor("label:Open this message", 5*time.Second)
	a.Press("label:Close")
	a.WaitFor("label:View conversation (3 messages)", 5*time.Second)
}
