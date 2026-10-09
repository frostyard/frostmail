package pimsync_test

import (
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
)

// TestSyncWithoutAnInitialToken: Google's CardDAV lists sync-collection
// among its reports but answers one without a token with 400 (recorded in
// Phase 5). pimsync lists the collection instead, every pass.
func TestSyncWithoutAnInitialToken(t *testing.T) {
	e := newEnv(t, davtest.Options{NoInitialSync: true}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "My Contacts")
	e.dav.Put(book+"ada.vcf", vcard("ada", "Ada Lovelace", "ada@example.com"))
	e.pass()
	if got := names(t, e); !slices.Equal(got, []string{"Ada Lovelace"}) {
		t.Fatalf("people = %q", got)
	}
	e.dav.Put(book+"alan.vcf", vcard("alan", "Alan Turing", "alan@example.com"))
	e.dav.Delete(book + "ada.vcf")
	e.pass()
	if got := names(t, e); !slices.Equal(got, []string{"Alan Turing"}) {
		t.Errorf("people after changes = %q", got)
	}
	var reports, lists int
	for _, r := range e.dav.Requests() {
		switch {
		case r.Method == "REPORT" && r.Report == "sync-collection":
			reports++
		case r.Method == "PROPFIND" && r.Depth == "1" && r.Path == book:
			lists++
		}
	}
	if reports != 1 || lists != 1 {
		t.Errorf("the last pass sent %d sync-collections and %d listings of the book", reports, lists)
	}
}

// names lists the people's display names in their order.
func names(t *testing.T, e *env) []string {
	t.Helper()
	people, err := e.db.People(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range people {
		out = append(out, p.DisplayName)
	}
	return out
}
