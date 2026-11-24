//go:build linux && integration

package main

import (
	"bufio"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	return fixtureLook(t, smtp, width, height, false)
}

// fixtureLook starts the client in a window of the given size, in the dark
// appearance or the light one: the test names it, so that a run does not
// follow its host's.
func fixtureLook(t *testing.T, smtp string, width, height core.Points, dark bool) *nsgotest.App {
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
	a := nsgotest.Start(t, root, nsgotest.Size(width, height), nsgotest.Environment(core.Environment{OS: "linux", Idiom: core.IdiomDesktop, PixelRatio: 1, Dark: dark}))
	// A compact window (under 720) hides the navigation and its status line:
	// its caller waits for what it shows instead.
	if width >= 720 {
		a.WaitFor("id:connection >> text:Local", 20*time.Second)
	}
	return a
}

// The General pane's choice of appearance shows at once and is kept on the
// device.
func TestAppearanceChoiceIsKept(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Settings")
	a.Press("role:tab General")
	a.WaitFor("role:tab Dark", 8*time.Second)
	a.Press("role:tab Dark")
	if a.Appearance != core.AppearanceDark {
		t.Fatalf("the app asked for appearance %v after Dark, want dark", a.Appearance)
	}
	if got, _ := a.Storage().GetItem("appearance"); got != "dark" {
		t.Fatalf("the kept appearance is %q, want dark", got)
	}
	a.Press("role:tab System")
	if _, kept := a.Storage().GetItem("appearance"); kept || a.Appearance != core.AppearanceSystem {
		t.Fatalf("after System the device keeps a choice (%v) or the app asks for %v", kept, a.Appearance)
	}
}

// TestToolbarFollowsTheOpenMessage checks the toolbar's reader segment: a
// toolbar whose buttons are dimmed with no message open, and act on the one
// that is.
func TestToolbarFollowsTheOpenMessage(t *testing.T) {
	a := fixtureWorkspace(t)
	const subject = "Your trip to Lisbon: itinerary"
	a.WaitFor("label:"+subject, 8*time.Second)
	a.Find("role:toolbar Message")
	for _, label := range []string{"Archive", "Delete", "Junk", "Reply", "Reply all", "Forward", "Flag", "Move"} {
		if a.Enabled("role:toolbar >> label:" + label) {
			t.Errorf("%s can be pressed with no message open", label)
		}
	}
	a.Press("label:" + subject)
	a.WaitUntil(5*time.Second, func() bool { return a.Enabled("role:toolbar >> label:Flag") })
	for _, label := range []string{"Archive", "Delete", "Junk", "Reply", "Reply all", "Forward", "Move"} {
		if !a.Enabled("role:toolbar >> label:" + label) {
			t.Errorf("%s cannot be pressed with a message open", label)
		}
	}
	a.Press("role:toolbar >> label:Flag")
	a.WaitFor("role:toolbar >> label:Unflag", 5*time.Second)
	a.Press("role:toolbar >> label:Unflag")
	a.WaitFor("role:toolbar >> label:Flag", 5*time.Second)
}

