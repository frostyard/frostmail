//go:build linux && integration

package main

import (
	"bufio"
	"context"
	"errors"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
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
	return fixtureWith(t, smtp, width, height, dark)
}

// fixtureWith starts the client as fixtureLook does, with NSGo's options
// for the run: what the device's storage holds at the start, for one.
func fixtureWith(t *testing.T, smtp string, width, height core.Points, dark bool, options ...nsgotest.Option) *nsgotest.App {
	t.Helper()
	a, _ := fixtureView(t, root, smtp, width, height, dark, options...)
	return a
}

// fixtureView starts view, the client or the client with what a test
// gives it, on a maild of its own, and returns that maild too, for a test
// that ends it.
func fixtureView(t *testing.T, view func(*core.Scope) core.Node, smtp string, width, height core.Points, dark bool, options ...nsgotest.Option) (*nsgotest.App, *exec.Cmd) {
	t.Helper()
	return fixtureStart(t, view, true, smtp, width, height, dark, options...)
}

// fixtureStart starts view on a maild of its own. With seed its store
// holds the showcase's accounts and mail; without, it holds nothing, as a
// first run's does.
func fixtureStart(t *testing.T, view func(*core.Scope) core.Node, seed bool, smtp string, width, height core.Points, dark bool, options ...nsgotest.Option) (*nsgotest.App, *exec.Cmd) {
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
	if seed {
		run(args...)
	}
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
	a := nsgotest.Start(t, view, append([]nsgotest.Option{nsgotest.Size(width, height),
		nsgotest.Environment(core.Environment{OS: "linux", Idiom: core.IdiomDesktop, PixelRatio: 1, Dark: dark})}, options...)...)
	// A compact window (under 720) hides the navigation and its status line:
	// its caller waits for what it shows instead.
	if width >= 720 {
		a.WaitFor("id:connection >> text:Local", 20*time.Second)
	}
	return a, daemon
}

// The General pane's choice of appearance shows at once and is kept on the
// device.
func TestAppearanceChoiceIsKept(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Settings")
	a.Press("label:General")
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
	// Each module's rows take the place of mail's while it is shown.
	for module, row := range map[string]string{"Calendar": "id:nav-0 >> text:All calendars", "People": "id:nav-0 >> text:All contacts", "Tasks": "id:nav-view:Today"} {
		a.Press("role:tab " + module)
		a.WaitFor(row, 5*time.Second)
		if a.Exists("id:mailbox-all-inboxes") {
			t.Errorf("%s shows mail's rows in the navigation", module)
		}
	}
	a.Press("role:tab Mail")
	a.WaitFor("id:mailbox-all-inboxes", 5*time.Second)
	a.Press("label:Outbox")
	a.WaitFor("text:Outbox is empty", 5*time.Second)
}

// TestReaderShowsWhoWhenAndWhat checks the reader: the header block names
// the sender, the recipients and the date, an invitation and a conversation
// have a banner with one action, and a file has its row.
func TestReaderShowsWhoWhenAndWhat(t *testing.T) {
	a := fixtureWorkspace(t)
	open := func(subject string) {
		t.Helper()
		a.Press("id:messages >> label:" + subject)
		a.WaitFor("id:reader >> text:"+subject, 8*time.Second)
	}
	a.WaitFor("id:messages >> label:Your trip to Lisbon: itinerary", 8*time.Second)
	open("Your trip to Lisbon: itinerary")
	a.Find("id:reader-header >> text:Skyline Travel")
	a.Find("id:reader-header >> text:To: Ann Lee")
	if got := a.Find("id:reader-date").Text; !strings.Contains(got, " at ") {
		t.Errorf("the date reads %q", got)
	}
	a.WaitFor("id:page-frame >> label:Message body", 8*time.Second)
	if a.Exists("id:reader >> role:status") {
		t.Error("a message with nothing to say about it has a banner")
	}
	open("Invitation: Release planning")
	a.WaitFor("id:reader >> role:status >> role:button Open invitation", 8*time.Second)
	open("Q4 roadmap review (slides attached)")
	a.WaitFor("id:reader >> id:attachment", 8*time.Second)
	open("Re: Aurora launch checklist")
	a.WaitFor("id:reader >> role:status >> role:button View conversation (3 messages)", 8*time.Second)
}

