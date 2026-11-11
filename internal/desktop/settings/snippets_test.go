package settings_test

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/settings"
)

func (f *fixture) snippets() []api.Snippet {
	f.t.Helper()
	list, err := f.c.Snippet().List(f.t.Context(), &api.SnippetListParams{})
	if err != nil {
		f.t.Fatal(err)
	}
	return list
}

func TestSnippetsAddSaveAndRemove(t *testing.T) {
	f := newFixture(t, setup{})
	f.show(settings.PaneSnippets)
	f.sees("Snippets are text you insert in a message with Insert Snippet in the compose window.")
	if !f.disabled("Remove Snippet") {
		t.Error("Remove works with nothing selected")
	}

	f.click("Add Snippet")
	f.sees("New Snippet")
	if !f.disabled("Add") || !f.disabled("Add Snippet") {
		t.Error("Add works with no name, or + while the form is open")
	}
	f.fill("Name:", strings.Repeat("n", 70))
	if got := f.value("Name:"); len(got) != 60 {
		t.Errorf("the name is %d long", len(got))
	}
	f.fill("Name:", " Hello ")
	f.press(f.control(ui.RoleTextField, "Text:"))
	f.tt.Type("Hi there")
	f.click("Add")
	f.until("the new snippet selected", func() bool { return f.tt.HasText("Save") && f.value("Name:") == "Hello" })
	list := f.snippets()
	if len(list) != 1 || list[0].Name != "Hello" || list[0].HTML != "<p>Hi there</p>" {
		t.Fatalf("snippets = %+v", list)
	}
	if !f.disabled("Save") {
		t.Error("Save works with nothing changed")
	}

	f.fill("Name:", "Greeting")
	f.click("Save")
	f.until("the saved name", func() bool { return f.snippets()[0].Name == "Greeting" })
	f.sees("Greeting")

	f.click("Remove Snippet")
	f.sees("Remove the snippet “Greeting”?")
	f.press(f.control(ui.RoleButton, "Remove"))
	f.until("the removal", func() bool { return len(f.snippets()) == 0 })
	f.sees("Snippets are text you insert")
}

func TestSnippetsFollowChangesAndSelection(t *testing.T) {
	f := newFixture(t, setup{})
	f.show(settings.PaneSnippets)
	first, err := f.c.Snippet().Create(t.Context(), &api.SnippetCreateParams{Name: "Alpha", HTML: "<p>one</p>"})
	if err != nil {
		t.Fatal(err)
	}
	f.sees("Alpha")
	f.until("the first selected", func() bool { return f.value("Text:") == "one" })
	if _, err := f.c.Snippet().Create(t.Context(), &api.SnippetCreateParams{Name: "Beta", HTML: "<p>two</p>"}); err != nil {
		t.Fatal(err)
	}
	f.sees("Beta")
	f.press(f.control(ui.RoleButton, "Beta"))
	f.until("Beta's text", func() bool { return f.value("Text:") == "two" })
	if err := f.c.Snippet().Delete(t.Context(), &api.SnippetDeleteParams{ID: first.ID}); err != nil {
		t.Fatal(err)
	}
	f.until("the list without Alpha", func() bool { return !f.tt.HasText("Alpha") })
	if f.value("Text:") != "two" {
		t.Errorf("the selection moved: %q", f.value("Text:"))
	}
}

func TestSnippetAddErrorLeavesTheFormOpen(t *testing.T) {
	f := newFixture(t, setup{})
	if _, err := f.c.Snippet().Create(t.Context(), &api.SnippetCreateParams{Name: "Taken", HTML: ""}); err != nil {
		t.Fatal(err)
	}
	f.show(settings.PaneSnippets)
	f.sees("Taken")
	f.click("Add Snippet")
	f.fill("Name:", "taken")
	f.click("Add")
	f.until("maild's error", func() bool { _, ok := f.tt.AccessByRole(ui.RoleAlert, ""); return ok })
	f.sees("New Snippet")
	f.click("Cancel")
	f.notSeen("New Snippet")
}

func TestAClickOnTheOpenSnippetKeepsItsEdits(t *testing.T) {
	f := newFixture(t, setup{})
	f.show(settings.PaneSnippets)
	if _, err := f.c.Snippet().Create(t.Context(), &api.SnippetCreateParams{Name: "Alpha", HTML: "<p>one</p>"}); err != nil {
		t.Fatal(err)
	}
	f.sees("Alpha")
	f.until("its text", func() bool { return f.value("Text:") == "one" })
	f.fill("Name:", "Edited")
	f.press(f.control(ui.RoleButton, "Alpha"))
	f.tt.Frame()
	if got := f.value("Name:"); got != "Edited" {
		t.Errorf("the name went back to %q", got)
	}
}
