package pim

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/store"
)

func TestPeopleListLetteredAndSelection(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	for _, want := range []string{"All Contacts", "3 contacts", "L", "T", "H", "Contacts", "me@a.test", "No Contact Selected", "Search Contacts"} {
		f.text(want) // the address books can come after the people
	}
	f.click("Ada Lovelace")
	f.until("the pane", func() bool { return f.tt.HasText("Countess · Engines") })
	for _, want := range []string{"Message", "ada@a.test", "+44 20 7946 0000", "December 10, 1815", "Notes on engines", "CONTACTS — ME@A.TEST", "Edit Contact"} {
		f.until(want, func() bool { return f.tt.HasText(want) })
	}
	if f.m.Selected(commands.People) != 1 {
		t.Error("Selected")
	}
	f.click("ada@a.test")
	if !slices.Contains(f.compose, "ada@a.test") {
		t.Errorf("compose %v", f.compose)
	}
	snapshot(t, f.tt, "people")
}

func TestPeopleKeysMoveTheSelection(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.click("Grace Hopper")
	f.until("selected", func() bool { return f.m.people.selected != 0 })
	first := f.m.people.selected
	f.tt.Key(0, ui.KeyDown)
	f.until("moved", func() bool { return f.m.people.selected != first })
}

func TestPeopleSearchAfterAPause(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.tt.Key(ui.Cmd|ui.Alt, ui.KeyF)
	f.tt.Frame()
	f.tt.Type("ada")
	f.tt.Frame()
	f.until("the filtered list", func() bool { return f.tt.HasText("1 results") && !f.tt.HasText("Alan Turing") })
	f.tt.Key(0, ui.KeyEscape)
	f.until("the whole list", func() bool { return f.tt.HasText("3 contacts") })
	f.m.people.draft, f.m.people.query = "zzzz", "zzzz"
	f.until("no results", func() bool { return f.tt.HasText("No Results") })
}

func TestAddressBookSelectsTheSource(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.click("Card, Read-only")
	f.until("the empty book", func() bool { return f.tt.HasText("No Contacts") })
	if f.m.people.source == "all" {
		t.Error("source")
	}
}

func TestEditingAContactSendsTheChange(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.click("Ada Lovelace")
	f.until("edit", func() bool { return f.tt.HasText("Edit Contact") })
	f.click("Edit Contact")
	f.until("the editor", func() bool { return f.m.Inert() && f.tt.HasText("Done") })
	e := f.m.people.editing
	if e.draft.givenName != "Ada" {
		t.Fatalf("%+v", e.draft)
	}
	for _, want := range []string{"first", "last", "company", "job title", "add email", "add phone", "add address", "add URL", "Birthday month", "note"} {
		if !f.tt.HasText(want) {
			t.Errorf("no %q in %s", want, dump(f.tt))
		}
	}
	snapshot(t, f.tt, "people-editor")
	f.click("add phone")
	if got := e.draft.phones[len(e.draft.phones)-1].Label; got == "" {
		t.Error("a new row has no label")
	}
	e.draft.phones[len(e.draft.phones)-1].Value = "555"
	e.draft.nickname = "Addie"
	f.tt.SetSize(1200, 1600)
	f.tt.Frame()
	f.click("Done")
	f.until("the write", func() bool { return f.m.people.editing == nil })
	f.until("the new nickname", func() bool { return f.tt.HasText("Addie") })
	if len(f.m.people.undo) != 1 {
		t.Errorf("undo %v", f.m.people.undo)
	}
	f.tt.Key(ui.Cmd, ui.KeyZ)
	f.until("the undo", func() bool { return !f.tt.HasText("Addie") })
}

