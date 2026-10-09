package pimsync_test

// CONTRACT TEST for task card T-0061 (docs/tasks). Do not edit.

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// env is a store, a DAV server and a Manager for one account.
type env struct {
	t      *testing.T
	db     *store.DB
	dav    *davtest.Server
	m      *pimsync.Manager
	acct   store.Account
	events []api.EventEnvelope
}

func newEnv(t *testing.T, opts davtest.Options, services ...api.ServiceKind) *env {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := &env{t: t, db: db, dav: davtest.New(t, opts)}
	db.OnCommit = func(evs []api.EventEnvelope) { e.events = append(e.events, evs...) }
	server := store.ServerConfig{Host: "mail.example", Port: 993, TLS: api.TLSModeTLS, Username: davtest.User}
	err = db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		e.acct, err = tx.InsertAccount(ctx, store.Account{
			Kind: api.AccountKindIMAP, Email: "user@example.com", Auth: api.AuthKindPassword, IMAP: server, SMTP: server,
		})
		if err != nil {
			return err
		}
		for _, s := range services {
			home := davtest.ContactsHome
			if s == api.ServiceKindCalendar {
				home = davtest.CalendarsHome
			}
			if err := tx.SetService(ctx, e.acct.ID, s, true, e.dav.URL+home); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sec := secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json"))
	if err := sec.Set(ctx, secrets.AccountPassword(e.acct.ID), davtest.Password); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e.m = pimsync.New(db, sec, log, pimsync.Config{HTTP: e.dav.Client, Batch: 2})
	return e
}

func (e *env) pass() {
	e.t.Helper()
	e.dav.ResetRequests()
	e.events = nil
	if err := e.m.Pass(e.t.Context(), e.acct.ID); err != nil {
		e.t.Fatalf("Pass: %v", err)
	}
}

func (e *env) collections(kind api.CollectionKind) []store.Collection {
	e.t.Helper()
	cs, err := e.db.Collections(e.t.Context(), store.CollectionFilter{AccountID: e.acct.ID, Kind: kind})
	if err != nil {
		e.t.Fatal(err)
	}
	return cs
}

func (e *env) collection(kind api.CollectionKind, href string) store.Collection {
	e.t.Helper()
	for _, c := range e.collections(kind) {
		if c.Href == href {
			return c
		}
	}
	e.t.Fatalf("no collection %s", href)
	return store.Collection{}
}

// contacts maps each object's href in a collection to its contact's
// display name, or to "!" + its parse error, or to "-" when the object is
// not a contact.
func (e *env) contacts(col store.Collection) map[string]string {
	e.t.Helper()
	ctx := e.t.Context()
	etags, err := e.db.ObjectETags(ctx, col.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	out := map[string]string{}
	for href := range etags {
		o, err := e.db.ObjectByHref(ctx, col.ID, href)
		if err != nil {
			e.t.Fatal(err)
		}
		c, err := e.db.Contact(ctx, o.ID)
		switch {
		case o.ParseError != "":
			out[href] = "!" + o.ParseError
		case errors.Is(err, store.ErrNotFound):
			out[href] = "-"
		case err != nil:
			e.t.Fatal(err)
		default:
			out[href] = c.DisplayName
		}
	}
	return out
}

// requests counts the requests of the last pass by "METHOD path report".
func (e *env) requests() map[string]int {
	out := map[string]int{}
	for _, r := range e.dav.Requests() {
		out[strings.TrimSpace(r.Method+" "+r.Path+" "+r.Report)]++
	}
	return out
}

func (e *env) eventNames() []string {
	var out []string
	for _, ev := range e.events {
		out = append(out, ev.Event)
	}
	return out
}

func vcard(uid, name string, emails ...string) []byte {
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + uid + "\r\nFN:" + name + "\r\n")
	for _, m := range emails {
		b.WriteString("EMAIL;TYPE=INTERNET:" + m + "\r\n")
	}
	b.WriteString("END:VCARD\r\n")
	return []byte(b.String())
}

const groupCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:g\r\nFN:Club\r\nX-ADDRESSBOOKSERVER-KIND:group\r\nEND:VCARD\r\n"

func TestFirstSync(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	work := e.dav.AddressBook("work", "Work")
	e.dav.Put(book+"ada.vcf", vcard("ada", "Ada Lovelace", "Ada@Example.com", "ada@work.example"))
	e.dav.Put(book+"grace.vcf", vcard("grace", "Grace Hopper", "grace@navy.example"))
	e.dav.Put(book+"club.vcf", []byte(groupCard))
	e.dav.Put(book+"broken.vcf", []byte("BEGIN:VCARD\r\nFN:Never closed\r\n"))
	e.dav.Put(book+"alan.vcf", vcard("alan", "Alan Turing"))
	e.dav.Put(work+"boss.vcf", vcard("boss", "The Boss", "boss@work.example"))

	e.pass()
	cols := e.collections(api.CollectionKindAddressbook)
	if len(cols) != 2 || cols[0].Href != book || cols[0].Name != "Contacts" || !cols[0].IsDefault || cols[1].Href != work {
		t.Fatalf("collections = %+v", cols)
	}
	got := e.contacts(cols[0])
	if got[book+"ada.vcf"] != "Ada Lovelace" || got[book+"grace.vcf"] != "Grace Hopper" || got[book+"alan.vcf"] != "Alan Turing" ||
		got[book+"club.vcf"] != "-" || !strings.HasPrefix(got[book+"broken.vcf"], "!") || len(got) != 5 {
		t.Errorf("contacts = %v", got)
	}
	if w := e.contacts(cols[1]); len(w) != 1 || w[work+"boss.vcf"] != "The Boss" {
		t.Errorf("work contacts = %v", w)
	}

	// Objects are stored as sent, with their ETags and UIDs, and indexed.
	ctx := t.Context()
	o, err := e.db.ObjectByHref(ctx, cols[0].ID, book+"ada.vcf")
	raw, etag, _ := e.dav.Object(book + "ada.vcf")
	if err != nil || string(o.Raw) != string(raw) || o.ETag != etag || o.UID != "ada" || o.Kind != store.ObjectVCard {
		t.Errorf("ada = %+v, %v", o, err)
	}
	c, _ := e.db.Contact(ctx, o.ID)
	if len(c.Emails) != 2 || c.Emails[0].Email != "ada@example.com" || c.SortKey != "ada lovelace" {
		t.Errorf("ada's index = %+v", c)
	}
	broken, _ := e.db.ObjectByHref(ctx, cols[0].ID, book+"broken.vcf")
	if string(broken.Raw) != "BEGIN:VCARD\r\nFN:Never closed\r\n" || broken.Kind != store.ObjectVCard {
		t.Errorf("a source that does not parse is kept: %+v", broken)
	}

	// One sync-collection per book, multigets in batches of two.
	reqs := e.requests()
	if reqs["REPORT "+book+" sync-collection"] != 1 || reqs["REPORT "+work+" sync-collection"] != 1 ||
		reqs["REPORT "+book+" addressbook-multiget"] != 3 || reqs["REPORT "+work+" addressbook-multiget"] != 1 {
		t.Errorf("requests = %v", reqs)
	}
	for _, r := range e.dav.Requests() {
		if r.Report == "addressbook-multiget" && r.Hrefs > 2 {
			t.Errorf("a multiget of %d hrefs; the batch is 2", r.Hrefs)
		}
	}
	if cols[0].SyncToken == "" || cols[0].SyncedAt == nil {
		t.Errorf("sync state = %+v", cols[0])
	}
	names := e.eventNames()
	if !slices.Contains(names, "account.changed") || !slices.Contains(names, "people.changed") {
		t.Errorf("events = %q", names)
	}
	svc, _ := e.db.Services(ctx, e.acct.ID)
	if len(svc) != 1 || svc[0].LastSyncAt == nil || svc[0].LastError != "" {
		t.Errorf("service = %+v", svc)
	}
}

func TestIncrementalSync(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	for _, n := range []string{"a", "b", "c", "d"} {
		e.dav.Put(book+n+".vcf", vcard(n, "Person "+strings.ToUpper(n)))
	}
	e.pass()
	before := e.collection(api.CollectionKindAddressbook, book)

	e.dav.Put(book+"a.vcf", vcard("a", "Changed A"))
	e.dav.Delete(book + "b.vcf")
	e.dav.Put(book+"e.vcf", vcard("e", "Person E"))
	e.pass()
	got := e.contacts(e.collection(api.CollectionKindAddressbook, book))
	want := map[string]string{book + "a.vcf": "Changed A", book + "c.vcf": "Person C", book + "d.vcf": "Person D", book + "e.vcf": "Person E"}
	if len(got) != len(want) {
		t.Errorf("contacts = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	var syncs []davtest.Request
	for _, r := range e.dav.Requests() {
		if r.Report == "sync-collection" {
			syncs = append(syncs, r)
		}
		if r.Report == "addressbook-multiget" && r.Hrefs != 2 {
			t.Errorf("multiget of %d hrefs; only a and e changed", r.Hrefs)
		}
	}
	if len(syncs) != 1 || syncs[0].Token != before.SyncToken {
		t.Errorf("sync-collection requests = %+v, want one from token %q", syncs, before.SyncToken)
	}
	if !slices.Contains(e.eventNames(), "people.changed") {
		t.Errorf("events = %q", e.eventNames())
	}

	// Nothing changed: the ctag says so, and no collection is asked.
	e.pass()
	reqs := e.requests()
	for k := range reqs {
		if strings.HasPrefix(k, "REPORT") {
			t.Errorf("a quiet pass sent %s", k)
		}
	}
	if slices.Contains(e.eventNames(), "people.changed") || slices.Contains(e.eventNames(), "account.changed") {
		t.Errorf("a quiet pass emitted %q", e.eventNames())
	}
}

func TestInvalidToken(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	for _, n := range []string{"a", "b", "c"} {
		e.dav.Put(book+n+".vcf", vcard(n, "Person "+n))
	}
	e.pass()
	e.dav.InvalidateTokens()
	e.dav.Delete(book + "b.vcf")
	e.dav.Put(book+"c.vcf", vcard("c", "Changed c"))
	e.pass()
	got := e.contacts(e.collection(api.CollectionKindAddressbook, book))
	if len(got) != 2 || got[book+"a.vcf"] != "Person a" || got[book+"c.vcf"] != "Changed c" {
		t.Errorf("contacts = %v", got)
	}
	var tokens []string
	fetched := 0
	for _, r := range e.dav.Requests() {
		if r.Report == "sync-collection" {
			tokens = append(tokens, r.Token)
		}
		if r.Report == "addressbook-multiget" {
			fetched += r.Hrefs
		}
	}
	if len(tokens) != 2 || tokens[0] == "" || tokens[1] != "" {
		t.Errorf("sync tokens sent = %q, want the stale one, then a full listing", tokens)
	}
	if fetched != 1 {
		t.Errorf("fetched %d objects; only c changed", fetched)
	}
}

func TestWithoutSyncCollection(t *testing.T) {
	e := newEnv(t, davtest.Options{NoSync: true}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	e.dav.Put(book+"a.vcf", vcard("a", "A"))
	e.dav.Put(book+"b.vcf", vcard("b", "B"))
	e.pass()
	e.dav.Delete(book + "a.vcf")
	e.dav.Put(book+"b.vcf", vcard("b", "B2"))
	e.dav.Put(book+"c.vcf", vcard("c", "C"))
	e.pass()
	got := e.contacts(e.collection(api.CollectionKindAddressbook, book))
	if len(got) != 2 || got[book+"b.vcf"] != "B2" || got[book+"c.vcf"] != "C" {
		t.Errorf("contacts = %v", got)
	}
	reqs := e.requests()
	if reqs["REPORT "+book+" sync-collection"] != 0 || reqs["PROPFIND "+book] != 1 {
		t.Errorf("requests = %v, want a PROPFIND listing and no sync-collection", reqs)
	}
}

func TestTruncatedSync(t *testing.T) {
	e := newEnv(t, davtest.Options{Truncate: 2}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	for _, n := range []string{"1", "2", "3", "4", "5"} {
		e.dav.Put(book+n+".vcf", vcard(n, "N"+n))
	}
	e.pass()
	if got := e.contacts(e.collection(api.CollectionKindAddressbook, book)); len(got) != 5 {
		t.Errorf("contacts = %v, want all five in one pass", got)
	}
}

func TestCollectionChanges(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	old := e.dav.AddressBook("old", "Old")
	quiet := e.dav.AddressBook("quiet", "Quiet")
	e.dav.Put(book+"a.vcf", vcard("a", "A"))
	e.dav.Put(old+"o.vcf", vcard("o", "O"))
	e.dav.Put(quiet+"q.vcf", vcard("q", "Q"))
	e.pass()
	q := e.collection(api.CollectionKindAddressbook, quiet)
	oldObj, _ := e.db.ObjectByHref(t.Context(), e.collection(api.CollectionKindAddressbook, old).ID, old+"o.vcf")

	off := false
	if err := e.db.Tx(t.Context(), func(tx *store.Tx) error {
		_, err := tx.UpdateCollection(t.Context(), q.ID, &off, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.dav.Rename(book, "Renamed")
	e.dav.RemoveCollection(old)
	e.dav.Put(quiet+"q2.vcf", vcard("q2", "Q2"))
	added := e.dav.AddressBook("new", "New")
	e.dav.Put(added+"n.vcf", vcard("n", "N"))
	e.pass()

	var hrefs []string
	for _, c := range e.collections(api.CollectionKindAddressbook) {
		hrefs = append(hrefs, c.Href)
	}
	if !slices.Equal(hrefs, []string{book, added, quiet}) && !slices.Equal(hrefs, []string{book, quiet, added}) {
		t.Errorf("collections = %q", hrefs)
	}
	if e.collection(api.CollectionKindAddressbook, book).Name != "Renamed" {
		t.Error("the rename did not arrive")
	}
	if _, err := e.db.GetObject(t.Context(), oldObj.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("an object of a removed collection: %v", err)
	}
	if got := e.contacts(e.collection(api.CollectionKindAddressbook, added)); got[added+"n.vcf"] != "N" {
		t.Errorf("new collection's contacts = %v", got)
	}
	if got := e.contacts(e.collection(api.CollectionKindAddressbook, quiet)); len(got) != 1 {
		t.Errorf("a disabled collection was synced: %v", got)
	}
	for k := range e.requests() {
		if strings.Contains(k, quiet) {
			t.Errorf("a disabled collection was asked: %s", k)
		}
	}
	if !slices.Contains(e.eventNames(), "account.changed") {
		t.Errorf("events = %q", e.eventNames())
	}
}

// TestPendingLeftAlone: an object with a change waiting to be written (not
// yet due, so the pass does not replay it) is neither overwritten nor
// deleted by a pass.
func TestPendingLeftAlone(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	book := e.dav.AddressBook("default", "Contacts")
	e.dav.Put(book+"a.vcf", vcard("a", "A"))
	e.dav.Put(book+"b.vcf", vcard("b", "B"))
	e.pass()
	ctx := t.Context()
	col := e.collection(api.CollectionKindAddressbook, book)
	for _, n := range []string{"a", "b"} {
		o, _ := e.db.ObjectByHref(ctx, col.ID, book+n+".vcf")
		if err := e.db.Tx(ctx, func(tx *store.Tx) error {
			_, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: e.acct.ID, CollectionID: col.ID, ObjectID: &o.ID,
				Kind: "put", Href: o.Href, IfMatch: o.ETag, NextTryAt: time.Now().Add(time.Hour)})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	e.dav.Put(book+"a.vcf", vcard("a", "Server A"))
	e.dav.Delete(book + "b.vcf")
	e.pass()
	got := e.contacts(e.collection(api.CollectionKindAddressbook, book))
	if got[book+"a.vcf"] != "A" || got[book+"b.vcf"] != "B" {
		t.Errorf("contacts = %v, want the local versions kept", got)
	}
}

func vevent(uid string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\nUID:" + uid +
		"\r\nDTSTAMP:20261001T000000Z\r\nDTSTART:20261009T090000Z\r\nDTEND:20261009T100000Z\r\nSUMMARY:Standup\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
}

func TestCalendars(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindCalendar)
	work := e.dav.Calendar("work", "Work", "#3366cc", "VEVENT", "VTODO")
	todo := e.dav.Calendar("todo", "Reminders", "", "VTODO")
	e.dav.Put(work+"standup.ics", vevent("standup-1"))
	e.dav.Put(work+"task.ics", []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VTODO\r\nUID:t1\r\nSUMMARY:Do\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"))
	e.dav.Put(work+"tz.ics", []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VTIMEZONE\r\nTZID:X\r\nEND:VTIMEZONE\r\nEND:VCALENDAR\r\n"))
	e.dav.Put(todo+"t2.ics", []byte("BEGIN:VCALENDAR\r\nBEGIN:VTODO\r\nUID:t2\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"))
	e.pass()

	cals := e.collections(api.CollectionKindCalendar)
	if len(cals) != 1 || cals[0].Href != work || cals[0].Color != "#3366cc" || !slices.Equal(cals[0].Components, []string{"VEVENT", "VTODO"}) {
		t.Fatalf("calendars = %+v; the VTODO-only one is not the calendar service's", cals)
	}
	ctx := t.Context()
	for href, want := range map[string]struct {
		kind store.ObjectKind
		uid  string
	}{
		work + "standup.ics": {store.ObjectVEvent, "standup-1"},
		work + "task.ics":    {store.ObjectVTodo, "t1"},
		work + "tz.ics":      {store.ObjectOther, ""},
	} {
		o, err := e.db.ObjectByHref(ctx, cals[0].ID, href)
		if err != nil || o.Kind != want.kind || o.UID != want.uid {
			t.Errorf("%s = %+v, %v; want %+v", href, o, err, want)
		}
	}
	if !slices.Contains(e.eventNames(), "calendar.changed") || slices.Contains(e.eventNames(), "people.changed") {
		t.Errorf("events = %q", e.eventNames())
	}
	if e.requests()["REPORT "+work+" calendar-multiget"] == 0 {
		t.Errorf("requests = %v", e.requests())
	}
}

func TestRefusedPassword(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindContacts)
	e.dav.AddressBook("default", "Contacts")
	sec := secrets.NewFile(filepath.Join(t.TempDir(), "other.json"))
	if err := sec.Set(t.Context(), secrets.AccountPassword(e.acct.ID), "wrong"); err != nil {
		t.Fatal(err)
	}
	m := pimsync.New(e.db, sec, slog.New(slog.NewTextHandler(io.Discard, nil)), pimsync.Config{HTTP: e.dav.Client})
	if err := m.Pass(t.Context(), e.acct.ID); !errors.Is(err, davx.ErrUnauthorized) {
		t.Errorf("Pass = %v, want davx.ErrUnauthorized", err)
	}
	svc, _ := e.db.Services(t.Context(), e.acct.ID)
	if svc[0].LastError == "" || svc[0].LastSyncAt != nil {
		t.Errorf("service = %+v", svc[0])
	}
}
