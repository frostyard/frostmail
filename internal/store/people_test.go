package store

// CONTRACT TEST for task card T-0062 (docs/tasks). Do not edit.

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// peopleFixture is three accounts' address books and their contacts.
type peopleFixture struct {
	d                                        *DB
	a, b, c                                  Account
	a1, a2, a3, b1                           Collection
	ada, ada2, adaWork, grace, nomail, ghost int64
	hidden                                   int64
}

func addContact(t *testing.T, d *DB, col int64, href string, c ContactIndex) int64 {
	t.Helper()
	id := putObject(t, d, Object{CollectionID: col, Href: href, Kind: ObjectVCard, Raw: []byte("BEGIN:VCARD\r\nEND:VCARD\r\n")})
	if err := d.Tx(t.Context(), func(tx *Tx) error { return tx.IndexContact(t.Context(), id, c) }); err != nil {
		t.Fatal(err)
	}
	return id
}

func relink(t *testing.T, d *DB) {
	t.Helper()
	if err := d.Tx(t.Context(), func(tx *Tx) error { return tx.RelinkPeople(t.Context()) }); err != nil {
		t.Fatalf("RelinkPeople: %v", err)
	}
}

func newPeopleFixture(t *testing.T) *peopleFixture {
	t.Helper()
	d, _ := openTest(t)
	ctx := t.Context()
	f := &peopleFixture{d: d}
	f.a = insertAccount(t, d, sampleAccount("a@mailtest.test"))
	f.b = insertAccount(t, d, sampleAccount("b@mailtest.test"))
	f.c = insertAccount(t, d, sampleAccount("c@mailtest.test"))
	if err := d.Tx(ctx, func(tx *Tx) error {
		for _, s := range []struct {
			id int64
			on bool
		}{{f.a.ID, true}, {f.b.ID, true}, {f.c.ID, false}} {
			if err := tx.SetService(ctx, s.id, api.ServiceKindContacts, s.on, "https://dav.test/"); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	as := replaceCols(t, d, f.a.ID, api.CollectionKindAddressbook,
		RemoteCollection{Href: "/a1/", Name: "A1"}, RemoteCollection{Href: "/a2/", Name: "A2", ReadOnly: true},
		RemoteCollection{Href: "/a3/", Name: "A3"})
	f.a1, f.a2, f.a3 = as[0], as[1], as[2]
	f.b1 = replaceCols(t, d, f.b.ID, api.CollectionKindAddressbook, RemoteCollection{Href: "/b1/", Name: "B1"})[0]
	c1 := replaceCols(t, d, f.c.ID, api.CollectionKindAddressbook, RemoteCollection{Href: "/c1/", Name: "C1"})[0]
	off := false
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.UpdateCollection(ctx, f.a3.ID, &off, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	f.ada = addContact(t, d, f.a1.ID, "/a1/ada.vcf", ContactIndex{DisplayName: "Ada Lovelace", SortKey: "lovelace ada",
		Emails: []ContactEmail{{Email: "ada@example.com"}}})
	f.ada2 = addContact(t, d, f.b1.ID, "/b1/ada.vcf", ContactIndex{Organization: "Engines",
		Emails: []ContactEmail{{Email: "ADA@example.com"}, {Email: "ada@work.example", Label: "work"}}})
	f.adaWork = addContact(t, d, f.a2.ID, "/a2/aw.vcf", ContactIndex{DisplayName: "Ada at Work",
		Emails: []ContactEmail{{Email: "ada@work.example"}}})
	f.grace = addContact(t, d, f.a1.ID, "/a1/grace.vcf", ContactIndex{DisplayName: "Grace Hopper", SortKey: "hopper grace",
		Emails: []ContactEmail{{Email: "grace@navy.example"}}, Photo: []byte{0xff, 0xd8}, PhotoType: "image/jpeg"})
	f.nomail = addContact(t, d, f.a1.ID, "/a1/nomail.vcf", ContactIndex{DisplayName: "No Mail", SortKey: "no mail"})
	f.ghost = addContact(t, d, c1.ID, "/c1/ghost.vcf", ContactIndex{DisplayName: "Ghost", SortKey: "ghost",
		Emails: []ContactEmail{{Email: "ada@example.com"}}})
	f.hidden = addContact(t, d, f.a3.ID, "/a3/hidden.vcf", ContactIndex{DisplayName: "Hidden", SortKey: "hidden",
		Emails: []ContactEmail{{Email: "hidden@example.com"}}})
	relink(t, d)
	return f
}

func ids(ps []PersonRow) []int64 {
	var out []int64
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func TestRelinkPeople(t *testing.T) {
	f := newPeopleFixture(t)
	ctx := t.Context()
	all, err := f.d.People(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(all), []int64{f.grace, f.ada, f.nomail}) {
		t.Fatalf("people = %+v; want Grace, Ada, No Mail by sort key", all)
	}
	ada := all[1]
	want := PersonRow{ID: f.ada, DisplayName: "Ada Lovelace", SortKey: "lovelace ada", Organization: "Engines",
		Email: "ada@example.com", ContactIDs: []int64{f.ada, f.ada2, f.adaWork}}
	if !personEqual(ada, want) {
		t.Errorf("Ada = %+v\nwant %+v", ada, want)
	}
	if !all[0].HasPhoto || all[0].Email != "grace@navy.example" || all[2].Email != "" || all[2].HasPhoto {
		t.Errorf("Grace, No Mail = %+v, %+v", all[0], all[2])
	}
	got, err := f.d.Person(ctx, f.ada)
	if err != nil || !personEqual(got, want) {
		t.Errorf("Person = %+v, %v", got, err)
	}
	if _, err := f.d.Person(ctx, f.ghost); !errors.Is(err, ErrNotFound) {
		t.Errorf("a contact whose account has contacts off is no person: %v", err)
	}
	if _, err := f.d.Person(ctx, f.hidden); !errors.Is(err, ErrNotFound) {
		t.Errorf("a contact in a disabled address book is no person: %v", err)
	}
	if p, err := f.d.PersonByEmail(ctx, " ADA@Work.example "); err != nil || p.ID != f.ada {
		t.Errorf("PersonByEmail = %+v, %v", p, err)
	}
	if _, err := f.d.PersonByEmail(ctx, "hidden@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("PersonByEmail(hidden) = %v", err)
	}

	// Ada's second card stops sharing her home address: two people, the
	// second named after its first named contact.
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		return tx.IndexContact(ctx, f.ada2, ContactIndex{Organization: "Engines",
			Emails: []ContactEmail{{Email: "ada@work.example"}}})
	}); err != nil {
		t.Fatal(err)
	}
	relink(t, f.d)
	all, _ = f.d.People(ctx, "")
	if !slices.Equal(ids(all), []int64{f.ada2, f.grace, f.ada, f.nomail}) {
		t.Fatalf("people = %+v", all)
	}
	work := all[0]
	if work.DisplayName != "Ada at Work" || work.SortKey != "ada at work" || work.Organization != "Engines" ||
		!slices.Equal(work.ContactIDs, []int64{f.ada2, f.adaWork}) || work.Email != "ada@work.example" {
		t.Errorf("work person = %+v", work)
	}
	if home, _ := f.d.Person(ctx, f.ada); home.Organization != "" || !slices.Equal(home.ContactIDs, []int64{f.ada}) {
		t.Errorf("home person = %+v", home)
	}

	// Deleting a contact leaves its person to the others.
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.DeleteObjects(ctx, f.b1.ID, []string{"/b1/ada.vcf"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	relink(t, f.d)
	if _, err := f.d.Person(ctx, f.ada2); !errors.Is(err, ErrNotFound) {
		t.Errorf("the deleted contact's person: %v", err)
	}
	if p, err := f.d.Person(ctx, f.adaWork); err != nil || p.Organization != "" || p.DisplayName != "Ada at Work" {
		t.Errorf("the remaining person = %+v, %v", p, err)
	}
}

func personEqual(a, b PersonRow) bool {
	return a.ID == b.ID && a.DisplayName == b.DisplayName && a.SortKey == b.SortKey && a.Organization == b.Organization &&
		a.Email == b.Email && a.HasPhoto == b.HasPhoto && slices.Equal(a.ContactIDs, b.ContactIDs)
}

func TestPeopleQuery(t *testing.T) {
	f := newPeopleFixture(t)
	for _, tc := range []struct {
		query string
		want  []int64
	}{
		{"ada", []int64{f.ada}},
		{"LOV", []int64{f.ada}},
		{"eng", []int64{f.ada}},     // the organization
		{"work", []int64{f.ada}},    // a word of an email address
		{"navy", []int64{f.grace}},  // likewise
		{"lov ada", []int64{f.ada}}, // every word must match
		{"lov grace", nil},          // not one person
		{"ovelace", nil},            // words match at their start
		{"hidden", nil},             // a disabled address book
		{"ghost", nil},              // an account with contacts off
		{"  ", []int64{f.grace, f.ada, f.nomail}},
	} {
		got, err := f.d.People(t.Context(), tc.query)
		if err != nil || !slices.Equal(ids(got), tc.want) {
			t.Errorf("People(%q) = %v, %v; want %v", tc.query, ids(got), err, tc.want)
		}
	}
}

func TestPersonPhoto(t *testing.T) {
	f := newPeopleFixture(t)
	data, typ, err := f.d.PersonPhoto(t.Context(), f.grace)
	if err != nil || !slices.Equal(data, []byte{0xff, 0xd8}) || typ != "image/jpeg" {
		t.Errorf("PersonPhoto = %v, %q, %v", data, typ, err)
	}
	if _, _, err := f.d.PersonPhoto(t.Context(), f.ada); !errors.Is(err, ErrNotFound) {
		t.Errorf("no photo: %v", err)
	}
	if _, _, err := f.d.PersonPhoto(t.Context(), 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("no person: %v", err)
	}
}

func TestSuggestContacts(t *testing.T) {
	f := newPeopleFixture(t)
	ctx := t.Context()
	seen := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		if err := tx.RecordAddresses(ctx, []Address{{Addr: "ada@work.example"}}, seen); err != nil {
			return err
		}
		return tx.RecordAddresses(ctx, []Address{{Addr: "grace@navy.example", Name: "Grace H."}}, seen)
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		prefix string
		limit  int
		want   []Address
	}{
		{"a", 0, []Address{{Addr: "ada@work.example", Name: "Ada Lovelace"}, {Addr: "ada@example.com", Name: "Ada Lovelace"}}},
		{"LOVE", 0, []Address{{Addr: "ada@work.example", Name: "Ada Lovelace"}, {Addr: "ada@example.com", Name: "Ada Lovelace"}}},
		{"a", 1, []Address{{Addr: "ada@work.example", Name: "Ada Lovelace"}}},
		{"ho", 0, []Address{{Addr: "grace@navy.example", Name: "Grace Hopper"}}},
		{"hid", 0, nil},
		{"gh", 0, nil},
		{"", 0, nil},
	} {
		got, err := f.d.SuggestContacts(ctx, tc.prefix, tc.limit)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("SuggestContacts(%q, %d) = %+v, %v; want %+v", tc.prefix, tc.limit, got, err, tc.want)
		}
	}
	if n, err := f.d.SeenName(ctx, "GRACE@navy.example"); err != nil || n != "Grace H." {
		t.Errorf("SeenName = %q, %v", n, err)
	}
	if n, err := f.d.SeenName(ctx, "nobody@example.com"); err != nil || n != "" {
		t.Errorf("SeenName(unknown) = %q, %v", n, err)
	}
}

func TestWritableAddressBooks(t *testing.T) {
	f := newPeopleFixture(t)
	ctx := t.Context()
	got, err := f.d.WritableAddressBooks(ctx)
	if err != nil || !slices.Equal(hrefsOf(got), []string{"/a1/", "/b1/"}) {
		t.Errorf("WritableAddressBooks = %q, %v", hrefsOf(got), err)
	}
	ro := true
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.UpdateAccount(ctx, f.a.ID, AccountUpdate{ReadOnly: &ro})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.d.WritableAddressBooks(ctx); !slices.Equal(hrefsOf(got), []string{"/b1/"}) {
		t.Errorf("with account A read-only = %q", hrefsOf(got))
	}
}

func TestMessagesWithAddress(t *testing.T) {
	f := newPeopleFixture(t)
	ctx := t.Context()
	var msgs []int64
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, f.a.ID, []ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		day := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
		hs := []MessageHeader{
			{UID: 1, InternalDate: day(1), Subject: "from Ada", From: Address{Name: "Ada", Addr: "ada@example.com"},
				To: []Address{{Addr: "a@mailtest.test"}}},
			{UID: 2, InternalDate: day(3), Subject: "cc Ada", From: Address{Addr: "a@mailtest.test"},
				To: []Address{{Addr: "x@example.com"}}, Cc: []Address{{Addr: "Ada@Example.com"}}},
			{UID: 3, InternalDate: day(2), Subject: "not Ada", From: Address{Addr: "ada@example.com.evil.test"},
				To: []Address{{Addr: "a@mailtest.test"}}},
			{UID: 4, InternalDate: day(4), Subject: "to Ada at work", From: Address{Addr: "a@mailtest.test"},
				To: []Address{{Addr: "ada@work.example"}}},
		}
		msgs, err = tx.InsertHeaders(ctx, f.a.ID, mbs[0].ID, hs)
		if err != nil {
			return err
		}
		for i, id := range msgs {
			if err := tx.IndexMessage(ctx, id, SearchDocFor(hs[i])); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got, err := f.d.MessagesWithAddress(ctx, "ADA@example.com", 10)
	if err != nil || !slices.Equal(got, []int64{msgs[1], msgs[0]}) {
		t.Errorf("MessagesWithAddress = %v, %v; want %v", got, err, []int64{msgs[1], msgs[0]})
	}
	if got, _ := f.d.MessagesWithAddress(ctx, "ada@example.com", 1); !slices.Equal(got, []int64{msgs[1]}) {
		t.Errorf("limit 1 = %v", got)
	}
	if got, _ := f.d.MessagesWithAddress(ctx, "nobody@example.com", 5); len(got) != 0 {
		t.Errorf("unknown address = %v", got)
	}
}
