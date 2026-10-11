package engine_test

// CONTRACT TEST for task card T-0062 (docs/tasks): people.list, get, card
// and photo, and contacts first in address.suggest, over the socket. Do
// not edit.

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

const adaCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:ada\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\n" +
	"ORG:Engines;Analysis\r\nTITLE:Countess\r\nEMAIL;TYPE=INTERNET;TYPE=HOME:ada@example.com\r\n" +
	"TEL;TYPE=CELL:+44 20 7946 0000\r\nADR;TYPE=HOME:;;12 Square;London;;SW1;UK\r\nURL:https://ada.example/\r\n" +
	"BDAY:1815-12-10\r\nNOTE:Met at the\\nExhibition\r\nPHOTO;ENCODING=b;TYPE=PNG:iVBORw0KGgo=\r\nEND:VCARD\r\n"

const sharedCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:ada-shared\r\nFN:Ada\r\nNICKNAME:Countess of Lovelace\r\n" +
	"EMAIL:ada@example.com\r\nEND:VCARD\r\n"

// peopleServer is maild with one account: an address book with Ada, a
// read-only shared book with Ada again, and mail from Ada and a stranger.
type peopleServer struct {
	c                 *api.Client
	acct              int64
	ada, shared       int64 // object IDs
	book, sharedBook  int64 // collection IDs
	fromAda, stranger int64 // message IDs
}

func newPeopleServer(t *testing.T) *peopleServer {
	t.Helper()
	srv := rpctest.Start(t)
	ps := &peopleServer{c: srv.Dial(t)}
	ctx := t.Context()
	a, err := ps.c.Account().Create(ctx, createParams("me@mailtest.test"))
	if err != nil {
		t.Fatal(err)
	}
	ps.acct = a.ID
	err = srv.DB.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetService(ctx, a.ID, api.ServiceKindContacts, true, "https://dav.test/"); err != nil {
			return err
		}
		cols, err := tx.ReplaceCollections(ctx, a.ID, api.CollectionKindAddressbook, []store.RemoteCollection{
			{Href: "/ab/", Name: "Contacts"}, {Href: "/shared/", Name: "Shared", ReadOnly: true},
		})
		if err != nil {
			return err
		}
		ps.book, ps.sharedBook = cols[0].ID, cols[1].ID
		for _, c := range []struct {
			col  int64
			href string
			raw  string
			idx  store.ContactIndex
			id   *int64
		}{
			{ps.book, "/ab/ada.vcf", adaCard, store.ContactIndex{DisplayName: "Ada Lovelace", SortKey: "lovelace ada",
				GivenName: "Ada", FamilyName: "Lovelace", Organization: "Engines",
				Emails: []store.ContactEmail{{Email: "ada@example.com", Label: "home"}},
				Photo:  []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, PhotoType: "image/png"}, &ps.ada},
			{ps.sharedBook, "/shared/ada.vcf", sharedCard, store.ContactIndex{DisplayName: "Ada", SortKey: "ada",
				Emails: []store.ContactEmail{{Email: "ada@example.com"}}}, &ps.shared},
		} {
			id, err := tx.PutObject(ctx, store.Object{CollectionID: c.col, Href: c.href, ETag: `"1"`, Kind: store.ObjectVCard, Raw: []byte(c.raw)})
			if err != nil {
				return err
			}
			*c.id = id
			if err := tx.IndexContact(ctx, id, c.idx); err != nil {
				return err
			}
		}
		if err := tx.RelinkPeople(ctx); err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		day := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
		hs := []store.MessageHeader{
			{UID: 1, InternalDate: day(1), Date: day(1), Subject: "Engines", MessageID: "1@x",
				From: store.Address{Name: "Ada L.", Addr: "ada@example.com"}, To: []store.Address{{Addr: "me@mailtest.test"}}},
			{UID: 2, InternalDate: day(2), Date: day(2), Subject: "Hello", MessageID: "2@x",
				From: store.Address{Name: "Stranger Danger", Addr: "stranger@example.com"}, To: []store.Address{{Addr: "me@mailtest.test"}}},
			{UID: 3, InternalDate: day(3), Date: day(3), Subject: "Hi Adam", MessageID: "3@x",
				From: store.Address{Name: "Adam", Addr: "adam@example.com"}, To: []store.Address{{Addr: "me@mailtest.test"}}},
		}
		ids, err := tx.InsertHeaders(ctx, a.ID, mbs[0].ID, hs)
		if err != nil {
			return err
		}
		for i, id := range ids {
			if err := tx.IndexMessage(ctx, id, store.SearchDocFor(hs[i])); err != nil {
				return err
			}
		}
		ps.fromAda, ps.stranger = ids[0], ids[1]
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ps
}

