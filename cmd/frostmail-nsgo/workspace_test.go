//go:build linux && integration

package main

import (
	"bufio"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestMain starts NSGo's test host, so the suite also runs in a real window:
// go tool nsgo test -target linux (make nsgo-live).
func TestMain(m *testing.M) { nsgotest.Main(m) }

// Each test owns its maild and generated accounts. No host config, keyring,
// mail server or installed app participates in the test.
func fixtureWorkspace(t *testing.T) *nsgotest.App {
	t.Helper()
	return fixtureWithSMTP(t, "")
}

// headlessOnly skips a workflow on a target where it fails and headless it
// passes, with what the target showed. Plan 0016's Phase 0.5 records each,
// and its Phase 3 rebuilds the rows these two act on.
func headlessOnly(t *testing.T, seen string) {
	t.Helper()
	if target := nsgotest.Target(); target != "" {
		t.Skipf("skipped on %s: %s", target, seen)
	}
}

// repoRoot finds the repository from the working directory, which is this
// package's directory under go test and a directory below it on a target.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "tools", "uifixture")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no tools/uifixture above the working directory")
		}
		dir = parent
	}
}

func fixtureWithSMTP(t *testing.T, smtp string) *nsgotest.App {
	t.Helper()
	return fixtureSized(t, smtp, 1280, 820)
}

// fixtureSized starts the client in a window of the given size.
func fixtureSized(t *testing.T, smtp string, width, height core.Points) *nsgotest.App {
	t.Helper()
	dir, err := os.MkdirTemp("", "fm-ui-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	settings := map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "config"), "XDG_STATE_HOME": filepath.Join(dir, "state"), "FROSTMAIL_SOCKET": filepath.Join(dir, "maild.sock"), "FROSTMAIL_DATA_DIR": filepath.Join(dir, "data"), "FROSTMAIL_CACHE_DIR": filepath.Join(dir, "cache"), "FROSTMAIL_SYNC": "off", "FROSTMAIL_SECRETS": "file"}
	if smtp != "" {
		settings["FROSTMAIL_SYNC"] = "on"
		settings["FROSTMAIL_UNDO_DELAY"] = "0s"
	}
	for k, v := range settings {
		t.Setenv(k, v)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "go", args...)
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, out)
		}
	}
	args := []string{"run", "./tools/uifixture", "-out", settings["FROSTMAIL_DATA_DIR"], "-showcase", "tools/uifixture/showcase"}
	if smtp != "" {
		args = append(args, "-smtp", smtp)
	}
	run(args...)
	bin := filepath.Join(dir, "maild")
	run("build", "-o", bin, "./cmd/maild")
	daemon := exec.CommandContext(t.Context(), bin)
	if smtp != "" {
		// The showcase also includes illustrative DAV accounts. Keep incidental
		// sync traffic on loopback while exercising the real SMTP sender.
		daemon.Env = append(os.Environ(), "HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "NO_PROXY=127.0.0.1,localhost")
	}
	daemon.Stderr = os.Stderr
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = daemon.Process.Kill(); _ = daemon.Wait() })
	a := nsgotest.Start(t, root, nsgotest.Size(width, height), nsgotest.Environment(core.Environment{OS: "linux", Idiom: core.IdiomDesktop, PixelRatio: 1}))
	// A compact window (under 720) hides the navigation and its status line:
	// its caller waits for what it shows instead.
	if width >= 720 {
		a.WaitFor("text:Connected · Local", 20*time.Second)
	}
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

func TestDragMessageToMailbox(t *testing.T) {
	headlessOnly(t, "a row dragged onto a mailbox stays in its list (Xvfb, 2026-11-24)")
	a := fixtureWorkspace(t)
	const subject = "Your trip to Lisbon: itinerary"
	a.WaitFor("label:"+subject, 8*time.Second)
	a.DragTo("label:Drag "+subject, "label:File messages in Drafts · ann.lee@example.com")
	a.WaitUntil(8*time.Second, func() bool { return !a.Exists("label:" + subject) })
	a.Press("id:mailbox-role:drafts")
	a.WaitFor("label:"+subject, 8*time.Second)
}

