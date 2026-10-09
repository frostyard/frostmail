package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/vcardx"
)

// showcaseContact is a made-up person for the showcase's address books.
type showcaseContact struct {
	given, family, org, title, email, phone, city string
}

// showcaseBooks are the address books of the showcase accounts, by account
// email: the people the showcase mail is from, all invented.
func showcaseBooks() map[string][]showcaseContact {
	return map[string][]showcaseContact{
		"ann@northwind.example": {
			{"Maria", "Lopez", "Northwind", "Engineering Manager", "maria.lopez@northwind.example", "+1 555 0101", "Portland"},
			{"Lena", "Price", "Northwind", "Product Designer", "lena.price@northwind.example", "+1 555 0102", "Portland"},
			{"Victor", "Walker", "Northwind", "Site Reliability", "victor.walker@northwind.example", "+1 555 0103", "Seattle"},
			{"Peter", "Brown", "Northwind", "Finance", "peter.brown@northwind.example", "+1 555 0104", "Portland"},
			{"Omar", "Ward", "Northwind", "QA Lead", "omar.ward@northwind.example", "+1 555 0105", "Denver"},
			{"David", "King", "Northwind", "Release Engineer", "david.king@northwind.example", "+1 555 0106", "Portland"},
			{"Grace", "Chen", "Northwind", "Head of Product", "grace.chen@northwind.example", "+1 555 0107", "San Francisco"},
			{"Hugo", "Bauer", "Northwind", "Support", "hugo.bauer@northwind.example", "+1 555 0108", "Berlin"},
		},
		"ann.lee@example.com": {
			{"Sam", "Ortega", "", "", "sam.ortega@example.net", "+1 555 0111", "Lisbon"},
			{"Nina", "Davis", "Riverside Climbing", "", "nina.davis@example.org", "+1 555 0112", "Portland"},
			{"Iris", "Young", "", "", "iris.young@example.org", "+1 555 0113", "Portland"},
		},
	}
}

// addShowcaseContacts turns contacts on for an account and stores its
// address book, as a pass of pimsync would have.
func addShowcaseContacts(ctx context.Context, db *store.DB, accountID int64, email string, now time.Time) error {
	people := showcaseBooks()[email]
	if len(people) == 0 {
		return nil
	}
	domain := email[strings.LastIndexByte(email, '@')+1:]
	return db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetService(ctx, accountID, api.ServiceKindContacts, true, "https://dav."+domain+"/"); err != nil {
			return err
		}
		if err := tx.ServiceSynced(ctx, accountID, api.ServiceKindContacts, now.Add(-4*time.Minute), ""); err != nil {
			return err
		}
		books, err := tx.ReplaceCollections(ctx, accountID, api.CollectionKindAddressbook,
			[]store.RemoteCollection{{Href: "/contacts/", Name: "Contacts"}})
		if err != nil {
			return err
		}
		for i, p := range people {
			if err := putShowcaseContact(ctx, tx, books[0].ID, fmt.Sprintf("%s-%d", domain, i), p); err != nil {
				return err
			}
		}
		return tx.RelinkPeople(ctx)
	})
}

func putShowcaseContact(ctx context.Context, tx *store.Tx, book int64, uid string, p showcaseContact) error {
	var b strings.Builder
	for _, l := range []string{
		"BEGIN:VCARD", "VERSION:3.0", "UID:" + uid,
		"FN:" + contentline.EscapeText(p.given+" "+p.family),
		"N:" + contentline.JoinFields(p.family, p.given, "", "", ""),
		"ORG:" + contentline.EscapeText(p.org), "TITLE:" + contentline.EscapeText(p.title),
		"EMAIL;TYPE=INTERNET;TYPE=WORK:" + p.email, "TEL;TYPE=CELL:" + p.phone,
		"ADR;TYPE=WORK:" + contentline.JoinFields("", "", "", p.city, "", "", ""), "END:VCARD",
	} {
		b.WriteString(contentline.Fold(l, "\r\n"))
	}
	raw := []byte(b.String())
	card, err := vcardx.Parse(raw)
	if err != nil {
		return err
	}
	id, err := tx.PutObject(ctx, store.Object{CollectionID: book, Href: "/contacts/" + uid + ".vcf", ETag: `"1"`,
		Kind: store.ObjectVCard, UID: uid, Raw: raw})
	if err != nil {
		return err
	}
	return tx.IndexContact(ctx, id, store.ContactIndex{
		DisplayName: card.DisplayName(), SortKey: card.SortKey(), GivenName: card.GivenName,
		FamilyName: card.FamilyName, Organization: card.Organization,
		Emails: []store.ContactEmail{{Email: p.email, Label: "work"}},
	})
}
