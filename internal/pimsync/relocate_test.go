package pimsync_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/store"
)

// TestRelocatedCreation: Google's CalDAV stores a new object under a name
// from its UID. Accepting an invitation not in the calendar creates one
// whose UID is the organizer's and whose name is Frostmail's, so the
// server says where it went (Location). One object stays here, under the
// server's name, and the next pass finds nothing new.
func TestRelocatedCreation(t *testing.T) {
	e := newEnv(t, davtest.Options{RelocateCreates: true})
	ctx := t.Context()
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		return tx.SetService(ctx, e.acct.ID, api.ServiceKindTasks, true, e.dav.URL+davtest.CalendarsHome)
	}); err != nil {
		t.Fatal(err)
	}
	todo := e.dav.Calendar("todo", "Reminders", "", "VTODO")
	e.pass()
	list := e.collections(api.CollectionKindTasklist)[0]
	var id int64
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		id, err = tx.PutObject(ctx, store.Object{CollectionID: list.ID, Href: todo + "mine.ics", Kind: store.ObjectVTodo,
			UID: "abc@example.org", Raw: vtodo("abc@example.org", "Call Ann")})
		if err != nil {
			return err
		}
		_, err = tx.QueuePIMOp(ctx, store.PIMOp{AccountID: e.acct.ID, CollectionID: list.ID, ObjectID: &id, Kind: "put", Href: todo + "mine.ics"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	want := todo + "abc@example.org.ics"
	obj, err := e.db.GetObject(ctx, id)
	if err != nil || obj.Href != want {
		t.Fatalf("object = %s, %v; want it at %s", obj.Href, err, want)
	}
	if _, etag, ok := e.dav.Object(want); !ok || obj.ETag != etag {
		t.Errorf("etag = %q, server %q (%v)", obj.ETag, etag, ok)
	}
	if ops, _ := e.db.DuePIMOps(ctx, e.acct.ID, e.db.Now()); len(ops) != 0 {
		t.Errorf("ops left = %+v", ops)
	}
	e.pass()
	if etags, _ := e.db.ObjectETags(ctx, list.ID); len(etags) != 1 {
		t.Errorf("objects = %v", etags)
	}
	if n := e.requests()["REPORT "+todo+" calendar-multiget"]; n != 0 {
		t.Errorf("the next pass fetched again: %v", e.requests())
	}
	if titles := e.taskTitles(); len(titles) != 1 || titles[0] != "Call Ann" {
		t.Errorf("tasks = %q", titles)
	}
}

// TestRelocatedOntoSynced: the server already has the object under its
// own name, and a pass brought it here before the creation was retried
// (a first attempt whose answer was lost). The retry leaves one object.
func TestRelocatedOntoSynced(t *testing.T) {
	e := newEnv(t, davtest.Options{RelocateCreates: true})
	ctx := t.Context()
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		return tx.SetService(ctx, e.acct.ID, api.ServiceKindTasks, true, e.dav.URL+davtest.CalendarsHome)
	}); err != nil {
		t.Fatal(err)
	}
	todo := e.dav.Calendar("todo", "Reminders", "", "VTODO")
	theirs := todo + "abc@example.org.ics"
	e.dav.Put(theirs, vtodo("abc@example.org", "Call Ann"))
	e.pass()
	list := e.collections(api.CollectionKindTasklist)[0]
	var id int64
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		id, err = tx.PutObject(ctx, store.Object{CollectionID: list.ID, Href: todo + "mine.ics", Kind: store.ObjectVTodo,
			UID: "abc@example.org", Raw: vtodo("abc@example.org", "Call Ann back")})
		if err != nil {
			return err
		}
		_, err = tx.QueuePIMOp(ctx, store.PIMOp{AccountID: e.acct.ID, CollectionID: list.ID, ObjectID: &id, Kind: "put", Href: todo + "mine.ics"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	etags, _ := e.db.ObjectETags(ctx, list.ID)
	if _, ok := etags[theirs]; len(etags) != 1 || !ok {
		t.Errorf("objects = %v", etags)
	}
	if titles := e.taskTitles(); len(titles) != 1 || titles[0] != "Call Ann back" {
		t.Errorf("tasks = %q", titles)
	}
	if ops, _ := e.db.DuePIMOps(ctx, e.acct.ID, e.db.Now()); len(ops) != 0 {
		t.Errorf("ops left = %+v", ops)
	}
}