// TestNavigationHoldsTheModulesAndTheFoot checks the navigation: the
// modules are tabs at its head, a row's count is the title's unread count,
// Activity and Settings are at its foot, and no row names a module.
func TestNavigationHoldsTheModulesAndTheFoot(t *testing.T) {
	a := fixtureWorkspace(t)
	count := a.WaitFor("id:mailbox-all-inboxes >> id:nav-count", 8*time.Second).Text
	if n, err := strconv.Atoi(count); err != nil || n <= 0 {
		t.Fatalf("All Inboxes' count reads %q, want its unread messages", count)
	}
	a.Find("id:content-header >> text:" + count + " unread")
	for _, module := range []string{"Calendar", "People", "Tasks", "Mail"} {
		a.Press("role:tab " + module)
		title := module
		if module == "Mail" {
			title = "All Inboxes"
		}
		a.WaitFor("id:content-header >> text:"+title, 5*time.Second)
	}
	for _, module := range []string{"Activity", "Settings"} {
		if got := len(a.Query("label:" + module)); got != 1 {
			t.Errorf("%d controls are named %s, want the one at the navigation's foot", got, module)
		}
		a.Press("label:" + module)
		a.WaitFor("id:content-header >> text:"+module, 5*time.Second)
	}
	a.Press("label:Outbox")
	a.WaitFor("text:Outbox is empty", 5*time.Second)
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
	a.Press("label:Local Drafts")
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
	a.WaitFor("label:Message body", 5*time.Second)
	a.Press("label:More")
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
	a := fixtureWorkspace(t)
	const subject = "Your trip to Lisbon: itinerary"
	a.WaitFor("label:"+subject, 8*time.Second)
	a.DragTo("label:Drag "+subject, "label:File messages in Drafts · ann.lee@example.com")
	// The list is what the message leaves: a real pointer also chooses the
	// row it drags, so the reader may go on showing the message.
	a.WaitUntil(8*time.Second, func() bool { return !a.Exists("id:messages >> label:" + subject) })
	a.Press("id:mailbox-role:drafts")
	a.WaitFor("id:messages >> label:"+subject, 8*time.Second)
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
	a.WaitUntil(5*time.Second, func() bool { return a.Enabled("label:Forward") })
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

// TestMessageRowQuickActions checks a row's quick actions: they show while
// the pointer is over their row, and Delete asks before it deletes. A
// target does not report the hover's style, so the check of it is headless;
// the tour's picture mail-row-hover shows it in a window.
func TestMessageRowQuickActions(t *testing.T) {
	a := fixtureWorkspace(t)
	const subject = "Your trip to Lisbon: itinerary"
	row := "id:messages >> label:" + subject
	a.WaitFor(row, 5*time.Second)
	shown := func() bool { return a.Find(row+" >> id:row-actions").Style.Opacity == 1 }
	if nsgotest.Target() == "" && shown() {
		t.Fatal("the quick actions show with no pointer over the row")
	}
	a.Hover(row)
	if nsgotest.Target() == "" && !shown() {
		t.Fatal("the quick actions do not show with the pointer over the row")
	}
	if got := a.Find(row + " >> id:row-actions").Bounds().Size.Height; got <= 0 {
		t.Fatalf("the quick actions are %v high", got)
	}
	a.Press(row + " >> label:Delete message")
	a.WaitFor("label:Cancel", 5*time.Second)
	a.Press("label:Cancel")
	a.WaitFor(row, 5*time.Second)
}

// TestListChoosesSeveralMessages checks the list's selection: Ctrl adds a
// row, the reader then names the count, and the toolbar acts on them all.
func TestListChoosesSeveralMessages(t *testing.T) {
	a := fixtureWorkspace(t)
	first, second := "id:messages >> label:Your trip to Lisbon: itinerary", "id:messages >> label:Dinner Saturday?"
	a.WaitFor(first, 8*time.Second)
	a.Press(first)
	a.WaitFor("label:Message body", 8*time.Second)
	a.PressWith(second, core.Modifiers{Control: true})
	a.WaitFor("text:2 messages selected", 5*time.Second)
	if a.Exists("label:Message body") {
		t.Error("a message is shown with 2 chosen")
	}
	if a.Enabled("role:toolbar >> label:Reply") || !a.Enabled("role:toolbar >> label:Delete") {
		t.Error("with 2 chosen, Reply can be pressed or Delete cannot")
	}
	// A flag with no color chosen is the first color's, Red.
	flagged := func() int { return len(a.Query("id:messages >> label:Flagged Red")) }
	before := flagged()
	a.Press("role:toolbar >> label:Flag")
	a.WaitUntil(5*time.Second, func() bool { return flagged() == before+2 })
	a.Press("role:toolbar >> label:More")
	a.Press("label:Clear flags")
	a.WaitUntil(5*time.Second, func() bool { return flagged() == before })
	a.WaitFor("text:No message selected", 5*time.Second)
}

// TestSearchAndFilterTheList checks the search field and the filter: a
// search narrows the list, one with no match says so, the field's button
// ends it, and the Flagged tab shows the flagged messages alone.
func TestSearchAndFilterTheList(t *testing.T) {
	a := fixtureWorkspace(t)
	lisbon, dinner, flagged := "id:messages >> label:Your trip to Lisbon: itinerary", "id:messages >> label:Dinner Saturday?", "id:messages >> label:Design review notes: onboarding flow"
	a.WaitFor(dinner, 8*time.Second)
	a.Type("label:Search All Inboxes", "Lisbon")
	a.Submit("label:Search All Inboxes")
	a.WaitForGone(dinner, 8*time.Second)
	a.WaitFor(lisbon, 8*time.Second)
	a.Clear("label:Search All Inboxes")
	a.Type("label:Search All Inboxes", "no such words anywhere")
	a.Submit("label:Search All Inboxes")
	a.WaitFor("text:No matching messages", 8*time.Second)
	a.Press("role:button Clear search")
	a.WaitFor(dinner, 8*time.Second)
	a.Press("role:tab Flagged")
	a.WaitForGone(dinner, 8*time.Second)
	a.WaitFor(flagged, 8*time.Second)
	a.Press("role:tab All")
	a.WaitFor(dinner, 8*time.Second)
}

// TestArchiveAnswersWithAnUndoToast checks rule 5: a move answers with a
// toast that has an Undo action. The fixture's mail never syncs, so maild
// holds an undo of a move as still moving: the undo itself is not run here.
func TestArchiveAnswersWithAnUndoToast(t *testing.T) {
	a := fixtureWorkspace(t)
	row := "id:messages >> label:Your trip to Lisbon: itinerary"
	a.WaitFor(row, 8*time.Second)
	a.Press(row)
	a.WaitUntil(5*time.Second, func() bool { return a.Enabled("role:toolbar >> label:Archive") })
	a.Press("role:toolbar >> label:Archive")
	a.WaitForGone(row, 8*time.Second)
	a.WaitFor("id:undo >> text:Message archived", 5*time.Second)
	a.Find("id:undo >> role:button Undo")
}

func TestClosingNewDraftAndDiscardingEdits(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Compose")
	a.WaitFor("label:Subject", 5*time.Second)
	a.Key("ctrl+w")
	a.WaitUntil(5*time.Second, func() bool { return !a.Exists("label:Subject") })
	a.Press("label:Local Drafts")
	a.WaitFor("text:No local drafts", 5*time.Second)
	a.Press("label:Close")
	a.Press("label:Compose")
	a.WaitFor("label:To", 5*time.Second)
	a.Type("label:To", "an incomplete address")
	a.Press("label:Discard…")
	a.Press("label:Discard draft")
	a.WaitUntil(5*time.Second, func() bool { return !a.Exists("label:Subject") })
	a.Press("label:Local Drafts")
	a.WaitFor("text:No local drafts", 5*time.Second)
}
