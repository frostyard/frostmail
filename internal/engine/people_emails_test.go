package engine_test

import (
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/vcardx"
)

// annCard gives each address more than once, as Google's vCards do
// (item1.EMAIL;TYPE=PREF then item2.EMAIL), in other cases and with labels
// on later lines.
const annCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:ann\r\nFN:Ann Example\r\n" +
	"item1.EMAIL;TYPE=PREF:ann@x.test\r\nitem2.EMAIL:ANN@X.test\r\n" +
	"item3.EMAIL;TYPE=INTERNET:ann@work.test\r\nEMAIL;TYPE=WORK:Ann@Work.test\r\nEMAIL;TYPE=OTHER:ann@x.test\r\n" +
	"item1.X-ABLabel:\r\nitem2.X-ABLabel:_$!<Home>!$_\r\nitem3.X-ABLabel:\r\nEND:VCARD\r\n"

// TestPeopleGetEmailsOnce: a contact shows each address once, the first
// line's, labeled by a later line only when the first has no label, while
// the stored vCard keeps every line.
func TestPeopleGetEmailsOnce(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	a, err := c.Account().Create(ctx, createParams("me@mailtest.test"))
	if err != nil {
		t.Fatal(err)
	}
	var ann int64
	err = srv.DB.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetService(ctx, a.ID, api.ServiceKindContacts, true, "https://dav.test/"); err != nil {
			return err
		}
		cols, err := tx.ReplaceCollections(ctx, a.ID, api.CollectionKindAddressbook,
			[]store.RemoteCollection{{Href: "/ab/", Name: "Contacts"}})
		if err != nil {
			return err
		}
		if ann, err = tx.PutObject(ctx, store.Object{CollectionID: cols[0].ID, Href: "/ab/ann.vcf", ETag: `"1"`,
			Kind: store.ObjectVCard, Raw: []byte(annCard)}); err != nil {
			return err
		}
		if err := tx.IndexContact(ctx, ann, store.ContactIndex{DisplayName: "Ann Example", SortKey: "example ann",
			Emails: []store.ContactEmail{{Email: "ann@x.test"}, {Email: "ann@work.test"}}}); err != nil {
			return err
		}
		return tx.RelinkPeople(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := c.People().List(ctx, &api.PeopleListParams{})
	if err != nil || len(list) != 1 {
		t.Fatalf("people.list = %+v, %v", list, err)
	}
	p, err := c.People().Get(ctx, &api.PeopleGetParams{ID: list[0].ID})
	if err != nil || len(p.Contacts) != 1 {
		t.Fatalf("people.get = %+v, %v", p, err)
	}
	want := []api.LabeledValue{{Label: "home", Value: "ann@x.test"}, {Label: "work", Value: "ann@work.test"}}
	if got := p.Contacts[0].Emails; !slices.Equal(got, want) {
		t.Errorf("emails = %+v, want %+v", got, want)
	}

	object, err := srv.DB.GetObject(ctx, ann)
	if err != nil {
		t.Fatal(err)
	}
	card, err := vcardx.Parse(object.Raw)
	if err != nil || len(card.Emails) != 5 {
		t.Errorf("stored vCard's emails = %+v, %v; want all 5 lines", card, err)
	}
}
