package pim

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/rpctest"
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
	f.until("the account", func() bool { return f.m.account(f.acct) != nil })
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

// fakePeople passes calls on to maild, noting them; a test can fail or
// hold some.
type fakePeople struct {
	api.PeopleService
	mu         sync.Mutex
	creates    int
	updates    []api.ContactFields
	photos     int
	photoErr   error // the next SetPhoto fails with it
	restores   int
	restoreErr error         // the next Restore fails with it
	hold       chan struct{} // Restore waits for it, when set
}

func (s *fakePeople) Create(ctx context.Context, p *api.PeopleCreateParams) (*api.ContactWrite, error) {
	s.mu.Lock()
	s.creates++
	s.mu.Unlock()
	return s.PeopleService.Create(ctx, p)
}

func (s *fakePeople) Update(ctx context.Context, p *api.PeopleUpdateParams) (*api.ContactWrite, error) {
	s.mu.Lock()
	s.updates = append(s.updates, p.Fields)
	s.mu.Unlock()
	return s.PeopleService.Update(ctx, p)
}

func (s *fakePeople) SetPhoto(ctx context.Context, p *api.PeopleSetPhotoParams) (*api.ContactWrite, error) {
	s.mu.Lock()
	s.photos++
	err := s.photoErr
	s.photoErr = nil
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return s.PeopleService.SetPhoto(ctx, p)
}

func (s *fakePeople) Restore(ctx context.Context, p *api.PeopleRestoreParams) error {
	s.mu.Lock()
	s.restores++
	err, hold := s.restoreErr, s.hold
	s.restoreErr = nil
	s.mu.Unlock()
	if hold != nil {
		<-hold
	}
	if err != nil {
		return err
	}
	return s.PeopleService.Restore(ctx, p)
}

func (s *fakePeople) restoreCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restores
}

func withFakePeople(fake *fakePeople) options {
	return options{Options: rpctest.Options{Services: func(s *api.Services) {
		fake.PeopleService = s.People
		s.People = fake
	}}}
}

// contactOf is the only contact of the person with that name.
func (f *fixture) contactOf(name string) int64 {
	f.t.Helper()
	ctx := f.t.Context()
	all, err := f.c.People().List(ctx, &api.PeopleListParams{})
	if err != nil {
		f.t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(p api.PersonSummary) bool { return p.DisplayName == name })
	if i < 0 {
		f.t.Fatalf("no %q in %+v", name, all)
	}
	p, err := f.c.People().Get(ctx, &api.PeopleGetParams{ID: all[i].ID})
	if err != nil {
		f.t.Fatal(err)
	}
	return p.Contacts[0].ID
}

// deleted deletes a person's contact and returns the write's restore.
func (f *fixture) deleted(name string) string {
	f.t.Helper()
	w, err := f.c.People().Delete(f.t.Context(), &api.PeopleDeleteParams{ID: f.contactOf(name)})
	if err != nil {
		f.t.Fatal(err)
	}
	return w.RestoreID
}

func (f *fixture) hasPerson(name string) bool {
	f.t.Helper()
	all, err := f.c.People().List(f.t.Context(), &api.PeopleListParams{})
	if err != nil {
		f.t.Fatal(err)
	}
	return slices.ContainsFunc(all, func(p api.PersonSummary) bool { return p.DisplayName == name })
}

// undoAcrossADisconnect presses Undo while disconnected, which must keep
// the entry, and again after the reconnect, which must restore it.
func undoAcrossADisconnect(t *testing.T, f *fixture, undo func(), entries func() int, restored func() bool) {
	t.Helper()
	f.m.Disconnect()
	f.until("the disconnect", func() bool { return f.m.client == nil })
	undo()
	f.tt.Frame()
	if entries() != 1 {
		t.Fatalf("an Undo while disconnected left %d entries", entries())
	}
	if len(f.toasts) != 1 {
		t.Errorf("toasts %q", f.toasts)
	}
	if err := f.m.Connect(t.Context(), f.c, live.NewEvents()); err != nil {
		t.Fatal(err)
	}
	f.until("the connection", func() bool { return f.m.client != nil })
	undo()
	f.until("the restore", func() bool { return entries() == 0 && restored() })
}

func TestUndoWhileDisconnectedKeepsTheEntry(t *testing.T) {
	t.Run("people", func(t *testing.T) {
		f := newFixture(t, commands.People)
		f.text("Alan Turing")
		f.m.people.undo = []string{f.deleted("Alan Turing")}
		undoAcrossADisconnect(t, f, f.m.undoPeople, func() int { return len(f.m.people.undo) }, func() bool { return f.hasPerson("Alan Turing") })
	})
	t.Run("calendar", func(t *testing.T) {
		f := newFixture(t, commands.Calendar)
		f.text("Review")
		r := f.block("Review, ")
		f.drag(r.X+r.W/2, r.Y+10, 0, 24)
		f.until("the write", func() bool { return len(f.m.cal.undo) == 1 })
		undoAcrossADisconnect(t, f, f.m.undoCalendar, func() int { return len(f.m.cal.undo) }, func() bool {
			return f.occurrence("Review", "2026-10-08", "2026-10-09").Start.Equal(time.Date(2026, 10, 8, 14, 0, 0, 0, time.UTC))
		})
	})
}

