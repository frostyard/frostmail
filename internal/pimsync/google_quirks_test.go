package pimsync_test

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/store"
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

// review is one occurrence of a series the calendar does not hold: an
// override without its master, starting at start (UTC) for an hour.
func review(start string) []byte {
	at, err := time.Parse("20060102T150405Z", start)
	if err != nil {
		panic(err)
	}
	return ics("BEGIN:VEVENT", "UID:review", "RECURRENCE-ID:20261010T140000Z", "DTSTART:"+start,
		"DTEND:"+at.Add(time.Hour).Format("20060102T150405Z"), "SUMMARY:Review", "END:VEVENT")
}

// TestServedObjectsCalledDeleted: Google's sync-collection calls some
// objects deleted, from no token as well, that it still serves: single
// occurrences whose series is gone (found on the user's account,
// 2026-10-14). A Gmail account's pass asks for every object a sync calls
// deleted, keeps what the server sends and removes the rest, so verify
// finds nothing missing. Other accounts follow their sync.
func TestServedObjectsCalledDeleted(t *testing.T) {
	ctx := t.Context()
	now := utcAt("2026-10-08T12:00Z")
	setUp := func(kind api.AccountKind) (*env, string) {
		e := newCalendarEnv(t, &now)
		if err := e.db.Tx(ctx, func(tx *store.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE accounts SET kind = ? WHERE id = ?`, kind, e.acct.ID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		cal := e.dav.Calendar("events", "Events", "")
		e.dav.Put(cal+"lunch.ics", ics("BEGIN:VEVENT", "UID:lunch", "DTSTART:20261009T110000Z", "DTEND:20261009T120000Z",
			"SUMMARY:Lunch", "END:VEVENT"))
		e.dav.Put(cal+"review.ics", review("20261010T150000Z"))
		e.dav.HideFromSync(cal + "review.ics")
		e.pass()
		return e, cal
	}
	verify := func(e *env) api.CollectionCheck {
		t.Helper()
		checks, err := e.m.Verify(ctx, e.acct.ID)
		if err != nil || len(checks) != 1 {
			t.Fatalf("checks = %+v, %v", checks, err)
		}
		return checks[0]
	}
	occurrences := func(e *env, want ...string) {
		t.Helper()
		if got := e.occurrences("2026-10-09", "2026-10-12"); !slices.Equal(got, want) {
			t.Errorf("occurrences = %q, want %q", got, want)
		}
	}

	e, cal := setUp(api.AccountKindGmail)
	occurrences(e, "10-09T11:00 Lunch", "10-10T15:00 Review")
	if c := verify(e); !pimsync.Clean(c) || c.Server != 2 || c.Local != 2 {
		t.Errorf("check = %+v", c)
	}
	// The occurrence moves, and the sync calls it deleted again.
	e.dav.Put(cal+"review.ics", review("20261010T160000Z"))
	e.pass()
	occurrences(e, "10-09T11:00 Lunch", "10-10T16:00 Review")
	// Both go: the server sends neither.
	e.dav.Delete(cal + "review.ics")
	e.dav.Delete(cal + "lunch.ics")
	e.pass()
	occurrences(e)
	if c := verify(e); !pimsync.Clean(c) || c.Server != 0 || c.Local != 0 {
		t.Errorf("check after the deletes = %+v", c)
	}

	e, cal = setUp(api.AccountKindIMAP)
	occurrences(e, "10-09T11:00 Lunch")
	if c := verify(e); !slices.Equal(c.MissingLocally, []string{cal + "review.ics"}) {
		t.Errorf("an IMAP account's check = %+v", c)
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
