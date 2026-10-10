package store

// CONTRACT TEST for task card T-0061 (docs/tasks). Do not edit.

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func replaceCols(t *testing.T, d *DB, acct int64, kind api.CollectionKind, list ...RemoteCollection) []Collection {
	t.Helper()
	var out []Collection
	err := d.Tx(t.Context(), func(tx *Tx) error {
		var err error
		out, err = tx.ReplaceCollections(t.Context(), acct, kind, list)
		return err
	})
	if err != nil {
		t.Fatalf("ReplaceCollections: %v", err)
	}
	return out
}

func putObject(t *testing.T, d *DB, o Object) int64 {
	t.Helper()
	var id int64
	err := d.Tx(t.Context(), func(tx *Tx) error {
		var err error
		id, err = tx.PutObject(t.Context(), o)
		return err
	})
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	return id
}

func hrefsOf(cs []Collection) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Href)
	}
	return out
}

func TestReplaceCollections(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	a := insertAccount(t, d, sampleAccount("one@mailtest.test"))
	b := insertAccount(t, d, sampleAccount("two@mailtest.test"))
	*events = nil

	got := replaceCols(t, d, a.ID, api.CollectionKindAddressbook,
		RemoteCollection{Href: "/ab/shared/", Name: "Shared", ReadOnly: true},
		RemoteCollection{Href: "/ab/default/", Name: "Contacts", Description: "Mine"},
	)
	if !slices.Equal(hrefsOf(got), []string{"/ab/shared/", "/ab/default/"}) {
		t.Fatalf("collections = %+v", got)
	}
	shared, def := got[0], got[1]
	if shared.Position != 0 || def.Position != 1 || !shared.Enabled || !def.Enabled ||
		shared.AccountID != a.ID || shared.Kind != api.CollectionKindAddressbook || def.Description != "Mine" {
		t.Errorf("inserted = %+v", got)
	}
	if shared.IsDefault || !def.IsDefault {
		t.Errorf("default: the first writable collection; got shared %v, default %v", shared.IsDefault, def.IsDefault)
	}
	if len(*events) != 1 || (*events)[0].Event != "account.changed" {
		t.Errorf("events = %+v, want one account.changed", *events)
	}

	// Calendars of another kind and account are separate.
	cals := replaceCols(t, d, a.ID, api.CollectionKindCalendar,
		RemoteCollection{Href: "/cal/work/", Name: "Work", Color: "#3366cc", Components: []string{"VEVENT", "VTODO"}})
	if len(cals) != 1 || !slices.Equal(cals[0].Components, []string{"VEVENT", "VTODO"}) || cals[0].Color != "#3366cc" || !cals[0].IsDefault {
		t.Errorf("calendars = %+v", cals)
	}
	replaceCols(t, d, b.ID, api.CollectionKindAddressbook, RemoteCollection{Href: "/ab/default/", Name: "B's"})

	// The user's settings and the sync state survive a refresh; the
	// server's fields follow the server.
	disabled := false
	if err := d.Tx(ctx, func(tx *Tx) error {
		if _, err := tx.UpdateCollection(ctx, def.ID, &disabled, false); err != nil {
			return err
		}
		return tx.SetCollectionSync(ctx, def.ID, "tok-1", "ctag-1")
	}); err != nil {
		t.Fatal(err)
	}
	*events = nil
	same := replaceCols(t, d, a.ID, api.CollectionKindAddressbook,
		RemoteCollection{Href: "/ab/shared/", Name: "Shared", ReadOnly: true},
		RemoteCollection{Href: "/ab/default/", Name: "Contacts", Description: "Mine"},
	)
	if len(*events) != 0 {
		t.Errorf("an unchanged list emitted %+v", *events)
	}
	if same[1].ID != def.ID || same[1].Enabled || same[1].SyncToken != "tok-1" || same[1].CTag != "ctag-1" || same[1].SyncedAt == nil {
		t.Errorf("after refresh = %+v", same[1])
	}

	objID := putObject(t, d, Object{CollectionID: shared.ID, Href: "/ab/shared/x.vcf", ETag: `"1"`, Kind: ObjectVCard, Raw: []byte("x")})
	if err := d.Tx(ctx, func(tx *Tx) error {
		return tx.IndexContact(ctx, objID, ContactIndex{DisplayName: "X", Emails: []ContactEmail{{Email: "x@example.com"}}})
	}); err != nil {
		t.Fatal(err)
	}
	renamed := replaceCols(t, d, a.ID, api.CollectionKindAddressbook,
		RemoteCollection{Href: "/ab/default/", Name: "Renamed", Color: "#ff0000"},
		RemoteCollection{Href: "/ab/new/", Name: "New"},
	)
	if !slices.Equal(hrefsOf(renamed), []string{"/ab/default/", "/ab/new/"}) || renamed[0].Name != "Renamed" ||
		renamed[0].Color != "#ff0000" || renamed[0].Position != 0 || renamed[1].Position != 1 || renamed[0].Enabled {
		t.Errorf("after rename = %+v", renamed)
	}
	if !renamed[0].IsDefault || renamed[1].IsDefault {
		t.Errorf("the default moved: %+v", renamed)
	}
	if len(*events) != 1 {
		t.Errorf("events = %d, want one account.changed", len(*events))
	}
	// The removed collection took its objects and contacts along.
	if _, err := d.GetObject(ctx, objID); !errors.Is(err, ErrNotFound) {
		t.Errorf("object of a removed collection: %v", err)
	}
	if _, err := d.Contact(ctx, objID); !errors.Is(err, ErrNotFound) {
		t.Errorf("contact of a removed collection: %v", err)
	}
	if _, err := d.GetCollection(ctx, shared.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed collection: %v", err)
	}

	all, err := d.Collections(ctx, CollectionFilter{})
	if err != nil || len(all) != 4 || all[3].AccountID != b.ID {
		t.Errorf("all collections = %+v, %v", all, err)
	}
	enabled, _ := d.Collections(ctx, CollectionFilter{AccountID: a.ID, Kind: api.CollectionKindAddressbook, EnabledOnly: true})
	if !slices.Equal(hrefsOf(enabled), []string{"/ab/new/"}) {
		t.Errorf("enabled address books = %q", hrefsOf(enabled))
	}
}