func TestEditorValidatesAndCancels(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.click("Ada Lovelace")
	f.until("edit", func() bool { return f.tt.HasText("Edit Contact") })
	f.click("Edit Contact")
	f.until("the editor", func() bool { return f.m.Inert() })
	e := f.m.people.editing
	e.draft.emails[0].Value = "broken"
	f.tt.Frame()
	if !f.tt.HasText("Enter an address like name@example.com.") {
		t.Errorf("shows %s", dump(f.tt))
	}
	a, ok := f.tt.AccessByRole(ui.RoleButton, "Done")
	if !ok || !a.Disabled {
		t.Errorf("Done: %+v", a)
	}
	e.draft.birthday = birthdayParts{month: "02", day: "30"}
	f.tt.Frame()
	if !f.tt.HasText("This day doesn't exist.") {
		t.Error("no birthday message")
	}
	f.tt.Key(0, ui.KeyEscape)
	f.until("closed", func() bool { return !f.m.Inert() })
}

func TestNewContactAndDelete(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.click("Ada Lovelace")
	f.until("selected", func() bool { return f.m.people.selected != 0 })
	before := f.m.people.selected
	f.until("the books", func() bool { return len(f.m.writableBooks()) > 0 })
	f.tt.Key(ui.Cmd, ui.KeyN)
	f.tt.SetSize(1200, 1600)
	f.until("the editor", func() bool { return f.tt.HasText("NEW CONTACT") })
	e := f.m.people.editing
	if f.m.people.selected != 0 || !e.isNew {
		t.Error("the selection stayed")
	}
	if a, _ := f.tt.AccessByRole(ui.RoleButton, "Done"); !a.Disabled {
		t.Error("Done enabled with nothing entered")
	}
	f.click("Cancel")
	f.until("back", func() bool { return f.m.people.selected == before && !f.m.Inert() })
	f.m.newContact()
	e = f.m.people.editing
	e.draft.givenName = "Dorothy"
	f.tt.Frame()
	f.click("Done")
	f.until("the new person", func() bool {
		return f.tt.HasText("Dorothy") && f.m.people.editing == nil && f.m.people.selected != before && f.m.people.selected != 0
	})
	f.until("edit", func() bool { return f.tt.HasText("Edit Contact") })
	f.click("Edit Contact")
	f.until("the editor", func() bool { return f.m.Inert() })
	f.click("Delete Contact")
	f.until("the question", func() bool { return f.shell.Dialogs.Showing() && f.tt.HasText("Delete Contact?") })
	if !slices.ContainsFunc(f.tt.Texts(), func(s string) bool {
		return strings.Contains(s, `Delete "Dorothy" from Contacts — me@a.test?`) && strings.Contains(s, "every device")
	}) {
		t.Errorf("shows %s", dump(f.tt))
	}
	f.click("Delete")
	f.until("gone", func() bool { return !f.tt.HasText("Dorothy") && f.m.people.selected == 0 })
}

func TestPhotoWellPicksAndWrites(t *testing.T) {
	f := newFixture(t, commands.People)
	path := filepath.Join(t.TempDir(), "p.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 800, 600))); err != nil {
		t.Fatal(err)
	}
	file.Close()
	f.m.o.OpenImage = func() (string, error) { return path, nil }
	f.text("Ada Lovelace")
	f.click("Ada Lovelace")
	f.until("edit", func() bool { return f.tt.HasText("Edit Contact") })
	f.click("Edit Contact")
	f.until("the editor", func() bool { return f.m.Inert() })
	f.tt.SetSize(1200, 1600)
	f.tt.Frame()
	f.click("Change Photo")
	e := f.m.people.editing
	f.until("the photo", func() bool { return e.photoChanged && e.preview != nil && f.tt.HasText("Remove Photo") })
	f.click("Done")
	f.until("written", func() bool { return f.m.people.editing == nil })
	f.until("the person's photo", func() bool { return f.m.shownPerson() != nil && f.m.shownPerson().HasPhoto })
	got, err := f.c.People().Photo(t.Context(), &api.PeoplePhotoParams{ID: f.m.people.selected})
	if err != nil || got.ContentType != "image/jpeg" {
		t.Errorf("%+v, %v", got, err)
	}
}