func TestPeopleListAndGet(t *testing.T) {
	ps := newPeopleServer(t)
	ctx := t.Context()
	list, err := ps.c.People().List(ctx, &api.PeopleListParams{})
	if err != nil {
		t.Fatal(err)
	}
	want := api.PersonSummary{ID: ps.ada, DisplayName: "Ada Lovelace", Organization: "Engines", Email: "ada@example.com",
		HasPhoto: true, Index: "L"}
	if len(list) != 1 || list[0] != want {
		t.Fatalf("people.list = %+v, want [%+v]", list, want)
	}
	if none, err := ps.c.People().List(ctx, &api.PeopleListParams{Query: ptr("zzz")}); err != nil || len(none) != 0 {
		t.Errorf("people.list zzz = %+v, %v", none, err)
	}
	if hit, _ := ps.c.People().List(ctx, &api.PeopleListParams{Query: ptr("love")}); len(hit) != 1 {
		t.Errorf("people.list love = %+v", hit)
	}
	if in, _ := ps.c.People().List(ctx, &api.PeopleListParams{CollectionID: ptr(ps.sharedBook)}); len(in) != 1 {
		t.Errorf("people.list in the shared book = %+v", in)
	}
	if in, _ := ps.c.People().List(ctx, &api.PeopleListParams{CollectionID: ptr(int64(9999))}); len(in) != 0 {
		t.Errorf("people.list in an unknown book = %+v", in)
	}

	p, err := ps.c.People().Get(ctx, &api.PeopleGetParams{ID: ps.ada})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != ps.ada || p.DisplayName != "Ada Lovelace" || p.Organization != "Engines" || !p.HasPhoto || len(p.Contacts) != 2 {
		t.Fatalf("people.get = %+v", p)
	}
	mine, shared := p.Contacts[0], p.Contacts[1]
	wantMine := api.Contact{
		ID: ps.ada, CollectionID: ps.book, AccountID: ps.acct, DisplayName: "Ada Lovelace", GivenName: "Ada",
		FamilyName: "Lovelace", Organization: "Engines", Title: "Countess",
		Emails:    []api.LabeledValue{{Label: "home", Value: "ada@example.com"}},
		Phones:    []api.LabeledValue{{Label: "mobile", Value: "+44 20 7946 0000"}},
		Addresses: []api.PostalAddress{{Label: "home", Street: "12 Square", Locality: "London", Postcode: "SW1", Country: "UK"}},
		URLs:      []api.LabeledValue{{Value: "https://ada.example/"}},
		Birthday:  "1815-12-10", Note: "Met at the\nExhibition",
	}
	if !contactEqual(mine, wantMine) {
		t.Errorf("first contact = %+v\nwant %+v", mine, wantMine)
	}
	if shared.ID != ps.shared || shared.CollectionID != ps.sharedBook || !shared.ReadOnly || mine.ReadOnly ||
		shared.Nickname != "Countess of Lovelace" || shared.DisplayName != "Ada" {
		t.Errorf("shared contact = %+v", shared)
	}
	if _, err := ps.c.People().Get(ctx, &api.PeopleGetParams{ID: 9999}); code(err) != api.CodeNotFound {
		t.Errorf("people.get 9999 = %v", err)
	}
}

func contactEqual(a, b api.Contact) bool {
	return a.ID == b.ID && a.CollectionID == b.CollectionID && a.AccountID == b.AccountID &&
		a.DisplayName == b.DisplayName && a.GivenName == b.GivenName && a.FamilyName == b.FamilyName &&
		a.Nickname == b.Nickname && a.Organization == b.Organization && a.Title == b.Title &&
		slices.Equal(a.Emails, b.Emails) && slices.Equal(a.Phones, b.Phones) && slices.Equal(a.Addresses, b.Addresses) &&
		slices.Equal(a.URLs, b.URLs) && a.Birthday == b.Birthday && a.Note == b.Note && a.ReadOnly == b.ReadOnly
}