// TestButtonsAreQuietUnlessNamed holds rule 3's helpers: a button is quiet
// unless it is named the region's main one or a destroying one.
func TestButtonsAreQuietUnlessNamed(t *testing.T) {
	nothing := func() {}
	if got := button("Close", nothing).Variant; got != ui.ButtonSecondary {
		t.Errorf("a button is variant %v, want the quiet one", got)
	}
	if got := primary("Save", nothing).Variant; got != ui.ButtonPrimary {
		t.Errorf("the main button is variant %v, want the filled one", got)
	}
	if got := danger("Delete", nothing).Variant; got != ui.ButtonDanger {
		t.Errorf("a destroying button is variant %v, want danger", got)
	}
}

// TestDeletingAsksInAnAlert checks rule 4's confirmation: an alert dialog
// with Cancel before the destroying action, which a cancel leaves undone.
func TestDeletingAsksInAnAlert(t *testing.T) {
	a := fixtureWorkspace(t)
	row := "id:messages >> label:Your trip to Lisbon: itinerary"
	a.WaitFor(row, 8*time.Second)
	a.Press(row)
	a.WaitUntil(5*time.Second, func() bool { return a.Enabled("role:toolbar >> label:Delete") })
	a.Press("role:toolbar >> label:Delete")
	cancel := a.WaitFor("role:alertdialog >> role:button Cancel", 5*time.Second).Bounds()
	confirm := a.Find("role:alertdialog >> role:button Delete").Bounds()
	if cancel.Origin.X >= confirm.Origin.X {
		t.Errorf("Cancel is at %v and Delete at %v: Cancel comes first", cancel.Origin.X, confirm.Origin.X)
	}
	a.Find("role:alertdialog >> text:Delete message?")
	a.Press("role:alertdialog >> role:button Cancel")
	a.WaitForGone("role:alertdialog", 5*time.Second)
	a.Find(row)
}

// TestSettingsSearchesItsRowsAndListsAPanelsActions checks Settings: its
// categories are its own, a search finds a preference and an account by
// name, and an account's panel is a list of rows whose last asks in an
// alert before it removes.
func TestSettingsSearchesItsRowsAndListsAPanelsActions(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Settings")
	a.WaitFor("id:settings >> label:Add account", 5*time.Second)
	for _, category := range []string{"general", "identities", "snippets", "filters", "mailboxes", "sign-in", "accounts"} {
		a.Find("id:nav-" + category)
	}
	a.Type("label:Search settings", "undo")
	a.WaitFor("id:settings >> text:Undo send delay", 5*time.Second)
	if a.Exists("id:settings >> text:List preview lines") {
		t.Error("a search for undo also lists the preview lines")
	}
	a.Clear("label:Search settings")
	a.Type("label:Search settings", "northwind")
	a.WaitFor("id:settings >> label:Manage ann@northwind.example", 5*time.Second)
	a.Press("id:settings >> label:Manage ann@northwind.example")
	a.WaitFor("role:dialog >> label:Verify account", 5*time.Second)
	a.Press("role:dialog >> label:Remove account")
	a.WaitFor("role:alertdialog >> text:Remove account?", 5*time.Second)
	a.Press("role:alertdialog >> role:button Cancel")
	a.WaitForGone("role:alertdialog", 5*time.Second)
	a.Find("id:settings >> label:Manage ann@northwind.example")
}

// TestListFollowsTheTextSizeSetting checks that a row of the list draws
// its text at the size the Message text size setting asks for, as the
// message's page does: Largest is 1.3 times the default.
func TestListFollowsTheTextSizeSetting(t *testing.T) {
	a := fixtureWorkspace(t)
	const when = "id:messages >> id:summary-when >> nth=0"
	a.WaitFor(when, 8*time.Second)
	size := func() core.Points { return a.Find(when).TextStyle.Font.Size }
	plain := size()
	a.Press("label:Settings")
	a.Press("id:nav-general")
	a.WaitFor("label:Message text size", 5*time.Second)
	a.Press("label:Message text size")
	a.Press("role:option Largest")
	a.Press("role:tab Mail")
	a.WaitFor(when, 8*time.Second)
	a.WaitUntil(5*time.Second, func() bool { return size() != plain })
	if got, want := size(), plain*13/10; got < want-0.01 || got > want+0.01 {
		t.Errorf("a row's date is %v at Largest and %v at the default: want 1.3 times", got, plain)
	}
}

