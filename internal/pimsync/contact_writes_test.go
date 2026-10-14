package pimsync_test

import (
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/engine"
)

// annCard is a contact as another client wrote it, with a property
// Frostmail does not know.
func annCard(title, note string) []byte {
	return []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:ann\r\nN:Smith;Ann;;;\r\nFN:Ann Smith\r\n" +
		"TITLE:" + title + "\r\nNOTE:" + note + "\r\nEMAIL;TYPE=INTERNET;TYPE=WORK:ann@northwind.test\r\n" +
		"X-CUSTOM:keep me\r\nEND:VCARD\r\n")
}

type contactEnv struct {
	*env
	book   string
	people api.PeopleService
}

func newContactEnv(t *testing.T) *contactEnv {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	e.dav.Put(book+"ann.vcf", annCard("Head of Operations", "Runs the offsite"))
	e.pass()
	return &contactEnv{env: e, book: book, people: engine.New(engine.Deps{DB: e.db, PIM: e.m}).People()}
}

// contact is the ID of the contact whose person has the name.
func (e *contactEnv) contact(name string) int64 {
	e.t.Helper()
	people, err := e.db.People(e.t.Context(), "")
	if err != nil {
		e.t.Fatal(err)
	}
	for _, p := range people {
		if p.DisplayName == name {
			return p.ContactIDs[0]
		}
	}
	e.t.Fatalf("no person %q in %+v", name, people)
	return 0
}

func (e *contactEnv) server(path string) string {
	e.t.Helper()
	data, _, ok := e.dav.Object(path)
	if !ok {
		e.t.Fatalf("%s is not on the server", path)
	}
	return string(data)
}

