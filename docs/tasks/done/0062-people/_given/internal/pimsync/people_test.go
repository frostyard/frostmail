package pimsync_test

// CONTRACT TEST for task card T-0062 (docs/tasks). Do not edit.

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
)

// TestPassLinksPeople: every pass that changes contacts leaves people in
// step: new and changed contacts, deleted ones, and removed address books.
func TestPassLinksPeople(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	other := e.dav.AddressBook("other", "Other")
	e.dav.Put(book+"a.vcf", vcard("a", "Ada", "ada@example.com"))
	e.dav.Put(other+"b.vcf", vcard("b", "Ada L", "ADA@example.com", "ada@work.example"))
	e.dav.Put(book+"g.vcf", vcard("g", "Grace", "grace@navy.example"))
	e.pass()
	ctx := t.Context()
	people, err := e.db.People(ctx, "")
	if err != nil || len(people) != 2 || len(people[0].ContactIDs) != 2 || people[0].DisplayName != "Ada" {
		t.Fatalf("people after the first pass = %+v, %v", people, err)
	}

	e.dav.Put(book+"a.vcf", vcard("a", "Ada", "ada@home.example"))
	e.pass()
	if people, _ = e.db.People(ctx, ""); len(people) != 3 {
		t.Errorf("people after a change = %+v; want Ada and Ada L apart", people)
	}

	e.dav.Delete(book + "g.vcf")
	e.pass()
	if people, _ = e.db.People(ctx, ""); len(people) != 2 {
		t.Errorf("people after a delete = %+v", people)
	}

	e.dav.RemoveCollection(other)
	e.pass()
	if people, _ = e.db.People(ctx, ""); len(people) != 1 || people[0].DisplayName != "Ada" {
		t.Errorf("people after the book was removed = %+v", people)
	}
}