func TestUpdateCollection(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	a := insertAccount(t, d, sampleAccount("one@mailtest.test"))
	got := replaceCols(t, d, a.ID, api.CollectionKindCalendar,
		RemoteCollection{Href: "/c/1/", Name: "One"}, RemoteCollection{Href: "/c/2/", Name: "Two"})
	cal := replaceCols(t, d, a.ID, api.CollectionKindAddressbook, RemoteCollection{Href: "/ab/", Name: "Book"})
	*events = nil
	var two Collection
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		two, err = tx.UpdateCollection(ctx, got[1].ID, nil, true)
		return err
	})
	if err != nil || !two.IsDefault || !two.Enabled {
		t.Fatalf("UpdateCollection = %+v, %v", two, err)
	}
	one, _ := d.GetCollection(ctx, got[0].ID)
	book, _ := d.GetCollection(ctx, cal[0].ID)
	if one.IsDefault || !book.IsDefault {
		t.Errorf("only the same account's same kind loses its default: one %v, book %v", one.IsDefault, book.IsDefault)
	}
	if len(*events) != 1 || (*events)[0].Event != "account.changed" {
		t.Errorf("events = %+v", *events)
	}
	// Hiding a collection also tells its domain; setting it as it is does not.
	for _, c := range []struct {
		id      int64
		enabled bool
		want    []string
	}{
		{got[0].ID, false, []string{"calendar.changed", "account.changed"}},
		{cal[0].ID, false, []string{"people.changed", "account.changed"}},
		{cal[0].ID, false, []string{"account.changed"}},
	} {
		*events = nil
		if err := d.Tx(ctx, func(tx *Tx) error {
			_, err := tx.UpdateCollection(ctx, c.id, &c.enabled, false)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range *events {
			names = append(names, e.Event)
		}
		if !slices.Equal(names, c.want) {
			t.Errorf("events after enabled=%v on %d = %q, want %q", c.enabled, c.id, names, c.want)
		}
	}
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.UpdateCollection(ctx, 999, nil, true)
		return err
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown collection: %v", err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.SetCollectionSync(ctx, 999, "t", "c") }); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetCollectionSync of an unknown collection: %v", err)
	}
}