func TestTodoRailCreatesTask(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:To-Do Bar")
	a.WaitFor("label:New task title", 5*time.Second)
	a.Type("label:New task title", "A task from the mail rail")
	a.Submit("label:New task title")
	a.WaitUntil(5*time.Second, func() bool { return a.Find("label:New task title").Text == "" })
	a.Press("label:Tasks")
	a.WaitFor("label:A task from the mail rail", 5*time.Second)
}

func TestUndoCalendarCreation(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Calendar")
	a.Press("label:New event")
	a.Type("label:Title", "A calendar change to undo")
	a.Press("label:Save")
	a.WaitFor("label:Undo calendar change", 5*time.Second)
	a.Press("label:Undo calendar change")
	a.Type("label:Search events", "A calendar change to undo")
	a.Submit("label:Search events")
	a.WaitFor("text:No matching items", 5*time.Second)
}

func TestForwardToLocalSMTP(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "smtpsink")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "./tools/smtpsink")
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smtpsink: %v: %s", err, out)
	}
	sink := exec.CommandContext(t.Context(), bin, "-dir", dir)
	output, err := sink.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sink.Process.Kill(); _ = sink.Wait() })
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	addr, ok := strings.CutPrefix(strings.TrimSpace(line), "listening ")
	if !ok || !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("unexpected SMTP listener %q", line)
	}
	a := fixtureWithSMTP(t, addr)
	const subject = "Your trip to Lisbon: itinerary"
	a.WaitFor("label:"+subject, 5*time.Second)
	a.Press("label:" + subject)
	a.WaitFor("label:Forward or reply all", 5*time.Second)
	a.Press("label:Forward or reply all")
	a.Press("label:Forward")
	a.WaitFor("label:To", 5*time.Second)
	a.Type("label:To", "traveler@example.test")
	a.Press("label:Send")
	var raw []byte
	a.WaitUntil(30*time.Second, func() bool { raw, _ = os.ReadFile(filepath.Join(dir, "1.eml")); return len(raw) > 0 })
	m, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Header.Get("To"), "traveler@example.test") || !strings.Contains(m.Header.Get("Subject"), "Lisbon") || !strings.Contains(string(raw), "Content-ID:") && !strings.Contains(string(raw), "Content-Id:") {
		t.Fatalf("forwarded fixture lost its recipient, subject or inline image")
	}
}

func TestMessageRowQuickActions(t *testing.T) {
	headlessOnly(t, "the pointer over a row shows no quick actions; the row's OnMouseEnter is not called (Xvfb, 2026-11-24)")
	a := fixtureWorkspace(t)
	const subject = "Your trip to Lisbon: itinerary"
	a.WaitFor("label:"+subject, 5*time.Second)
	a.Hover("label:" + subject)
	a.WaitFor("label:Delete message", 5*time.Second)
	a.Press("label:Delete message")
	a.WaitFor("label:Cancel", 5*time.Second)
	a.Press("label:Cancel")
	a.WaitFor("label:"+subject, 5*time.Second)
}

func TestClosingNewDraftAndDiscardingEdits(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Compose")
	a.WaitFor("label:Subject", 5*time.Second)
	a.Key("ctrl+w")
	a.WaitUntil(5*time.Second, func() bool { return !a.Exists("label:Subject") })
	a.Press("label:Open local drafts")
	a.WaitFor("text:No local drafts", 5*time.Second)
	a.Press("label:Close")
	a.Press("label:Compose")
	a.WaitFor("label:To", 5*time.Second)
	a.Type("label:To", "an incomplete address")
	a.Press("label:Discard…")
	a.Press("label:Discard draft")
	a.WaitUntil(5*time.Second, func() bool { return !a.Exists("label:Subject") })
	a.Press("label:Open local drafts")
	a.WaitFor("text:No local drafts", 5*time.Second)
}
