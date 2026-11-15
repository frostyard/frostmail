package pimsync_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/gtasks/gtaskstest"
	"github.com/frostyard/frostmail/internal/store"
)

// opCounts counts the store's costly calls (store.DB.Observe).
type opCounts struct {
	mu sync.Mutex
	n  map[string]int
}

func countOps(db *store.DB) *opCounts {
	o := &opCounts{n: map[string]int{}}
	db.Observe = func(op string) {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.n[op]++
	}
	return o
}

// take returns how many times op ran since the last take, and resets it.
func (o *opCounts) take(op string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := o.n[op]
	o.n[op] = 0
	return n
}

// A pass relinks people once, after all its batches and address books,
// and not at all when no contact changed (frostmail#291).
func TestDAVContactPassRelinksPeopleOnce(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts) // 2 hrefs per multiget
	ops := countOps(e.db)
	book := e.dav.AddressBook("default", "Contacts")
	other := e.dav.AddressBook("other", "Other")
	for i := range 7 {
		e.dav.Put(fmt.Sprintf("%sc%d.vcf", book, i), vcard(fmt.Sprint("c", i), fmt.Sprint("Person ", i), fmt.Sprintf("p%d@example.com", i)))
	}
	e.dav.Put(other+"d.vcf", vcard("d", "Person 0 at work", "P0@example.com"))
	e.pass()
	if n := ops.take(store.OpRelinkPeople); n != 1 {
		t.Errorf("a first pass of 5 batches relinked %d times, want once", n)
	}
	ctx := t.Context()
	if people, err := e.db.People(ctx, ""); err != nil || len(people) != 7 {
		t.Fatalf("people after the first pass = %+v, %v", people, err)
	}

	ops.take(store.OpRelinkPeople)
	e.pass()
	if n := ops.take(store.OpRelinkPeople); n != 0 {
		t.Errorf("a pass with no change relinked %d times", n)
	}

	ops.take(store.OpRelinkPeople)
	e.dav.Put(book+"c1.vcf", vcard("c1", "Person 1", "p0@example.com"))
	e.dav.Delete(book + "c2.vcf")
	e.pass()
	if n := ops.take(store.OpRelinkPeople); n != 1 {
		t.Errorf("a pass with a change and a delete relinked %d times, want once", n)
	}
	if people, _ := e.db.People(ctx, ""); len(people) != 5 {
		t.Errorf("people after the change = %+v; want Person 1 joined to Person 0, and Person 2 gone", people)
	}
}

// A pass that fails part-way still relinks the contacts its committed
// batches stored.
func TestDAVContactPassRelinksAfterAFailure(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	for i := range 5 {
		e.dav.Put(fmt.Sprintf("%sc%d.vcf", book, i), vcard(fmt.Sprint("c", i), fmt.Sprint("Person ", i), fmt.Sprintf("p%d@example.com", i)))
	}
	// The book's REPORTs: the sync, then 3 multigets; the second multiget fails.
	var reports atomic.Int64
	e.dav.FailWith(func(r *http.Request) int {
		if r.Method == "REPORT" && strings.HasSuffix(r.URL.Path, "/default/") && reports.Add(1) == 3 {
			return http.StatusServiceUnavailable
		}
		return 0
	})
	if err := e.m.Pass(t.Context(), e.acct.ID); err == nil {
		t.Fatal("a pass with a failed multiget succeeded")
	}
	people, err := e.db.People(t.Context(), "")
	if err != nil || len(people) != 2 {
		t.Fatalf("people after a pass that stored 1 batch of 2 = %+v, %v", people, err)
	}
	if stale, err := e.db.PeopleStale(t.Context()); err != nil || stale {
		t.Errorf("people stale after the relink = %v, %v", stale, err)
	}
}

// A stale mark that a pass leaves (its relink failed, or it was cut off)
// is relinked by the next pass, even one with no change.
func TestNextPassRelinksAStaleMark(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	ops := countOps(e.db)
	book := e.dav.AddressBook("default", "Contacts")
	e.dav.Put(book+"a.vcf", vcard("a", "Ada", "ada@example.com"))
	e.pass()
	ctx := t.Context()
	if err := e.db.Tx(ctx, func(tx *store.Tx) error { return tx.MarkPeopleStale(ctx) }); err != nil {
		t.Fatal(err)
	}
	ops.take(store.OpRelinkPeople)
	e.pass()
	if n := ops.take(store.OpRelinkPeople); n != 1 {
		t.Errorf("a quiet pass after a stale mark relinked %d times, want once", n)
	}
	if stale, err := e.db.PeopleStale(ctx); err != nil || stale {
		t.Errorf("people stale after the pass = %v, %v", stale, err)
	}
}

func graphPerson(i int) string {
	return fmt.Sprintf(`{"id": "AAMk-c%d", "displayName": "Person %d", "emailAddresses": [{"address": "p%d@example.com"}]}`, i, i, i)
}