func TestAFailedUndoKeepsTheEntry(t *testing.T) {
	fake := &fakePeople{restoreErr: errors.New("refused")}
	f := newFixture(t, commands.People, withFakePeople(fake))
	f.text("Alan Turing")
	f.m.people.undo = []string{f.deleted("Alan Turing")}
	f.m.undoPeople()
	f.until("the toast", func() bool { return len(f.toasts) == 1 })
	if len(f.m.people.undo) != 1 {
		t.Fatalf("a failed undo left %v", f.m.people.undo)
	}
	f.m.undoPeople()
	f.until("the restore", func() bool { return len(f.m.people.undo) == 0 && f.hasPerson("Alan Turing") })
}

func TestARestoreMaildNoLongerHoldsLeavesTheList(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Alan Turing")
	older := f.deleted("Alan Turing")
	f.m.people.undo = []string{older, "never-made"}
	f.m.undoPeople()
	f.until("the entry leaving", func() bool { return len(f.m.people.undo) == 1 })
	if f.m.people.undo[0] != older || len(f.toasts) != 1 {
		t.Fatalf("undo %v, toasts %q", f.m.people.undo, f.toasts)
	}
	f.m.undoPeople()
	f.until("the older restore", func() bool { return len(f.m.people.undo) == 0 && f.hasPerson("Alan Turing") })
}

func TestUndosRunOneAtATime(t *testing.T) {
	hold := make(chan struct{})
	fake := &fakePeople{hold: hold}
	f := newFixture(t, commands.People, withFakePeople(fake))
	f.text("Alan Turing")
	alan, grace := f.deleted("Alan Turing"), f.deleted("Grace Hopper")
	f.m.people.undo = []string{alan, grace}
	f.m.undoPeople()
	f.m.undoPeople()
	f.until("the first restore", func() bool { return fake.restoreCount() == 1 })
	if len(f.m.people.undo) != 2 {
		t.Fatalf("a running restore left %v", f.m.people.undo)
	}
	// A write made during the restore moves it from the end of the list.
	f.m.people.undo = remember(f.m.people.undo, "newer")
	close(hold)
	f.until("the restore", func() bool { return len(f.m.people.undo) == 2 && !f.m.people.restoring })
	if fake.restoreCount() != 1 || !slices.Equal(f.m.people.undo, []string{alan, "newer"}) {
		t.Errorf("%d restores, undo %v", fake.restoreCount(), f.m.people.undo)
	}
	if !f.hasPerson("Grace Hopper") || f.hasPerson("Alan Turing") {
		t.Error("restored the wrong contact")
	}
}

func TestANoteOnlyEditLeavesTheLists(t *testing.T) {
	fake := &fakePeople{}
	f := newFixture(t, commands.People, withFakePeople(fake))
	f.text("Alan Turing")
	f.click("Alan Turing")
	f.until("edit", func() bool { return f.tt.HasText("Edit Contact") })
	f.click("Edit Contact")
	f.until("the editor", func() bool { return f.m.Inert() && f.tt.HasText("Done") })
	e := f.m.people.editing
	if len(e.draft.emails) != 1 || e.draft.emails[0].Label != "" {
		t.Fatalf("drawing changed the draft: %+v", e.draft.emails)
	}
	if !f.tt.HasText("home") {
		t.Errorf("an empty label shows no default: %s", dump(f.tt))
	}
	e.draft.note = "A note"
	f.tt.SetSize(1200, 1600)
	f.tt.Frame()
	f.click("Done")
	f.until("the write", func() bool { return f.m.people.editing == nil })
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.updates) != 1 {
		t.Fatalf("updates %+v", fake.updates)
	}
	got := fake.updates[0]
	if got.Note == nil || *got.Note != "A note" {
		t.Errorf("note %v", got.Note)
	}
	got.Note = nil
	if !emptyContactFields(got) {
		t.Errorf("sent more than the note: %+v", got)
	}
}

func scaledPhoto(t *testing.T) *api.Photo {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 80, 60))); err != nil {
		t.Fatal(err)
	}
	photo, err := scalePhoto(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return photo
}

func TestAFailedPhotoThenDoneDoesNotCreateAgain(t *testing.T) {
	fake := &fakePeople{photoErr: errors.New("photo refused")}
	f := newFixture(t, commands.People, withFakePeople(fake))
	f.text("Ada Lovelace")
	f.until("the books", func() bool { return len(f.m.writableBooks()) > 0 })
	f.m.newContact()
	e := f.m.people.editing
	e.draft.givenName = "Dorothy"
	e.photoChanged, e.photo = true, scaledPhoto(t)
	f.tt.SetSize(1200, 1600)
	f.tt.Frame()
	f.click("Done")
	f.until("the failure", func() bool { return strings.Contains(e.failure, "photo refused") && !e.busy })
	if e.isNew || e.contact == nil || f.m.people.editing != e {
		t.Fatalf("after the create: isNew %v, contact %+v", e.isNew, e.contact)
	}
	f.until("the editor on its person", func() bool { return f.tt.HasText("Done") && f.m.shownPerson() != nil })
	f.click("Done")
	f.until("written", func() bool { return f.m.people.editing == nil })
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.creates != 1 || fake.photos != 2 {
		t.Errorf("%d creates, %d photos", fake.creates, fake.photos)
	}
	if len(fake.updates) != 0 {
		t.Errorf("the retry rewrote the fields: %+v", fake.updates)
	}
	if len(f.m.people.undo) != 2 {
		t.Errorf("undo %v", f.m.people.undo)
	}
}

