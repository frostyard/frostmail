package engine

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/vcardx"
)

// People implements the people domain.
func (e *Engine) People() api.PeopleService { return people{e.d} }

type people struct{ Deps }

// List returns matching people in store sort order, only those with a
// contact in the address book when one is named.
func (p people) List(ctx context.Context, params *api.PeopleListParams) ([]api.PersonSummary, error) {
	query := ""
	if params.Query != nil {
		query = *params.Query
	}
	rows, err := p.DB.People(ctx, query)
	if err != nil {
		return nil, err
	}
	var in map[int64]bool
	if params.CollectionID != nil {
		if in, err = p.DB.ContactsIn(ctx, *params.CollectionID); err != nil {
			return nil, err
		}
	}
	out := make([]api.PersonSummary, 0, len(rows))
	for _, r := range rows {
		if in != nil && !slices.ContainsFunc(r.ContactIDs, func(id int64) bool { return in[id] }) {
			continue
		}
		out = append(out, api.PersonSummary{ID: r.ID, DisplayName: r.DisplayName, Organization: r.Organization,
			Email: r.Email, HasPhoto: r.HasPhoto, Index: indexLetter(r.SortKey)})
	}
	return out, nil
}

// indexLetter is the letter a sort key files under in the People list: its
// first letter uppercased, or "#" when it does not start with a letter.
func indexLetter(sortKey string) string {
	r, _ := utf8.DecodeRuneInString(sortKey)
	if !unicode.IsLetter(r) {
		return "#"
	}
	return string(unicode.ToUpper(r))
}

// Get returns a person and their parseable source contacts in address book order.
func (p people) Get(ctx context.Context, params *api.PeopleGetParams) (*api.Person, error) {
	row, err := p.DB.Person(ctx, params.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("person %d", params.ID))
	}
	return p.fullPerson(ctx, row)
}

type orderedContact struct {
	contact  api.Contact
	position int
}

func (p people) fullPerson(ctx context.Context, row store.PersonRow) (*api.Person, error) {
	contacts := make([]orderedContact, 0, len(row.ContactIDs))
	for _, id := range row.ContactIDs {
		contact, err := p.sourceContact(ctx, id)
		if err != nil {
			return nil, err
		}
		if contact != nil {
			contacts = append(contacts, *contact)
		}
	}
	slices.SortFunc(contacts, func(a, b orderedContact) int {
		if n := cmp.Compare(a.contact.AccountID, b.contact.AccountID); n != 0 {
			return n
		}
		if n := cmp.Compare(a.position, b.position); n != 0 {
			return n
		}
		return cmp.Compare(a.contact.ID, b.contact.ID)
	})
	out := &api.Person{ID: row.ID, DisplayName: row.DisplayName, Organization: row.Organization,
		HasPhoto: row.HasPhoto, Contacts: make([]api.Contact, 0, len(contacts))}
	for _, c := range contacts {
		out.Contacts = append(out.Contacts, c.contact)
	}
	return out, nil
}

func (p people) sourceContact(ctx context.Context, id int64) (*orderedContact, error) {
	object, err := p.DB.GetObject(ctx, id)
	if err != nil {
		return nil, err
	}
	card, err := vcardx.Parse(object.Raw)
	if err != nil {
		return nil, nil
	}
	col, err := p.DB.GetCollection(ctx, object.CollectionID)
	if err != nil {
		return nil, err
	}
	account, err := p.DB.GetAccount(ctx, col.AccountID)
	if err != nil {
		return nil, err
	}
	c := contactFromCard(card)
	c.ID, c.CollectionID, c.AccountID = id, col.ID, col.AccountID
	c.ReadOnly = col.ReadOnly || account.ReadOnly
	return &orderedContact{contact: c, position: col.Position}, nil
}

func contactFromCard(card *vcardx.Card) api.Contact {
	c := api.Contact{
		DisplayName: card.DisplayName(), GivenName: card.GivenName, FamilyName: card.FamilyName,
		Nickname: card.Nickname, Organization: card.Organization, Title: card.Title,
		Emails: labeledValues(card.Emails), Phones: labeledValues(card.Phones), URLs: labeledValues(card.URLs),
		Addresses: make([]api.PostalAddress, 0, len(card.Addresses)), Birthday: card.Birthday, Note: card.Note,
	}
	for _, a := range card.Addresses {
		c.Addresses = append(c.Addresses, api.PostalAddress{Label: a.Label, Street: a.Street,
			Locality: a.Locality, Region: a.Region, Postcode: a.Postcode, Country: a.Country})
	}
	return c
}

func labeledValues(values []vcardx.Labeled) []api.LabeledValue {
	out := make([]api.LabeledValue, 0, len(values))
	for _, v := range values {
		out = append(out, api.LabeledValue{Label: v.Label, Value: v.Value})
	}
	return out
}

// Card returns a normalized address's person, recent mail and ability to add a contact.
func (p people) Card(ctx context.Context, params *api.PeopleCardParams) (*api.ContactCard, error) {
	email := strings.ToLower(strings.TrimSpace(params.Email))
	if !strings.Contains(email, "@") {
		return nil, api.InvalidParams("email must contain @")
	}
	out := &api.ContactCard{Email: email, Recent: []api.MessageSummary{}, Upcoming: []api.Occurrence{}}
	if err := p.cardPerson(ctx, out); err != nil {
		return nil, err
	}
	ids, err := p.DB.MessagesWithAddress(ctx, email, 5)
	if err != nil {
		return nil, err
	}
	rows, err := p.DB.Summaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Recent = append(out.Recent, toAPISummary(r))
	}
	return out, nil
}

func (p people) cardPerson(ctx context.Context, out *api.ContactCard) error {
	row, err := p.DB.PersonByEmail(ctx, out.Email)
	if err == nil {
		out.Name = row.DisplayName
		out.Person, err = p.fullPerson(ctx, row)
		return err
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	out.Name, err = p.DB.SeenName(ctx, out.Email)
	if err != nil {
		return err
	}
	books, err := p.DB.WritableAddressBooks(ctx)
	if err != nil {
		return err
	}
	out.CanAdd = len(books) > 0
	return nil
}

// Photo returns the person's first inline photo encoded as standard base64.
func (p people) Photo(ctx context.Context, params *api.PeoplePhotoParams) (*api.Photo, error) {
	data, typ, err := p.DB.PersonPhoto(ctx, params.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("person photo %d", params.ID))
	}
	return &api.Photo{ContentType: typ, Data: base64.StdEncoding.EncodeToString(data)}, nil
}

// Add reserves contact creation for the planned write path.
func (p people) Add(context.Context, *api.PeopleAddParams) (*api.Person, error) {
	return nil, notBuilt("people.add")
}