func TestObjects(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	a := insertAccount(t, d, sampleAccount("one@mailtest.test"))
	books := replaceCols(t, d, a.ID, api.CollectionKindAddressbook, RemoteCollection{Href: "/ab/", Name: "Book"})
	col := books[0].ID

	id := putObject(t, d, Object{CollectionID: col, Href: "/ab/a.vcf", ETag: `"a1"`, Kind: ObjectVCard, UID: "a", Raw: []byte("A1")})
	putObject(t, d, Object{CollectionID: col, Href: "/ab/b.vcf", ETag: `"b1"`, Kind: ObjectVCard, Raw: []byte("B1"), ParseError: "bad"})
	again := putObject(t, d, Object{CollectionID: col, Href: "/ab/a.vcf", ETag: `"a2"`, Kind: ObjectVCard, UID: "a", Raw: []byte("A2")})
	if again != id {
		t.Errorf("replacing an object changed its ID: %d → %d", id, again)
	}
	o, err := d.GetObject(ctx, id)
	if err != nil || string(o.Raw) != "A2" || o.ETag != `"a2"` || o.UID != "a" || o.Kind != ObjectVCard ||
		o.CollectionID != col || !o.UpdatedAt.Equal(testNow) {
		t.Errorf("GetObject = %+v, %v", o, err)
	}
	b, err := d.ObjectByHref(ctx, col, "/ab/b.vcf")
	if err != nil || b.ParseError != "bad" {
		t.Errorf("ObjectByHref = %+v, %v", b, err)
	}
	if _, err := d.ObjectByHref(ctx, col, "/ab/none.vcf"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing href: %v", err)
	}
	etags, err := d.ObjectETags(ctx, col)
	if err != nil || !maps.Equal(etags, map[string]string{"/ab/a.vcf": `"a2"`, "/ab/b.vcf": `"b1"`}) {
		t.Errorf("ObjectETags = %v, %v", etags, err)
	}

	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.QueuePIMOp(ctx, PIMOp{AccountID: a.ID, CollectionID: col, ObjectID: &id, Kind: "put", Href: "/ab/a.vcf", IfMatch: `"a2"`})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := d.PendingHrefs(ctx, col)
	if err != nil || !maps.Equal(pending, map[string]bool{"/ab/a.vcf": true}) {
		t.Errorf("PendingHrefs = %v, %v", pending, err)
	}

	var n int
	if err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		n, err = tx.DeleteObjects(ctx, col, []string{"/ab/b.vcf", "/ab/none.vcf"})
		return err
	}); err != nil || n != 1 {
		t.Errorf("DeleteObjects = %d, %v", n, err)
	}
	if _, err := d.ObjectByHref(ctx, col, "/ab/b.vcf"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted object: %v", err)
	}
}