func TestShowPerson(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.m.people.source = bookKey(f.book)
	f.m.ShowPerson(f.ada)
	f.until("the person", func() bool { return f.tt.HasText("Countess · Engines") })
	if f.m.people.source != "all" || !slices.Contains(f.shown, commands.People) {
		t.Error("ShowPerson")
	}
}

func TestSelectionEndsWhenThePersonIsGone(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.click("Alan Turing")
	f.until("selected", func() bool { return f.m.shownPerson() != nil })
	id := f.m.shownPerson().Contacts[0].ID
	if _, err := f.c.People().Delete(t.Context(), &api.PeopleDeleteParams{ID: id}); err != nil {
		t.Fatal(err)
	}
	f.until("the empty pane", func() bool { return f.tt.HasText("No Contact Selected") && !f.tt.HasText("Alan Turing") })
}

func TestPersonPaneRecentMailAndUpcoming(t *testing.T) {
	f := newFixture(t, commands.People)
	ctx := t.Context()
	var id int64
	err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, f.acct, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		d := testNow.Add(-time.Hour)
		h := store.MessageHeader{UID: 1, Subject: "Mail from Ada", MessageID: "e@x", From: store.Address{Name: "Ada", Addr: "ada@a.test"}, To: []store.Address{{Addr: "me@a.test"}}, Date: d, InternalDate: d}
		ids, err := tx.InsertHeaders(ctx, f.acct, mbs[0].ID, []store.MessageHeader{h})
		if err != nil {
			return err
		}
		return tx.IndexMessage(ctx, ids[0], store.SearchDocFor(h))
	})
	if err != nil {
		t.Fatal(err)
	}
	f.text("Ada Lovelace")
	f.click("Ada Lovelace")
	f.until("Recent Mail", func() bool { return f.tt.HasText("RECENT MAIL") && f.tt.HasText("Mail from Ada") })
	if !f.tt.HasText("UPCOMING") {
		t.Errorf("no Upcoming: %s", dump(f.tt))
	}
	f.click("Mail from Ada")
	f.until("opened", func() bool { return len(f.opened) == 1 })
	id = f.opened[0]
	if id == 0 {
		t.Error("no message")
	}
	var upcoming string
	for _, s := range f.tt.Texts() {
		if strings.HasPrefix(s, "Lunch with Ada,") {
			upcoming = s
		}
	}
	f.click(upcoming)
	f.until("Calendar", func() bool { return slices.Contains(f.shown, commands.Calendar) && f.m.cal.date == "2026-10-12" })
}

func TestCustomLabelBecomesAField(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.click("Ada Lovelace")
	f.until("edit", func() bool { return f.tt.HasText("Edit Contact") })
	f.click("Edit Contact")
	f.until("the editor", func() bool { return f.m.Inert() })
	e := f.m.people.editing
	if !f.tt.HasText("Label for phone 1") || !f.tt.HasText("Email 1") || !f.tt.HasText("Remove email 1") {
		t.Errorf("names: %s", dump(f.tt))
	}
	e.custom["phone0"] = true
	f.tt.Frame()
	if !f.tt.HasText("Custom label for phone 1") {
		t.Errorf("no field: %s", dump(f.tt))
	}
	f.click("Remove email 1")
	if len(e.draft.emails) != 0 {
		t.Errorf("%+v", e.draft.emails)
	}
}

func TestEditingKeepsTheListInert(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.click("Ada Lovelace")
	f.until("edit", func() bool { return f.tt.HasText("Edit Contact") })
	f.click("Edit Contact")
	f.until("the editor", func() bool { return f.m.Inert() })
	selected := f.m.people.selected
	r, ok := f.tt.Find("Grace Hopper")
	if !ok {
		t.Fatal("no row")
	}
	f.tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	f.tt.Frame()
	if f.m.people.selected != selected {
		t.Error("the list took a click while editing")
	}
	f.m.people.editing.cancel()
}

func TestTabCyclesThePeoplePanes(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.tt.Key(0, ui.KeyTab)
	f.tt.Frame()
	if f.m.people.focus != panePerson {
		t.Errorf("Tab went to %q", f.m.people.focus)
	}
}