// Graph's contact pass relinks people once, after its folder, and not at
// all when nothing changed.
func TestGraphContactPassRelinksPeopleOnce(t *testing.T) {
	e := newGraphEnv(t, allGraphScopes)
	ops := countOps(e.db)
	for i := range 5 {
		e.srv.PutContact("contacts", graphPerson(i), nil)
	}
	e.pass()
	if n := ops.take(store.OpRelinkPeople); n != 1 {
		t.Errorf("a first pass relinked %d times, want once", n)
	}
	if got := e.people(); len(got) != 5 {
		t.Errorf("people = %v", got)
	}
	ops.take(store.OpRelinkPeople)
	e.pass()
	if n := ops.take(store.OpRelinkPeople); n != 0 {
		t.Errorf("a pass with no change relinked %d times", n)
	}
}

// photoRequests counts the photo reads Graph has answered.
func (e *graphEnv) photoRequests() int {
	n := 0
	for k, v := range e.srv.Requests {
		if strings.HasPrefix(k, "GET me/contacts/") && strings.HasSuffix(k, "/photo/$value") {
			n += v
		}
	}
	return n
}

// A full round of unchanged contacts reads no photo; a new or changed
// contact's photo is read, and a contact without one (404) has none.
func TestGraphReadsPhotosOnlyOfNewOrChangedContacts(t *testing.T) {
	e := newGraphEnv(t, allGraphScopes)
	ctx := t.Context()
	jpeg := []byte("\xff\xd8\xff\xe0jpeg")
	for i := range 6 {
		var photo []byte
		if i%2 == 0 {
			photo = jpeg
		}
		e.srv.PutContact("contacts", graphPerson(i), photo)
	}
	e.pass()
	if n := e.photoRequests(); n != 6 {
		t.Errorf("a first pass read %d photos, want 6", n)
	}
	withPhoto := 0
	for _, p := range e.people() {
		if strings.HasSuffix(p, "(photo)") {
			withPhoto++
		}
	}
	if withPhoto != 3 {
		t.Errorf("people = %v, want 3 with a photo", e.people())
	}

	// The stored token names a round Graph no longer knows: the next pass
	// reads every contact again, all unchanged.
	cols, err := e.db.Collections(ctx, store.CollectionFilter{AccountID: e.acct.ID, Kind: api.CollectionKindAddressbook})
	if err != nil || len(cols) == 0 {
		t.Fatalf("address books = %+v, %v", cols, err)
	}
	tok := strings.Replace(cols[0].SyncToken, "%24deltatoken=", "%24deltatoken=9", 1)
	if err := e.db.Tx(ctx, func(tx *store.Tx) error { return tx.SetCollectionSync(ctx, cols[0].ID, tok, "") }); err != nil {
		t.Fatal(err)
	}
	before := e.photoRequests()
	e.pass()
	if n := e.photoRequests() - before; n != 0 {
		t.Errorf("a full round of unchanged contacts read %d photos, want 0", n)
	}
	if got := e.people(); len(got) != 6 {
		t.Errorf("people after the full round = %v", got)
	}

	// Another client gives Person 1 a photo.
	e.srv.PutContact("contacts", graphPerson(1), jpeg)
	before = e.photoRequests()
	e.pass()
	if n := e.photoRequests() - before; n != 1 {
		t.Errorf("a pass with 1 changed contact read %d photos, want 1", n)
	}
	withPhoto = 0
	for _, p := range e.people() {
		if strings.HasSuffix(p, "(photo)") {
			withPhoto++
		}
	}
	if withPhoto != 4 {
		t.Errorf("people = %v, want 4 with a photo", e.people())
	}
}

// A pass reads a Google task list's stored tasks in 1 query, not 1 for
// each task the server sends.
func TestGoogleTasksReadStoredTasksOncePerList(t *testing.T) {
	e := newTasksEnv(t, allScopes())
	ops := countOps(e.db)
	work := e.srv.AddList("Work")
	for i := range 12 {
		e.srv.AddTask(work, gtaskstest.Task{Title: fmt.Sprint("Task ", i)})
	}
	e.mustPass()
	if n := ops.take(store.OpObjectByHref); n != 0 {
		t.Errorf("a pass of 12 tasks read %d tasks one by one, want none", n)
	}
	if n := ops.take(store.OpObjectRaws); n != 1 {
		t.Errorf("a pass of 1 list read its stored tasks %d times, want once", n)
	}
	if got := e.titles(store.TaskFilter{}); len(got) != 12 {
		t.Errorf("tasks = %q", got)
	}
	// An unchanged task is left alone; a changed one is stored again.
	tasks := e.srv.Tasks(work)
	e.srv.Update(work, tasks[0].ID, func(t *gtaskstest.Task) { t.Title = "Renamed" })
	e.mustPass()
	if n, m := ops.take(store.OpObjectByHref), ops.take(store.OpObjectRaws); n != 0 || m != 1 {
		t.Errorf("a second pass read %d tasks one by one and the list %d times, want none and once", n, m)
	}
	if got := e.titles(store.TaskFilter{}); len(got) != 12 || !slices.Contains(got, "Renamed") {
		t.Errorf("tasks after the rename = %q", got)
	}
}
