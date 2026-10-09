// CONTRACT TEST for task card T-0094 (docs/tasks). Do not edit.
package pimsync_test

import (
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/pimsync"
)

func TestVerify(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	e.dav.Put(book+"a.vcf", vcard("a", "A"))
	e.dav.Put(book+"b.vcf", vcard("b", "B"))
	ctx := t.Context()
	if checks, err := e.m.Verify(ctx, e.acct.ID); err != nil || len(checks) != 0 {
		t.Fatalf("before the first pass = %+v, %v; the home set is not known yet", checks, err)
	}
	e.pass()
	checks, err := e.m.Verify(ctx, e.acct.ID)
	if err != nil || len(checks) != 1 || !pimsync.Clean(checks[0]) || checks[0].Server != 2 || checks[0].Local != 2 ||
		checks[0].Name != "Contacts" {
		t.Fatalf("after a pass = %+v, %v", checks, err)
	}
	// Changes on the server that no pass has seen show, and nothing changes.
	e.dav.Put(book+"a.vcf", vcard("a", "A2"))
	e.dav.Delete(book + "b.vcf")
	e.dav.Put(book+"c.vcf", vcard("c", "C"))
	e.dav.ResetRequests()
	checks, err = e.m.Verify(ctx, e.acct.ID)
	if err != nil || len(checks) != 1 {
		t.Fatal(checks, err)
	}
	c := checks[0]
	if pimsync.Clean(c) || c.EtagDiffs != 1 || !slices.Equal(c.MissingLocally, []string{book + "c.vcf"}) ||
		!slices.Equal(c.MissingOnServer, []string{book + "b.vcf"}) {
		t.Errorf("check = %+v", c)
	}
	// It lists, and asks for what is missing locally; it writes nothing.
	for _, r := range e.dav.Requests() {
		if r.Method != "PROPFIND" && r.Report != "addressbook-multiget" {
			t.Errorf("verify sent %s %s %s", r.Method, r.Path, r.Report)
		}
	}
}