func TestARelinkKeepsTheEditorOnScreen(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Grace Hopper")
	ctx := t.Context()
	grace := f.contactOf("Grace Hopper")
	// A shared address joins Grace's contact to Alan's person.
	if _, err := f.c.People().Update(ctx, &api.PeopleUpdateParams{ID: grace, Fields: api.ContactFields{Emails: []api.LabeledValue{{Value: "alan@a.test"}}}}); err != nil {
		t.Fatal(err)
	}
	f.until("the join", func() bool { return len(f.m.people.list.value) == 2 })
	f.click("Alan Turing")
	f.until("both contacts", func() bool { p := f.m.shownPerson(); return p != nil && len(p.Contacts) == 2 })
	p := f.m.shownPerson()
	f.m.editContact(&p.Contacts[slices.IndexFunc(p.Contacts, func(c api.Contact) bool { return c.ID == grace })])
	f.tt.SetSize(1200, 1600)
	f.until("the editor", func() bool { return f.tt.HasText("Done") })
	// Another device gives the contact its own address again.
	if _, err := f.c.People().Update(ctx, &api.PeopleUpdateParams{ID: grace, Fields: api.ContactFields{Emails: []api.LabeledValue{{Value: "grace@a.test"}}}}); err != nil {
		t.Fatal(err)
	}
	f.until("the split", func() bool { p := f.m.shownPerson(); return p != nil && len(p.Contacts) == 1 })
	f.tt.Frame()
	if !f.m.Inert() || !f.tt.HasText("Done") {
		t.Fatalf("the editor went: inert %v, shows %s", f.m.Inert(), dump(f.tt))
	}
	f.click("Cancel")
	f.until("closed", func() bool { return !f.m.Inert() })
}

func TestDoneWaitsForThePhoto(t *testing.T) {
	f := newFixture(t, commands.People)
	path := filepath.Join(t.TempDir(), "p.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 80, 60))); err != nil {
		t.Fatal(err)
	}
	file.Close()
	release := make(chan struct{})
	f.m.o.OpenImage = func() (string, error) { <-release; return path, nil }
	f.text("Ada Lovelace")
	f.click("Ada Lovelace")
	f.until("edit", func() bool { return f.tt.HasText("Edit Contact") })
	f.click("Edit Contact")
	f.until("the editor", func() bool { return f.m.Inert() })
	f.tt.SetSize(1200, 1600)
	f.tt.Frame()
	f.click("Change Photo")
	e := f.m.people.editing
	if a, _ := f.tt.AccessByRole(ui.RoleButton, "Done"); !e.picking || !a.Disabled {
		t.Errorf("picking %v, Done %+v", e.picking, a)
	}
	e.done()
	if e.busy {
		t.Error("Done ran while the photo was being prepared")
	}
	close(release)
	f.until("the photo", func() bool { return e.photoChanged && !e.picking })
	f.click("Done")
	f.until("written", func() bool { return f.m.people.editing == nil })
	f.until("the person's photo", func() bool { return f.m.shownPerson() != nil && f.m.shownPerson().HasPhoto })
}

func TestACancelledPhotoPickEndsThePick(t *testing.T) {
	f := newFixture(t, commands.People)
	f.m.o.OpenImage = func() (string, error) { return "", nil }
	f.text("Ada Lovelace")
	f.until("the books", func() bool { return len(f.m.writableBooks()) > 0 })
	f.m.newContact()
	e := f.m.people.editing
	e.pickPhoto()
	f.until("the pick's end", func() bool { return !e.picking })
	if e.photoChanged || e.failure != "" {
		t.Errorf("changed %v, failure %q", e.photoChanged, e.failure)
	}
}

func TestAddressBooksWithOneNameStayApart(t *testing.T) {
	f := newFixture(t, commands.People)
	f.text("Ada Lovelace")
	f.until("the books", func() bool { return len(f.m.writableBooks()) > 0 && f.m.account(f.acct) != nil })
	f.m.people.books.value = []api.Collection{{ID: f.book, Name: "Same", AccountID: f.acct}, {ID: f.book + 100, Name: "Same", AccountID: f.acct}}
	f.m.newContact()
	e := f.m.people.editing
	f.click("Same — me@a.test")
	f.click("Same — me@a.test (2)")
	f.until("the second book", func() bool { return e.collectionID == f.book+100 })
	f.tt.Frame()
	if !f.tt.HasText("Same — me@a.test (2)") {
		t.Errorf("the select shows %s", dump(f.tt))
	}
	e.cancel()
}