func TestPeoplePhoto(t *testing.T) {
	ps := newPeopleServer(t)
	ctx := t.Context()
	ph, err := ps.c.People().Photo(ctx, &api.PeoplePhotoParams{ID: ps.ada})
	want := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	if err != nil || ph.ContentType != "image/png" || ph.Data != want {
		t.Errorf("people.photo = %+v, %v", ph, err)
	}
	if _, err := ps.c.People().Photo(ctx, &api.PeoplePhotoParams{ID: 9999}); code(err) != api.CodeNotFound {
		t.Errorf("people.photo 9999 = %v", err)
	}
}

func TestContactCard(t *testing.T) {
	ps := newPeopleServer(t)
	ctx := t.Context()
	card, err := ps.c.People().Card(ctx, &api.PeopleCardParams{Email: " ADA@Example.com "})
	if err != nil {
		t.Fatal(err)
	}
	if card.Email != "ada@example.com" || card.Name != "Ada Lovelace" || card.Person == nil || card.Person.ID != ps.ada ||
		len(card.Person.Contacts) != 2 || card.CanAdd || card.Upcoming == nil || len(card.Upcoming) != 0 {
		t.Errorf("ada's card = %+v", card)
	}
	if !slices.Equal(summaryIDs(card.Recent), []int64{ps.fromAda}) {
		t.Errorf("ada's recent = %v, want %v", summaryIDs(card.Recent), []int64{ps.fromAda})
	}

	card, err = ps.c.People().Card(ctx, &api.PeopleCardParams{Email: "stranger@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if card.Name != "Stranger Danger" || card.Person != nil || !card.CanAdd || !slices.Equal(summaryIDs(card.Recent), []int64{ps.stranger}) {
		t.Errorf("stranger's card = %+v", card)
	}
	card, err = ps.c.People().Card(ctx, &api.PeopleCardParams{Email: "nobody@example.com"})
	if err != nil || card.Name != "" || card.Person != nil || len(card.Recent) != 0 || card.Recent == nil {
		t.Errorf("nobody's card = %+v, %v", card, err)
	}
	if _, err := ps.c.People().Card(ctx, &api.PeopleCardParams{Email: "not an address"}); code(err) != api.CodeInvalidParams {
		t.Errorf("people.card of a non-address = %v", err)
	}
}

// TestSuggestContactsFirst: address.suggest lists contacts before the
// addresses seen only in mail, each address once.
func TestSuggestContactsFirst(t *testing.T) {
	ps := newPeopleServer(t)
	got, err := ps.c.Address().Suggest(t.Context(), &api.AddressSuggestParams{Prefix: "ada"})
	if err != nil {
		t.Fatal(err)
	}
	var addrs []string
	for _, a := range got {
		addrs = append(addrs, a.Name+" <"+a.Address+">")
	}
	if strings.Join(addrs, ", ") != "Ada Lovelace <ada@example.com>, Adam <adam@example.com>" {
		t.Errorf("address.suggest ada = %q", addrs)
	}
	one, _ := ps.c.Address().Suggest(t.Context(), &api.AddressSuggestParams{Prefix: "ada", Limit: ptr(int64(1))})
	if len(one) != 1 || one[0].Address != "ada@example.com" {
		t.Errorf("limit 1 = %+v", one)
	}
}

// TestPeopleSenders: people.senders names the addresses whose person has a
// photo, in any case, and only those.
func TestPeopleSenders(t *testing.T) {
	ps := newPeopleServer(t)
	ctx := t.Context()
	got, err := ps.c.People().Senders(ctx, &api.PeopleSendersParams{Addresses: []string{" ADA@example.com", "stranger@example.com", "alan@example.com"}})
	if err != nil || len(got) != 1 || got[0].Address != "ada@example.com" || got[0].PersonID != ps.ada {
		t.Errorf("people.senders = %+v, %v", got, err)
	}
	if got, err := ps.c.People().Senders(ctx, &api.PeopleSendersParams{Addresses: []string{}}); err != nil || len(got) != 0 {
		t.Errorf("no addresses = %+v, %v", got, err)
	}
	many := make([]string, 501)
	for i := range many {
		many[i] = "x@example.com"
	}
	if _, err := ps.c.People().Senders(ctx, &api.PeopleSendersParams{Addresses: many}); code(err) != api.CodeInvalidParams {
		t.Errorf("501 addresses: %v", err)
	}
}