// refused is the people.refused events of the last pass.
func (e *contactEnv) refused() []api.PeopleRefused {
	var out []api.PeopleRefused
	for _, ev := range e.events {
		if ev.Event != "people.refused" {
			continue
		}
		var r api.PeopleRefused
		if err := json.Unmarshal(ev.Data, &r); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func code(err error) api.ErrorCode {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

func ptr2[T any](v T) *T { return &v }

func TestContactEditsReachTheServer(t *testing.T) {
	e := newContactEnv(t)
	ctx := t.Context()
	ann := e.contact("Ann Smith")

	// Two edits before a pass make one put with both.
	w, err := e.people.Update(ctx, &api.PeopleUpdateParams{ID: ann, Fields: api.ContactFields{Title: ptr2("COO")}})
	if err != nil || w.Person == nil || w.Person.Contacts[0].Title != "COO" || w.RestoreID == "" || w.ContactID != ann {
		t.Fatalf("update = %+v, %v", w, err)
	}
	if _, err := e.people.Update(ctx, &api.PeopleUpdateParams{ID: ann, Fields: api.ContactFields{Note: ptr2("Back Monday")}}); err != nil {
		t.Fatal(err)
	}
	if ops, _ := e.db.DuePIMOps(ctx, e.acct.ID, e.db.Now()); len(ops) != 1 {
		t.Errorf("queued %d ops; want one put", len(ops))
	}
	e.pass()
	got := e.server(e.book + "ann.vcf")
	for _, line := range []string{"TITLE:COO\r\n", "NOTE:Back Monday\r\n", "X-CUSTOM:keep me\r\n", "UID:ann\r\n"} {
		if !strings.Contains(got, line) {
			t.Errorf("the server's card lacks %q:\n%s", line, got)
		}
	}
	obj, _ := e.db.GetObject(ctx, ann)
	if _, etag, _ := e.dav.Object(e.book + "ann.vcf"); obj.ETag != etag || string(obj.Raw) != got {
		t.Errorf("local copy %q (etag %q) differs from the server's (%q)", obj.Raw, obj.ETag, etag)
	}

	photo := base64.StdEncoding.EncodeToString([]byte("\xff\xd8\xff\xe0 a jpeg"))
	if w, err := e.people.SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: ann,
		Photo: &api.Photo{ContentType: "image/jpeg", Data: photo}}); err != nil || !w.Person.HasPhoto {
		t.Fatalf("set photo = %+v, %v", w, err)
	}
	e.pass()
	if !strings.Contains(e.server(e.book+"ann.vcf"), "PHOTO;ENCODING=b;TYPE=JPEG:") {
		t.Error("the photo did not reach the server")
	}
}

func TestContactCreateAndDelete(t *testing.T) {
	e := newContactEnv(t)
	ctx := t.Context()
	w, err := e.people.Create(ctx, &api.PeopleCreateParams{Fields: api.ContactFields{GivenName: ptr2("Kai"),
		FamilyName: ptr2("Berg"), Emails: []api.LabeledValue{{Label: "home", Value: "kai@berg.test"}}}})
	if err != nil || w.Person == nil || w.Person.DisplayName != "Kai Berg" {
		t.Fatalf("create = %+v, %v", w, err)
	}
	e.pass()
	obj, _ := e.db.GetObject(ctx, w.ContactID)
	if got := e.server(obj.Href); !strings.Contains(got, "FN:Kai Berg\r\n") ||
		!strings.Contains(got, "EMAIL;TYPE=INTERNET;TYPE=HOME:kai@berg.test\r\n") {
		t.Errorf("the new card on the server = %s", got)
	}
	gone, err := e.people.Delete(ctx, &api.PeopleDeleteParams{ID: w.ContactID})
	if err != nil || gone.Person != nil {
		t.Fatalf("delete = %+v, %v", gone, err)
	}
	e.pass()
	if _, _, ok := e.dav.Object(obj.Href); ok {
		t.Error("the deleted card is still on the server")
	}

	// A contact the server never had is only dropped.
	temp, _ := e.people.Create(ctx, &api.PeopleCreateParams{Fields: api.ContactFields{GivenName: ptr2("Temp")}})
	if _, err := e.people.Delete(ctx, &api.PeopleDeleteParams{ID: temp.ContactID}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	if i := slices.IndexFunc(e.dav.Requests(), func(r davtest.Request) bool { return r.Method == "PUT" || r.Method == "DELETE" }); i >= 0 {
		t.Errorf("a write for a contact never written: %+v", e.dav.Requests()[i])
	}
}

// Another client's change to other fields: the edit is applied again to
// the server's version.
func TestContactEditAppliedAgainAfterAnotherClient(t *testing.T) {
	e := newContactEnv(t)
	ctx := t.Context()
	ann := e.contact("Ann Smith")
	if _, err := e.people.Update(ctx, &api.PeopleUpdateParams{ID: ann, Fields: api.ContactFields{Title: ptr2("COO")}}); err != nil {
		t.Fatal(err)
	}
	e.dav.Put(e.book+"ann.vcf", annCard("Head of Operations", "Moved to Lisbon"))
	e.pass()
	got := e.server(e.book + "ann.vcf")
	if !strings.Contains(got, "TITLE:COO\r\n") || !strings.Contains(got, "NOTE:Moved to Lisbon\r\n") {
		t.Errorf("the server's card = %s; want both changes", got)
	}
	if obj, _ := e.db.GetObject(ctx, ann); string(obj.Raw) != got {
		t.Errorf("local = %s", obj.Raw)
	}
	if r := e.refused(); len(r) != 0 {
		t.Errorf("refused = %+v", r)
	}
}

// Another client's change to the same field: the server's version stands,
// and the app hears so.
func TestContactEditRefusedWhenTheFieldChanged(t *testing.T) {
	e := newContactEnv(t)
	ctx := t.Context()
	ann := e.contact("Ann Smith")
	if _, err := e.people.Update(ctx, &api.PeopleUpdateParams{ID: ann, Fields: api.ContactFields{Title: ptr2("COO")}}); err != nil {
		t.Fatal(err)
	}
	e.dav.Put(e.book+"ann.vcf", annCard("Chief of Staff", "Runs the offsite"))
	e.pass()
	if got := e.server(e.book + "ann.vcf"); !strings.Contains(got, "TITLE:Chief of Staff\r\n") {
		t.Errorf("the server's card = %s", got)
	}
	p, err := e.people.Get(ctx, &api.PeopleGetParams{ID: mustPerson(t, e, ann)})
	if err != nil || p.Contacts[0].Title != "Chief of Staff" {
		t.Errorf("local contact = %+v, %v", p, err)
	}
	r := e.refused()
	if len(r) != 1 || r[0].Reason != "changed" || r[0].Name != "Ann Smith" || r[0].ContactID == nil {
		t.Errorf("refused = %+v", r)
	}
	if ops, _ := e.db.DuePIMOps(ctx, e.acct.ID, e.db.Now()); len(ops) != 0 {
		t.Errorf("the refused put is still queued: %+v", ops)
	}
}

func TestContactEditOfAContactDeletedElsewhere(t *testing.T) {
	e := newContactEnv(t)
	ctx := t.Context()
	ann := e.contact("Ann Smith")
	if _, err := e.people.Update(ctx, &api.PeopleUpdateParams{ID: ann, Fields: api.ContactFields{Title: ptr2("COO")}}); err != nil {
		t.Fatal(err)
	}
	e.dav.Delete(e.book + "ann.vcf")
	e.pass()
	if _, err := e.db.GetObject(ctx, ann); err == nil {
		t.Error("the contact is still here")
	}
	if r := e.refused(); len(r) != 1 || r[0].Reason != "deleted" || r[0].Name != "Ann Smith" {
		t.Errorf("refused = %+v", r)
	}
}

func TestContactRestore(t *testing.T) {
	e := newContactEnv(t)
	ctx := t.Context()
	ann := e.contact("Ann Smith")
	before := e.server(e.book + "ann.vcf")

	w, _ := e.people.Update(ctx, &api.PeopleUpdateParams{ID: ann, Fields: api.ContactFields{Title: ptr2("COO")}})
	e.pass()
	if err := e.people.Restore(ctx, &api.PeopleRestoreParams{RestoreID: w.RestoreID}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	if got := e.server(e.book + "ann.vcf"); got != before {
		t.Errorf("after the restore the server has %s; want %s", got, before)
	}
	if err := e.people.Restore(ctx, &api.PeopleRestoreParams{RestoreID: w.RestoreID}); code(err) != api.CodeNotFound {
		t.Errorf("a second restore = %v", err)
	}

	gone, _ := e.people.Delete(ctx, &api.PeopleDeleteParams{ID: ann})
	e.pass()
	if err := e.people.Restore(ctx, &api.PeopleRestoreParams{RestoreID: gone.RestoreID}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	if got := e.server(e.book + "ann.vcf"); got != before {
		t.Errorf("after restoring the delete the server has %s", got)
	}
	e.contact("Ann Smith")

	made, _ := e.people.Create(ctx, &api.PeopleCreateParams{Fields: api.ContactFields{GivenName: ptr2("Kai")}})
	if err := e.people.Restore(ctx, &api.PeopleRestoreParams{RestoreID: made.RestoreID}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	if slices.ContainsFunc(e.dav.Requests(), func(r davtest.Request) bool { return r.Method == "PUT" }) {
		t.Error("an undone creation was written")
	}
}

func TestRestoresLastADay(t *testing.T) {
	e := newContactEnv(t)
	ctx := t.Context()
	w, _ := e.people.Update(ctx, &api.PeopleUpdateParams{ID: e.contact("Ann Smith"), Fields: api.ContactFields{Title: ptr2("COO")}})
	now := time.Now()
	e.db.Now = func() time.Time { return now.Add(25 * time.Hour) }
	e.pass()
	if err := e.people.Restore(ctx, &api.PeopleRestoreParams{RestoreID: w.RestoreID}); code(err) != api.CodeNotFound {
		t.Errorf("a day-old restore = %v", err)
	}
}

func TestContactWritesRefuseWhatCannotBeWritten(t *testing.T) {
	e := newContactEnv(t)
	ctx := t.Context()
	ann := e.contact("Ann Smith")
	for _, tc := range []struct {
		name string
		err  error
		want api.ErrorCode
	}{
		{"a bad email", func() error {
			_, err := e.people.Update(ctx, &api.PeopleUpdateParams{ID: ann,
				Fields: api.ContactFields{Emails: []api.LabeledValue{{Value: "ann at home"}}}})
			return err
		}(), api.CodeInvalidParams},
		{"a bad birthday", func() error {
			_, err := e.people.Update(ctx, &api.PeopleUpdateParams{ID: ann, Fields: api.ContactFields{Birthday: ptr2("1990-02-30")}})
			return err
		}(), api.CodeInvalidParams},
		{"a GIF", func() error {
			_, err := e.people.SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: ann, Photo: &api.Photo{ContentType: "image/gif", Data: "R0lG"}})
			return err
		}(), api.CodeInvalidParams},
		{"a big photo", func() error {
			big := base64.StdEncoding.EncodeToString(make([]byte, 300<<10))
			_, err := e.people.SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: ann, Photo: &api.Photo{ContentType: "image/png", Data: big}})
			return err
		}(), api.CodeInvalidParams},
		{"no name", func() error {
			_, err := e.people.Create(ctx, &api.PeopleCreateParams{Fields: api.ContactFields{Note: ptr2("nobody")}})
			return err
		}(), api.CodeInvalidParams},
		{"an unknown contact", func() error {
			_, err := e.people.Update(ctx, &api.PeopleUpdateParams{ID: 999, Fields: api.ContactFields{Title: ptr2("x")}})
			return err
		}(), api.CodeNotFound},
	} {
		if code(tc.err) != tc.want {
			t.Errorf("%s: %v; want code %d", tc.name, tc.err, tc.want)
		}
	}

	e.dav.SetReadOnly(e.book, true)
	e.pass()
	if _, err := e.people.Update(ctx, &api.PeopleUpdateParams{ID: ann, Fields: api.ContactFields{Title: ptr2("x")}}); code(err) != api.CodeConflict {
		t.Errorf("an edit in a read-only book = %v", err)
	}
}

func mustPerson(t *testing.T, e *contactEnv, contactID int64) int64 {
	t.Helper()
	row, err := e.db.PersonOfContact(t.Context(), contactID)
	if err != nil {
		t.Fatal(err)
	}
	return row.ID
}

// A delete made on an old ETag is refused: the contact comes back as the
// other client left it, and the app hears so.
func TestContactDeleteRefusedAfterAnotherClient(t *testing.T) {
	e := newContactEnv(t)
	ctx := t.Context()
	if _, err := e.people.Delete(ctx, &api.PeopleDeleteParams{ID: e.contact("Ann Smith")}); err != nil {
		t.Fatal(err)
	}
	e.dav.Put(e.book+"ann.vcf", annCard("COO", "Runs the offsite"))
	e.pass()
	if r := e.refused(); len(r) != 1 || r[0].Reason != "changed" || r[0].Name != "Ann Smith" {
		t.Errorf("refused = %+v", r)
	}
	if !strings.Contains(e.server(e.book+"ann.vcf"), "TITLE:COO") {
		t.Error("the server lost the other client's card")
	}
	e.pass()
	p, err := e.people.Get(ctx, &api.PeopleGetParams{ID: mustPerson(t, e, e.contact("Ann Smith"))})
	if err != nil || p.Contacts[0].Title != "COO" {
		t.Errorf("after the next pass the contact is %+v, %v", p, err)
	}
}