// TestComposeHasItsBarAndAsksInAlerts checks the compose window: the header
// is rows of named fields, formatting is a toolbar of buttons, a time to
// send at is NSGo's date field, and discarding asks in an alert that a
// cancel leaves.
func TestComposeHasItsBarAndAsksInAlerts(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Compose")
	a.WaitFor("label:Subject", 5*time.Second)
	for _, field := range []string{"From", "To", "Cc", "Bcc", "Subject"} {
		a.Find("role:dialog >> label:" + field)
	}
	for _, tool := range []string{"Bold", "Italic", "Bulleted list", "Link", "Insert", "Attach file"} {
		a.Find("role:toolbar Formatting >> label:" + tool)
	}
	// The compose window leaves by Discard or by Save and close, which keep
	// or drop the draft: it has no Cancel or Close that does neither.
	for _, stray := range []string{"Cancel", "Close"} {
		if a.Exists("role:dialog >> role:button " + stray) {
			t.Errorf("the compose window has a %s button", stray)
		}
	}
	for _, own := range []string{"Discard…", "Save and close", "Send"} {
		if len(a.Query("role:dialog >> role:button "+own)) == 0 {
			t.Errorf("the compose window has no %s button", own)
		}
	}
	a.Type("label:Subject", "A draft to keep")
	a.Press("label:Send later")
	a.Press("label:Choose date and time")
	a.WaitFor("label:Send at", 5*time.Second)
	a.Press("label:Cancel schedule")
	a.WaitForGone("label:Send at", 5*time.Second)
	a.Press("label:Discard…")
	a.WaitFor("role:alertdialog >> text:Discard this draft?", 5*time.Second)
	a.Press("role:alertdialog >> role:button Keep draft")
	a.WaitForGone("role:alertdialog", 5*time.Second)
	if got := a.Find("label:Subject").Text; got != "A draft to keep" {
		t.Errorf("after Keep draft the subject reads %q", got)
	}
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
	a.WaitFor("id:settings >> text:Fixture signoff", 5*time.Second)
	a.Find("label:Edit Fixture signoff")
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

// TestArchiveAndJunkSeveralMessages checks Archive and Junk with several
// messages chosen, of 2 accounts: each goes to its own account's mailbox,
// one toast answers for all of them, and Junk reads "Not junk" for
// messages that are all in Junk (#402).
func TestArchiveAndJunkSeveralMessages(t *testing.T) {
	a := fixtureWorkspace(t)
	// One message of each account, in All Inboxes.
	trip, standup := "id:messages >> label:Your trip to Lisbon: itinerary", "id:messages >> label:Standup moved to 10:30"
	choose := func(first, second string) {
		t.Helper()
		a.WaitFor(first, 8*time.Second)
		a.Press(first)
		a.PressWith(second, core.Modifiers{Control: true})
		a.WaitFor("text:2 messages selected", 5*time.Second)
	}
	choose(trip, standup)
	a.WaitUntil(5*time.Second, func() bool { return a.Enabled("role:toolbar >> label:Archive") })
	a.Press("role:toolbar >> label:Archive")
	a.WaitFor("id:undo >> text:2 messages archived", 8*time.Second)
	a.WaitForGone(trip, 8*time.Second)
	a.WaitForGone(standup, 8*time.Second)
	a.Find("id:undo >> role:button Undo")
	a.Press("id:mailbox-role:archive")
	a.WaitFor(trip, 8*time.Second)
	a.WaitFor(standup, 8*time.Second)

	a.Press("id:mailbox-all-inboxes")
	dinner, hiring := "id:messages >> label:Dinner Saturday?", "id:messages >> label:Hiring panel on Friday"
	choose(dinner, hiring)
	a.WaitUntil(5*time.Second, func() bool { return a.Enabled("role:toolbar >> label:Junk") })
	a.Press("role:toolbar >> label:Junk")
	a.WaitFor("id:undo >> text:2 messages moved to Junk", 8*time.Second)
	a.WaitForGone(dinner, 8*time.Second)
	a.Press("id:mailbox-role:junk")
	choose(dinner, hiring)
	a.WaitUntil(5*time.Second, func() bool { return a.Enabled("role:toolbar >> label:Not junk") })
	if a.Exists("role:toolbar >> label:Junk") {
		t.Error("with 2 messages of Junk chosen, the toolbar still offers Junk")
	}
	a.Press("role:toolbar >> label:Not junk")
	// The fixture's maild does not sync, so the move to Junk has not
	// reached a server, and maild leaves such a message where it is with
	// no error. The client says so, and claims no move.
	a.WaitFor("text:still moving on the server", 8*time.Second)
	if a.Exists("id:undo >> text:moved to Inbox") {
		t.Error("the client says the messages moved to Inbox, and maild left them in Junk")
	}
	a.Find(dinner)
	a.Find(hiring)
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

// TestUndoArchive uses loopback IMAP so the first move can settle. The
// offline showcase leaves it queued, and maild then skips the inverse move.
func TestUndoArchive(t *testing.T) {
	const subject = "Archive this message"
	for _, tc := range []struct {
		name string
		undo func(*nsgotest.App)
	}{
		{"Toast", func(a *nsgotest.App) { a.Press("id:undo >> role:button Undo") }},
		{"More", func(a *nsgotest.App) {
			a.Press("role:toolbar >> label:More")
			a.Press("role:menuitem Undo")
		}},
		{"Shortcut", func(a *nsgotest.App) { a.Key("ctrl+z") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := syncedWorkspace(t, subject)
			row := "id:messages >> label:" + subject
			a.WaitFor(row, 8*time.Second)
			a.Press(row)
			a.WaitUntil(5*time.Second, func() bool { return a.Enabled("role:toolbar >> label:Archive") })
			a.Press("role:toolbar >> label:Archive")
			a.WaitForGone(row, 8*time.Second)
			a.WaitFor("id:undo >> text:Message archived", 5*time.Second)
			tc.undo(a)
			a.WaitFor(row, 12*time.Second)
		})
	}
}

func syncedWorkspace(t *testing.T, subject string) *nsgotest.App {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	raw := "From: sender@example.test\r\nTo: me@example.test\r\nSubject: " + subject +
		"\r\nMessage-ID: <archive@example.test>\r\n\r\nBody.\r\n"
	if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &mailsync.Config{NoPrefetch: true}})
	c := srv.Dial(t)
	o := mem.DialOptions()
	account, err := c.Account().Create(t.Context(), &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "me@example.test", DisplayName: "Me", Auth: api.AuthKindPassword,
		IMAP: &api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: o.Username},
		SMTP: &api.ServerConfig{Host: o.Host, Port: 1, TLS: o.TLS, Username: o.Username},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(t.Context(), &api.AccountSetPasswordParams{ID: account.ID, Password: o.Password}); err != nil {
		t.Fatal(err)
	}
	// Open the workspace after initial sync, as with the seeded showcase.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		_, err := c.Message().ByMessageID(ctx, &api.MessageByMessageIDParams{MessageID: "archive@example.test"})
		if err == nil {
			break
		}
		if !errors.Is(err, api.ErrNotFound) {
			t.Fatal(err)
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			t.Fatalf("initial IMAP sync did not deliver %q: %v", subject, ctx.Err())
		}
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("FROSTMAIL_SOCKET", srv.Socket)
	return nsgotest.Start(t, root, nsgotest.Size(1280, 820),
		nsgotest.Environment(core.Environment{OS: "linux", Idiom: core.IdiomDesktop, PixelRatio: 1}))
}