func TestContacts(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	a := insertAccount(t, d, sampleAccount("one@mailtest.test"))
	col := replaceCols(t, d, a.ID, api.CollectionKindAddressbook, RemoteCollection{Href: "/ab/", Name: "Book"})[0].ID
	id := putObject(t, d, Object{CollectionID: col, Href: "/ab/a.vcf", Kind: ObjectVCard, Raw: []byte("A")})
	index := func(c ContactIndex) {
		t.Helper()
		if err := d.Tx(ctx, func(tx *Tx) error { return tx.IndexContact(ctx, id, c) }); err != nil {
			t.Fatal(err)
		}
	}
	index(ContactIndex{
		DisplayName: "Ada Lovelace", SortKey: "lovelace ada", GivenName: "Ada", FamilyName: "Lovelace",
		Organization: "Engines", Photo: []byte{1, 2}, PhotoType: "image/png",
		// Google's vCards may give an address twice (preferred, then plain):
		// the first is kept.
		Emails: []ContactEmail{{Email: " Ada@Example.COM ", Label: "home"}, {Email: ""}, {Email: "ada@work.example", Label: "work"},
			{Email: "ada@example.com"}},
	})
	got, err := d.Contact(ctx, id)
	want := ContactIndex{
		DisplayName: "Ada Lovelace", SortKey: "lovelace ada", GivenName: "Ada", FamilyName: "Lovelace",
		Organization: "Engines", Photo: []byte{1, 2}, PhotoType: "image/png",
		Emails: []ContactEmail{{Email: "ada@example.com", Label: "home"}, {Email: "ada@work.example", Label: "work"}},
	}
	if err != nil || !contactEqual(got, want) {
		t.Errorf("Contact = %+v, %v\nwant %+v", got, err, want)
	}
	index(ContactIndex{DisplayName: "Ada", Emails: []ContactEmail{{Email: "ada@new.example"}}})
	got, _ = d.Contact(ctx, id)
	if got.DisplayName != "Ada" || len(got.Emails) != 1 || got.Emails[0].Email != "ada@new.example" || got.Photo != nil {
		t.Errorf("reindexed = %+v", got)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.RemoveContact(ctx, id) }); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Contact(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed contact: %v", err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.RemoveContact(ctx, id) }); err != nil {
		t.Errorf("removing again: %v", err)
	}
	index(ContactIndex{DisplayName: "Back"})
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.DeleteObjects(ctx, col, []string{"/ab/a.vcf"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Contact(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("contact of a deleted object: %v", err)
	}
}

func contactEqual(a, b ContactIndex) bool {
	return a.DisplayName == b.DisplayName && a.SortKey == b.SortKey && a.GivenName == b.GivenName &&
		a.FamilyName == b.FamilyName && a.Organization == b.Organization && slices.Equal(a.Emails, b.Emails) &&
		slices.Equal(a.Photo, b.Photo) && a.PhotoType == b.PhotoType
}

func TestUseServerDefault(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	a := insertAccount(t, d, sampleAccount("one@mailtest.test"))
	replaceCols(t, d, a.ID, api.CollectionKindCalendar,
		RemoteCollection{Href: "/maint/", Name: "Maintenance"}, RemoteCollection{Href: "/cal/", Name: "Calendar"},
		RemoteCollection{Href: "/ro/", Name: "Holidays", ReadOnly: true})
	defaultHref := func() string {
		t.Helper()
		cols, err := d.Collections(ctx, CollectionFilter{AccountID: a.ID, Kind: api.CollectionKindCalendar})
		if err != nil {
			t.Fatal(err)
		}
		href := ""
		for _, c := range cols {
			if c.IsDefault {
				if href != "" {
					t.Fatalf("two defaults: %+v", cols)
				}
				href = c.Href
			}
		}
		return href
	}
	use := func(href string) bool {
		t.Helper()
		var changed bool
		if err := d.Tx(ctx, func(tx *Tx) error {
			var err error
			changed, err = tx.UseServerDefault(ctx, a.ID, api.CollectionKindCalendar, href)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return changed
	}
	if got := defaultHref(); got != "/maint/" {
		t.Fatalf("the first listed is the default: %q", got)
	}
	if !use("/cal/") || defaultHref() != "/cal/" {
		t.Errorf("the server's default = %q", defaultHref())
	}
	for _, href := range []string{"/cal/", "/ro/", "/missing/"} {
		if use(href) || defaultHref() != "/cal/" {
			t.Errorf("use %s: default %q", href, defaultHref())
		}
	}
	cols, _ := d.Collections(ctx, CollectionFilter{AccountID: a.ID, Kind: api.CollectionKindCalendar})
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.UpdateCollection(ctx, cols[0].ID, nil, true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if use("/cal/") || defaultHref() != cols[0].Href {
		t.Errorf("the user's choice gave way: %q", defaultHref())
	}
}
