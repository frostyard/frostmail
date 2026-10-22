package engine

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/vcardx"
)

// raceEnv is an account with an address book, a calendar and a task list
// on a DAV server, and the services that write to them. It starts a race a
// sync pass wins: a write reads its object, a pass settles the object's
// creation, and the write stores what it read.
type raceEnv struct {
	t      *testing.T
	db     *store.DB
	dav    *davtest.Server
	m      *pimsync.Manager
	acct   int64
	events []api.EventEnvelope
	book   int64
	cal    int64
	list   int64

	calendar calendarService
	tasks    tasksService
	people   people
}

func newRaceEnv(t *testing.T) *raceEnv {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := &raceEnv{t: t, db: db, dav: davtest.New(t, davtest.Options{})}
	db.OnCommit = func(evs []api.EventEnvelope) { e.events = append(e.events, evs...) }
	e.dav.AddressBook("default", "Contacts")
	e.dav.Calendar("work", "Work", "#3366cc", "VEVENT")
	e.dav.Calendar("todo", "Reminders", "", "VTODO")
	server := store.ServerConfig{Host: "mail.example", Port: 993, TLS: api.TLSModeTLS, Username: davtest.User}
	err = db.Tx(ctx, func(tx *store.Tx) error {
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "user@example.com",
			Auth: api.AuthKindPassword, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		e.acct = acct.ID
		if err := tx.SetService(ctx, acct.ID, api.ServiceKindContacts, true, e.dav.URL+davtest.ContactsHome); err != nil {
			return err
		}
		for _, s := range []api.ServiceKind{api.ServiceKindCalendar, api.ServiceKindTasks} {
			if err := tx.SetService(ctx, acct.ID, s, true, e.dav.URL+davtest.CalendarsHome); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sec := secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json"))
	if err := sec.Set(ctx, secrets.AccountPassword(e.acct), davtest.Password); err != nil {
		t.Fatal(err)
	}
	e.m = pimsync.New(db, sec, slog.New(slog.NewTextHandler(io.Discard, nil)), pimsync.Config{HTTP: e.dav.Client, Batch: 2})
	e.pass()
	for kind, id := range map[api.CollectionKind]*int64{api.CollectionKindAddressbook: &e.book,
		api.CollectionKindCalendar: &e.cal, api.CollectionKindTasklist: &e.list} {
		cols, err := db.Collections(ctx, store.CollectionFilter{AccountID: e.acct, Kind: kind})
		if err != nil || len(cols) != 1 {
			t.Fatalf("%s collections = %+v, %v", kind, cols, err)
		}
		*id = cols[0].ID
	}
	d := Deps{DB: db, PIM: e.m}
	e.calendar, e.tasks, e.people = calendarService{d}, tasksService{d}, people{d}
	return e
}

// pass runs a sync pass of the account.
func (e *raceEnv) pass() {
	e.t.Helper()
	if err := e.m.Pass(e.t.Context(), e.acct); err != nil {
		e.t.Fatalf("Pass: %v", err)
	}
}

// refused lists the refusals the passes reported.
func (e *raceEnv) refused() []string {
	var out []string
	for _, ev := range e.events {
		if strings.HasSuffix(ev.Event, ".refused") {
			out = append(out, ev.Event+" "+string(ev.Data))
		}
	}
	return out
}

// newEvent creates an event whose creation is queued.
func (e *raceEnv) newEvent() int64 {
	e.t.Helper()
	start := time.Date(2026, 10, 25, 14, 0, 0, 0, time.UTC)
	w, err := e.calendar.Create(e.t.Context(), &api.CalendarCreateParams{CalendarID: &e.cal, Fields: api.EventFields{
		Summary: pointerTo("Review"), AllDay: pointerTo(false), Start: &start, End: pointerTo(start.Add(time.Hour)), TimeZone: pointerTo("UTC")}})
	if err != nil || w.Event == nil {
		e.t.Fatalf("create event = %+v, %v", w, err)
	}
	row, err := e.db.Event(e.t.Context(), w.Event.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return row.ObjectID
}

// newTask creates a task whose creation is queued.
func (e *raceEnv) newTask() int64 {
	e.t.Helper()
	task, err := e.tasks.Create(e.t.Context(), &api.TasksCreateParams{ListID: &e.list, Title: "Call Ann"})
	if err != nil {
		e.t.Fatal(err)
	}
	return task.ID
}

// newContact creates a contact whose creation is queued.
func (e *raceEnv) newContact() int64 {
	e.t.Helper()
	w, err := e.people.Create(e.t.Context(), &api.PeopleCreateParams{CollectionID: &e.book,
		Fields: api.ContactFields{GivenName: pointerTo("Ann"), FamilyName: pointerTo("Smith")}})
	if err != nil {
		e.t.Fatal(err)
	}
	return w.ContactID
}

// ops lists the changes that wait.
func (e *raceEnv) ops() []store.PIMOp {
	e.t.Helper()
	ops, err := e.db.DuePIMOps(e.t.Context(), e.acct, e.db.Now().Add(24*time.Hour))
	if err != nil {
		e.t.Fatal(err)
	}
	return ops
}

// settleIn does in tx what a pass does when it writes an object's queued
// creation: the server's ETag goes in the store, and the change is done.
func (e *raceEnv) settleIn(tx *store.Tx, id int64) {
	e.t.Helper()
	ctx := e.t.Context()
	if err := tx.SetObjectETag(ctx, id, `"e1"`); err != nil {
		e.t.Fatal(err)
	}
	for _, op := range e.ops() {
		if op.ObjectID != nil && *op.ObjectID == id && op.Kind == "put" && op.IfMatch == "" {
			if err := tx.DonePIMOp(ctx, op.ID); err != nil {
				e.t.Fatal(err)
			}
			return
		}
	}
	e.t.Fatalf("no creation of object %d waits in %+v", id, e.ops())
}

// settle is a pass that wrote the object's creation.
func (e *raceEnv) settle(id int64) {
	e.t.Helper()
	if err := e.db.Tx(e.t.Context(), func(tx *store.Tx) error { e.settleIn(tx, id); return nil }); err != nil {
		e.t.Fatal(err)
	}
}

// replace is a pass that brought another source of the object.
func (e *raceEnv) replace(id int64) store.Object {
	e.t.Helper()
	ctx := e.t.Context()
	obj, err := e.db.GetObject(ctx, id)
	if err != nil {
		e.t.Fatal(err)
	}
	obj.Raw = append(bytes.Clone(obj.Raw), "X-CHANGED:by another client\r\n"...)
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.PutObject(ctx, obj)
		return err
	}); err != nil {
		e.t.Fatal(err)
	}
	return obj
}

// wantSource checks that the stored source is the one a pass brought.
func (e *raceEnv) wantSource(passed store.Object) {
	e.t.Helper()
	obj, err := e.db.GetObject(e.t.Context(), passed.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	if !bytes.Equal(obj.Raw, passed.Raw) {
		e.t.Errorf("the pass's source was replaced:\n%s", obj.Raw)
	}
}

// wantETag checks that the object keeps the ETag a pass wrote.
func (e *raceEnv) wantETag(id int64) {
	e.t.Helper()
	obj, err := e.db.GetObject(e.t.Context(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	if obj.ETag != `"e1"` {
		e.t.Errorf("the object's etag = %q; want %q", obj.ETag, `"e1"`)
	}
}

// wantOp checks that one change waits, of the kind, on the ETag the pass
// wrote.
func (e *raceEnv) wantOp(kind string) {
	e.t.Helper()
	ops := e.ops()
	if len(ops) != 1 || ops[0].Kind != kind || ops[0].IfMatch != `"e1"` {
		e.t.Errorf("ops = %+v; want one %s on %q", ops, kind, `"e1"`)
	}
}

// TestEventUpdateKeepsTheETagAPassWrote: an update written after a pass
// settled the event's creation queues its put on the pass's ETag.
func TestEventUpdateKeepsTheETagAPassWrote(t *testing.T) {
	e := newRaceEnv(t)
	id := e.newEvent()
	w, err := e.calendar.prepareUpdate(t.Context(), &api.CalendarUpdateParams{ID: eventOf(t, e, id),
		Fields: api.EventFields{Summary: pointerTo("Review moved")}})
	if err != nil {
		t.Fatal(err)
	}
	e.settle(id)
	if _, err := e.calendar.writeUpdate(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	e.wantOp("put")
	e.wantETag(id)
}

// TestEventDeleteKeepsTheETagAPassWrote: a delete written after a pass
// settled the event's creation queues the server's delete.
func TestEventDeleteKeepsTheETagAPassWrote(t *testing.T) {
	e := newRaceEnv(t)
	id := e.newEvent()
	w, err := e.calendar.prepareDelete(t.Context(), &api.CalendarDeleteParams{ID: eventOf(t, e, id)})
	if err != nil {
		t.Fatal(err)
	}
	e.settle(id)
	if _, err := e.calendar.writeDelete(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	e.wantOp("delete")
}

// TestEventWriteOfAChangedSourceStartsAgain: a write whose event a pass
// replaced since it was read writes nothing and reports the change.
func TestEventWriteOfAChangedSourceStartsAgain(t *testing.T) {
	e := newRaceEnv(t)
	id := e.newEvent()
	w, err := e.calendar.prepareUpdate(t.Context(), &api.CalendarUpdateParams{ID: eventOf(t, e, id),
		Fields: api.EventFields{Summary: pointerTo("Review moved")}})
	if err != nil {
		t.Fatal(err)
	}
	passed := e.replace(id)
	if _, err := e.calendar.writeUpdate(t.Context(), w); !errors.Is(err, errObjectChanged) {
		t.Fatalf("writeUpdate = %v; want errObjectChanged", err)
	}
	e.wantSource(passed)
}

func eventOf(t *testing.T, e *raceEnv, objectID int64) int64 {
	t.Helper()
	rows, err := e.db.Events(t.Context(), store.EventsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ObjectID == objectID && r.RecurrenceID == "" {
			return r.ID
		}
	}
	t.Fatalf("no event of object %d", objectID)
	return 0
}

// TestTaskUpdateKeepsTheETagAPassWrote: an update written after a pass
// settled the task's creation queues its put on the pass's ETag.
func TestTaskUpdateKeepsTheETagAPassWrote(t *testing.T) {
	e := newRaceEnv(t)
	id := e.newTask()
	w, err := e.tasks.prepareUpdate(t.Context(), &api.TasksUpdateParams{ID: id, Title: pointerTo("Call Ann back")})
	if err != nil {
		t.Fatal(err)
	}
	e.settle(id)
	if _, err := e.tasks.writeUpdate(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	e.wantOp("put")
	e.wantETag(id)
}

// TestTaskDeleteSeesAnETagInItsTransaction: a delete reads the ETag a pass
// wrote in the same transaction, which the store's reads do not see yet.
func TestTaskDeleteSeesAnETagInItsTransaction(t *testing.T) {
	ctx := t.Context()
	e := newRaceEnv(t)
	id := e.newTask()
	col, err := e.db.GetCollection(ctx, e.list)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		e.settleIn(tx, id)
		return e.tasks.deleteLocal(ctx, tx, col, false, []int64{id})
	}); err != nil {
		t.Fatal(err)
	}
	e.wantOp("delete")
}

// TestTaskDeleteOfAGoneTask: a task a pass removed since it was read is
// not found, and a subtask that is gone is skipped.
func TestTaskDeleteOfAGoneTask(t *testing.T) {
	ctx := t.Context()
	e := newRaceEnv(t)
	id, sub := e.newTask(), e.newTask()
	col, err := e.db.GetCollection(ctx, e.list)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		return e.tasks.deleteLocal(ctx, tx, col, false, []int64{id, sub + 1000})
	}); err != nil {
		t.Fatalf("a gone subtask = %v; want it skipped", err)
	}
	err = e.db.Tx(ctx, func(tx *store.Tx) error { return e.tasks.deleteLocal(ctx, tx, col, false, []int64{id}) })
	if errCode(err) != api.CodeNotFound {
		t.Errorf("a gone task = %v; want notFound", err)
	}
}

// TestTaskWriteOfAChangedSourceStartsAgain: an update whose task a pass
// replaced since it was read writes nothing and reports the change.
func TestTaskWriteOfAChangedSourceStartsAgain(t *testing.T) {
	e := newRaceEnv(t)
	id := e.newTask()
	w, err := e.tasks.prepareUpdate(t.Context(), &api.TasksUpdateParams{ID: id, Title: pointerTo("Call Ann back")})
	if err != nil {
		t.Fatal(err)
	}
	passed := e.replace(id)
	if _, err := e.tasks.writeUpdate(t.Context(), w); !errors.Is(err, errObjectChanged) {
		t.Fatalf("writeUpdate = %v; want errObjectChanged", err)
	}
	e.wantSource(passed)
}

// TestContactEditKeepsTheETagAPassWrote: an edit written after a pass
// settled the contact's creation queues its put on the pass's ETag.
func TestContactEditKeepsTheETagAPassWrote(t *testing.T) {
	e := newRaceEnv(t)
	id := e.newContact()
	w, err := e.people.prepareEdit(t.Context(), id, vcardFields("COO"))
	if err != nil {
		t.Fatal(err)
	}
	e.settle(id)
	if _, err := e.people.writeEdit(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	e.wantOp("put")
	e.wantETag(id)
}

// TestContactDeleteKeepsTheETagAPassWrote: a delete written after a pass
// settled the contact's creation queues the server's delete.
func TestContactDeleteKeepsTheETagAPassWrote(t *testing.T) {
	e := newRaceEnv(t)
	id := e.newContact()
	w, err := e.people.prepareDelete(t.Context(), &api.PeopleDeleteParams{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	e.settle(id)
	if _, err := e.people.writeDelete(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	e.wantOp("delete")
}

// TestContactWriteOfAChangedSourceStartsAgain: an edit whose contact a pass
// replaced since it was read writes nothing and reports the change.
func TestContactWriteOfAChangedSourceStartsAgain(t *testing.T) {
	e := newRaceEnv(t)
	id := e.newContact()
	w, err := e.people.prepareEdit(t.Context(), id, vcardFields("COO"))
	if err != nil {
		t.Fatal(err)
	}
	passed := e.replace(id)
	if _, err := e.people.writeEdit(t.Context(), w); !errors.Is(err, errObjectChanged) {
		t.Fatalf("writeEdit = %v; want errObjectChanged", err)
	}
	e.wantSource(passed)
}

// TestRetryWrite: a write that reports a changed object runs again, up to
// writeAttempts times, and ends in a conflict; any other result returns at
// once.
func TestRetryWrite(t *testing.T) {
	calls := 0
	_, err := retryWrite("the thing", func() (int, error) {
		calls++
		return 0, errObjectChanged
	})
	if errCode(err) != api.CodeConflict || calls != writeAttempts {
		t.Errorf("err = %v after %d calls; want a conflict after %d", err, calls, writeAttempts)
	}

	calls = 0
	got, err := retryWrite("the thing", func() (int, error) {
		calls++
		if calls < 2 {
			return 0, errObjectChanged
		}
		return 7, nil
	})
	if err != nil || got != 7 || calls != 2 {
		t.Errorf("got %d, %v after %d calls; want 7 after 2", got, err, calls)
	}

	boom := errors.New("boom")
	calls = 0
	if _, err := retryWrite("the thing", func() (int, error) {
		calls++
		return 0, boom
	}); !errors.Is(err, boom) || calls != 1 {
		t.Errorf("err = %v after %d calls; want boom after 1", err, calls)
	}
}

// The 412 path: a write made on the object before a pass wrote its creation
// is stored after it. Its put or delete must carry the server's ETag. On an
// empty one, the server refuses the creation of an object it has, and the
// pass removed the local object.

// passAfterWrite runs the pass that sends the write, and checks that it
// changed the server's object, never created it: a creation of an object
// the server has is a 412, which an event or contact edit survives only by
// applying itself again.
func (e *raceEnv) passAfterWrite() {
	e.t.Helper()
	e.dav.ResetRequests()
	e.pass()
	for _, r := range e.dav.Requests() {
		if r.Method == "PUT" && r.Header.Get("If-None-Match") != "" {
			e.t.Errorf("a creation of an object the server has: %s %s", r.Method, r.Path)
		}
	}
}

func (e *raceEnv) wantNoRefusal() {
	e.t.Helper()
	if r := e.refused(); len(r) != 0 {
		e.t.Errorf("refused = %q", r)
	}
}

func (e *raceEnv) wantLocal(id int64) store.Object {
	e.t.Helper()
	obj, err := e.db.GetObject(e.t.Context(), id)
	if err != nil {
		e.t.Fatalf("the local object is gone: %v", err)
	}
	return obj
}

func (e *raceEnv) wantGone(id int64) {
	e.t.Helper()
	if _, err := e.db.GetObject(e.t.Context(), id); !errors.Is(err, store.ErrNotFound) {
		e.t.Errorf("the local object stays: %v", err)
	}
}

func (e *raceEnv) wantServer(href, want string) {
	e.t.Helper()
	data, _, ok := e.dav.Object(href)
	if !ok || !strings.Contains(string(data), want) {
		e.t.Errorf("%s on the server (found %v) = %s; want %q", href, ok, data, want)
	}
}

func (e *raceEnv) wantNotOnServer(href string) {
	e.t.Helper()
	if data, _, ok := e.dav.Object(href); ok {
		e.t.Errorf("%s is still on the server: %s", href, data)
	}
}

func TestEventUpdateAfterAPassReachesTheServer(t *testing.T) {
	e := newRaceEnv(t)
	ctx := t.Context()
	id := e.newEvent()
	w, err := e.calendar.prepareUpdate(ctx, &api.CalendarUpdateParams{ID: eventOf(t, e, id),
		Fields: api.EventFields{Summary: pointerTo("Review moved")}})
	if err != nil {
		t.Fatal(err)
	}
	e.pass()
	if _, err := e.calendar.writeUpdate(ctx, w); err != nil {
		t.Fatal(err)
	}
	e.passAfterWrite()
	obj := e.wantLocal(id)
	e.wantServer(obj.Href, "SUMMARY:Review moved")
	e.wantNoRefusal()
}

func TestEventDeleteAfterAPassReachesTheServer(t *testing.T) {
	e := newRaceEnv(t)
	ctx := t.Context()
	id := e.newEvent()
	obj := e.wantLocal(id)
	w, err := e.calendar.prepareDelete(ctx, &api.CalendarDeleteParams{ID: eventOf(t, e, id)})
	if err != nil {
		t.Fatal(err)
	}
	e.pass()
	e.wantServer(obj.Href, "SUMMARY:Review")
	if _, err := e.calendar.writeDelete(ctx, w); err != nil {
		t.Fatal(err)
	}
	e.passAfterWrite()
	e.wantGone(id)
	e.wantNotOnServer(obj.Href)
	e.wantNoRefusal()
}

func TestTaskUpdateAfterAPassReachesTheServer(t *testing.T) {
	e := newRaceEnv(t)
	ctx := t.Context()
	id := e.newTask()
	w, err := e.tasks.prepareUpdate(ctx, &api.TasksUpdateParams{ID: id, Title: pointerTo("Call Ann back")})
	if err != nil {
		t.Fatal(err)
	}
	e.pass()
	if _, err := e.tasks.writeUpdate(ctx, w); err != nil {
		t.Fatal(err)
	}
	e.passAfterWrite()
	obj := e.wantLocal(id)
	e.wantServer(obj.Href, "SUMMARY:Call Ann back")
	e.wantNoRefusal()
}

func TestTaskDeleteAfterAPassReachesTheServer(t *testing.T) {
	e := newRaceEnv(t)
	ctx := t.Context()
	id := e.newTask()
	col, err := e.db.GetCollection(ctx, e.list)
	if err != nil {
		t.Fatal(err)
	}
	obj := e.wantLocal(id)
	e.pass()
	e.wantServer(obj.Href, "SUMMARY:Call Ann")
	// deleteLocal reads in its transaction, so a pass that settled the
	// creation before it is seen whole.
	if err := e.db.Tx(ctx, func(tx *store.Tx) error { return e.tasks.deleteLocal(ctx, tx, col, false, []int64{id}) }); err != nil {
		t.Fatal(err)
	}
	e.pass()
	e.wantGone(id)
	e.wantNotOnServer(obj.Href)
	e.wantNoRefusal()
}

func TestContactEditAfterAPassReachesTheServer(t *testing.T) {
	e := newRaceEnv(t)
	ctx := t.Context()
	id := e.newContact()
	w, err := e.people.prepareEdit(ctx, id, vcardFields("COO"))
	if err != nil {
		t.Fatal(err)
	}
	e.pass()
	if _, err := e.people.writeEdit(ctx, w); err != nil {
		t.Fatal(err)
	}
	e.passAfterWrite()
	obj := e.wantLocal(id)
	e.wantServer(obj.Href, "TITLE:COO")
	e.wantNoRefusal()
}

func TestContactDeleteAfterAPassReachesTheServer(t *testing.T) {
	e := newRaceEnv(t)
	ctx := t.Context()
	id := e.newContact()
	obj := e.wantLocal(id)
	w, err := e.people.prepareDelete(ctx, &api.PeopleDeleteParams{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	e.pass()
	e.wantServer(obj.Href, "FN:Ann Smith")
	if _, err := e.people.writeDelete(ctx, w); err != nil {
		t.Fatal(err)
	}
	e.passAfterWrite()
	e.wantGone(id)
	e.wantNotOnServer(obj.Href)
	e.wantNoRefusal()
}

// vcardFields is a patch that sets a contact's title.
func vcardFields(title string) vcardx.Fields { return vcardx.Fields{Title: &title} }

func pointerTo[T any](v T) *T { return &v }

// errCode is the code of an API error, or 0 for another error.
func errCode(err error) api.ErrorCode {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}