// TestComposeOpensWithTheKeyboardInAField checks where the keyboard is as
// Compose opens: in To for a new message, so that text typed at once is
// its first recipient, and not in To for a reply, whose To is filled.
// Seen on Windows before this: text typed after Ctrl+N went nowhere.
func TestComposeOpensWithTheKeyboardInAField(t *testing.T) {
	a := fixtureWorkspace(t)
	a.Press("label:Compose")
	a.WaitFor("label:To", 5*time.Second)
	a.WaitUntil(5*time.Second, func() bool { return a.Find("label:To").Focused })
	a.Key("ctrl+w")
	a.WaitUntil(5*time.Second, func() bool { return !a.Exists("label:Subject") })

	row := "id:messages >> label:Your trip to Lisbon: itinerary"
	a.WaitFor(row, 8*time.Second)
	a.Press(row)
	a.WaitUntil(8*time.Second, func() bool { return a.Enabled("role:toolbar >> label:Reply") })
	a.Press("role:toolbar >> label:Reply")
	a.WaitFor("label:Subject", 8*time.Second)
	a.WaitUntil(5*time.Second, func() bool { return a.Find("label:To").Text != "" })
	if a.Find("label:To").Focused {
		t.Error("a reply, whose To is filled, opens with the keyboard in To")
	}
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
