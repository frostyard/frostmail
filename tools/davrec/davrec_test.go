package main

// CONTRACT TEST for task card T-0092 (docs/tasks). Do not edit.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/gtasks/gtaskstest"
	"github.com/frostyard/frostmail/internal/httprec"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

var update = flag.Bool("update", false, "record testdata/session.trace again")

const (
	account = "ann.lee@example.com"
	name    = "Ann Lee"
	trace   = "testdata/session.trace"
	keyfile = "testdata/test.key"
)

// sensitive are values of the recorded session that must not survive
// scrubbing, in every form the session holds them.
var sensitive = []string{
	account, name, "Ada Lovelace", "Lovelace", "ada@example.com",
	"+44 20 7946 0000", "Countess of Lovelace", "Analytical Engines", "Grace Hopper", "grace@navy.example",
	"Arlington", "Design review", "Studio B", "Bring the latest numbers", "Maria Lopez", "maria@northwind.example",
	"standup-2026", "Send the Q3 report", "Numbers from Maria", "Buy oat milk", "Oat, not soy", "Check the price",
	"Errands", "Analytical",
}

type fakeTokens struct{}

func (fakeTokens) AccessToken(context.Context, int64) (string, error) { return gtaskstest.Token, nil }
func (fakeTokens) Invalidate(int64)                                   {}

// session is pimsync for Ann's two accounts: a DAV account (contacts,
// calendar, CalDAV tasks) at davBase and a Gmail account whose Google
// Tasks are at tasksBase.
type session struct {
	db         *store.DB
	m          *pimsync.Manager
	dav, gmail int64
}

func newSession(t *testing.T, davBase, tasksBase string, client *http.Client, email string) *session {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := &session{db: db}
	err = db.Tx(ctx, func(tx *store.Tx) error {
		server := store.ServerConfig{Host: "mail.example.com", Port: 993, TLS: api.TLSModeTLS, Username: davtest.User}
		dav, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: email, Auth: api.AuthKindPassword,
			IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		s.dav = dav.ID
		for svc, home := range map[api.ServiceKind]string{api.ServiceKindContacts: davtest.ContactsHome,
			api.ServiceKindCalendar: davtest.CalendarsHome, api.ServiceKindTasks: davtest.CalendarsHome} {
			if err := tx.SetService(ctx, dav.ID, svc, true, davBase+home); err != nil {
				return err
			}
		}
		gmail := store.ServerConfig{Host: "imap.gmail.com", Port: 993, TLS: api.TLSModeTLS, Username: "ann.lee@gmail.example"}
		g, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindGmail, Email: "ann.lee@gmail.example",
			Auth: api.AuthKindOAuth2, IMAP: gmail, SMTP: gmail})
		if err != nil {
			return err
		}
		s.gmail = g.ID
		if err := tx.SetGrantedScopes(ctx, g.ID, "https://mail.google.com/ "+oauth.GoogleTasksScope); err != nil {
			return err
		}
		return tx.SetService(ctx, g.ID, api.ServiceKindTasks, true, tasksBase+"/")
	})
	if err != nil {
		t.Fatal(err)
	}
	sec := secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json"))
	if err := sec.Set(ctx, secrets.AccountPassword(s.dav), davtest.Password); err != nil {
		t.Fatal(err)
	}
	s.m = pimsync.New(db, sec, slog.New(slog.DiscardHandler), pimsync.Config{HTTP: client, Tokens: fakeTokens{},
		AllowHTTP: true, Batch: 2, Now: func() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) }})
	return s
}

// passes syncs both accounts twice: a first sync, then an incremental one.
func (s *session) passes(t *testing.T) {
	t.Helper()
	for range 2 {
		for _, id := range []int64{s.dav, s.gmail} {
			if err := s.m.Pass(t.Context(), id); err != nil {
				t.Fatalf("pass %d: %v", id, err)
			}
		}
	}
}

// shape describes the store without its words: what scrubbing keeps.
func (s *session) shape(t *testing.T) []string {
	t.Helper()
	ctx := t.Context()
	var out []string
	cols, err := s.db.Collections(ctx, store.CollectionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cols {
		out = append(out, fmt.Sprintf("collection %d %s read-only=%v", c.AccountID, c.Kind, c.ReadOnly))
	}
	people, err := s.db.People(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range people {
		out = append(out, fmt.Sprintf("person contacts=%d photo=%v email=%v org=%v", len(p.ContactIDs), p.HasPhoto,
			p.Email != "", p.Organization != ""))
	}
	events, err := s.db.Events(ctx, store.EventsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		full, err := s.db.Event(ctx, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("event %s–%s all-day=%v repeats=%v answer=%q attendees=%d organizer=%v",
			e.Start.UTC().Format(time.RFC3339), e.End.UTC().Format(time.RFC3339), e.AllDay, e.Recurrence != "",
			e.PartStat, len(full.Attendees), full.Organizer != ""))
	}
	tasks, err := s.db.Tasks(ctx, store.TaskFilter{Completed: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range tasks {
		out = append(out, fmt.Sprintf("task %d due=%q done=%v subtask=%v notes=%v", r.AccountID, r.Due, r.Completed,
			r.ParentID != 0, r.Notes != ""))
	}
	slices.Sort(out)
	return out
}

// pixel is a one-pixel PNG standing in for a contact's photo.
var pixel = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 'a', 'd', 'a', '-', 'p', 'h', 'o', 't', 'o'}

func ics(lines ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VCALENDAR\r\n")
}

// TestRecordSession records testdata/session.trace with -update: Ann's
// contacts, calendar, CalDAV tasks and Google Tasks.
func TestRecordSession(t *testing.T) {
	if !*update {
		t.Skip("run with -update to record the session again")
	}
	dav := davtest.New(t, davtest.Options{})
	book := dav.AddressBook("contacts", "Contacts")
	dav.Put(book+"ada@example.com.vcf", []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:ada-2026\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\n"+
		"ORG:Analytical Engines\r\nNOTE:Countess of Lovelace\r\nEMAIL;TYPE=HOME:ada@example.com\r\n"+
		"TEL;TYPE=CELL:+44 20 7946 0000\r\nPHOTO;ENCODING=b;TYPE=PNG:"+base64.StdEncoding.EncodeToString(pixel)+"\r\nEND:VCARD\r\n"))
	dav.Put(book+"grace.vcf", []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:grace-2026\r\nFN:Grace Hopper\r\nN:Hopper;Grace;;;\r\n"+
		"EMAIL:grace@navy.example\r\nADR;TYPE=WORK:;;1 Navy Way;Arlington;VA;22202;USA\r\nEND:VCARD\r\n"))
	work := dav.Calendar("work", "Work", "#3366cc", "VEVENT", "VTODO")
	dav.Put(work+"invite-"+account+".ics", ics("BEGIN:VEVENT", "UID:review-2026@example.com", "DTSTART:20261015T140000Z",
		"DTEND:20261015T153000Z", "SUMMARY:Design review", "LOCATION:Studio B", "DESCRIPTION:Bring the latest numbers",
		"ORGANIZER;CN=Maria Lopez:mailto:maria@northwind.example",
		"ATTENDEE;CN=Ann Lee;PARTSTAT=ACCEPTED:mailto:"+account, "ATTENDEE;CN=Ada Lovelace:mailto:ada@example.com", "END:VEVENT"))
	dav.Put(work+"standup-2026.ics", ics("BEGIN:VEVENT", "UID:standup-2026", "DTSTART;TZID=Europe/Berlin:20261012T091500",
		"DTEND;TZID=Europe/Berlin:20261012T093000", "RRULE:FREQ=DAILY;COUNT=5", "SUMMARY:Standup", "END:VEVENT"))
	dav.Put(work+"report.ics", ics("BEGIN:VTODO", "UID:report-2026", "SUMMARY:Send the Q3 report",
		"DESCRIPTION:Numbers from Maria", "DUE;VALUE=DATE:20261012", "END:VTODO"))
	gt := gtaskstest.New(t, gtaskstest.Options{})
	list := gt.AddList("Errands")
	milk := gt.AddTask(list, gtaskstest.Task{Title: "Buy oat milk", Notes: "Oat, not soy", Due: "2026-10-10T00:00:00.000Z"})
	gt.AddTask(list, gtaskstest.Task{Title: "Check the price", Parent: milk})

	var out bytes.Buffer
	insecure := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // the test servers' certificates
	client := &http.Client{Transport: &httprec.Transport{Base: insecure, W: &out}}
	s := newSession(t, dav.URL, gt.Base, client, account)
	s.passes(t)
	xs, err := httprec.Parse(&out)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(trace)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	note := fmt.Sprintf("Recorded by TestRecordSession: pimsync against davtest (%s) and gtaskstest (%s).", dav.URL, gt.Base)
	if err := httprec.Write(f, note, xs); err != nil {
		t.Fatal(err)
	}
	t.Logf("recorded %d exchanges; shape %q", len(xs), s.shape(t))
}

// replay runs the session against a trace and returns the store's shape.
// The DAV account's address is the trace's "# account" line, the fake
// davrec made of it, or else the recorded one.
func replay(t *testing.T, path string) []string {
	t.Helper()
	xs, err := httprec.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	rs := httprec.Serve(t, xs)
	s := newSession(t, rs.URL, rs.URL+"/tasks/v1", http.DefaultClient, accountOf(t, path))
	s.passes(t)
	return s.shape(t)
}

// accountOf reads a trace's "# account" comment; the recorded account
// without one.
func accountOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if a, ok := strings.CutPrefix(line, "# account "); ok {
			return a
		}
	}
	return account
}

func scrub(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "scrubbed.trace")
	var stdout bytes.Buffer
	err := run(append(append([]string{"-account", account, "-name", name, "-o", out}, args...), trace), &stdout)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(out)
	return string(data), err
}

func TestTheRecordingReplays(t *testing.T) {
	shape := replay(t, trace)
	if len(shape) < 10 {
		t.Fatalf("the recorded session's shape = %q", shape)
	}
	raw, _ := os.ReadFile(trace)
	for _, s := range sensitive {
		if !strings.Contains(string(raw), s) {
			t.Errorf("the recording lacks %q: the test checks nothing", s)
		}
	}
}

func TestScrubHidesThePerson(t *testing.T) {
	out, err := scrub(t, "-keyfile", keyfile, "-note", "Ann's session, scrubbed")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "# Ann's session, scrubbed\n# account ") {
		t.Errorf("the note and the account's fake are missing: %.80q", out)
	}
	fake, _, _ := strings.Cut(strings.SplitN(out, "\n", 3)[1], "\n")
	fake = strings.TrimPrefix(fake, "# account ")
	if fake == account || !strings.HasSuffix(fake, ".com") || !strings.Contains(fake, "@") || len(fake) != len(account) {
		t.Errorf("the account's fake = %q", fake)
	}
	for _, s := range sensitive {
		if strings.Contains(out, s) || strings.Contains(strings.ToLower(out), strings.ToLower(s)) {
			t.Errorf("%q survived scrubbing", s)
		}
	}
	if strings.Contains(out, base64.StdEncoding.EncodeToString(pixel)) {
		t.Error("the photo survived scrubbing")
	}
	xs, err := httprec.Parse(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := httprec.Load(trace)
	if len(xs) != len(raw) {
		t.Errorf("exchanges = %d, want %d", len(xs), len(raw))
	}
	for i := range xs {
		if i < len(raw) && (xs[i].Method != raw[i].Method || xs[i].Status != raw[i].Status) {
			t.Errorf("exchange %d = %s %d, want %s %d", i, xs[i].Method, xs[i].Status, raw[i].Method, raw[i].Status)
		}
	}
}

func TestScrubbedSessionReplaysTheSame(t *testing.T) {
	out, err := scrub(t, "-keyfile", keyfile)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "scrubbed.trace")
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	want, got := replay(t, trace), replay(t, path)
	if !slices.Equal(got, want) {
		t.Errorf("scrubbed shape =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestScrubIsStableWithAKey(t *testing.T) {
	a, err := scrub(t, "-keyfile", keyfile)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := scrub(t, "-keyfile", keyfile)
	c, _ := scrub(t)
	if a != b {
		t.Error("two scrubs with one key differ")
	}
	if a == c {
		t.Error("a scrub without the key made the same fakes")
	}
}

// writeTrace writes exchanges to a trace file in a temporary directory.
func writeTrace(t *testing.T, xs []httprec.Exchange) string {
	t.Helper()
	var b bytes.Buffer
	if err := httprec.Write(&b, "", xs); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "in.trace")
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestScrubEncodedAddresses: Google names a calendar home by the account's
// address, %40-encoded, in request URLs and in the hrefs it answers; both
// must become the same fake.
func TestScrubEncodedAddresses(t *testing.T) {
	home := "/caldav/v2/ann.lee%40example.com/events/"
	xs := []httprec.Exchange{
		{Method: "PROPFIND", URL: "https://apidata.googleusercontent.com" + home, ReqBody: "<propfind/>", Status: 207,
			RespBody: "<multistatus><response><href>" + home + "x.ics</href></response></multistatus>"},
		{Method: "GET", URL: "https://apidata.googleusercontent.com" + home + "x.ics", Status: 200,
			RespBody: "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"},
	}
	out := filepath.Join(t.TempDir(), "out.trace")
	if err := run([]string{"-account", account, "-keyfile", keyfile, "-o", out, writeTrace(t, xs)}, io.Discard); err != nil {
		t.Fatal(err)
	}
	got, err := httprec.Load(out)
	if err != nil || len(got) != 2 {
		t.Fatalf("scrubbed = %+v, %v", got, err)
	}
	raw, _ := os.ReadFile(out)
	if strings.Contains(string(raw), "ann.lee") {
		t.Fatalf("the address survived:\n%s", raw)
	}
	reqPath := strings.TrimPrefix(got[0].URL, "https://apidata.googleusercontent.com")
	if !strings.Contains(got[0].RespBody, "<href>"+reqPath+"x.ics</href>") ||
		got[1].URL != got[0].URL+"x.ics" || !strings.Contains(reqPath, "%40") {
		t.Errorf("the home is not scrubbed the same everywhere: %q, %q, %q", got[0].URL, got[0].RespBody, got[1].URL)
	}
}

func TestScrubRefusesWhatItCannotScrub(t *testing.T) {
	// A binary body that is not an image cannot be scrubbed: the account's
	// address inside it would survive.
	xs := []httprec.Exchange{{Method: "GET", URL: "https://dav.example/blob", Status: 200,
		RespHeader: http.Header{"Content-Type": {"application/octet-stream"}},
		RespBody:   base64.StdEncoding.EncodeToString(append([]byte{0xff, 0xfe}, account...)), RespBase64: true}}
	path := filepath.Join(t.TempDir(), "binary.trace")
	var b bytes.Buffer
	if err := httprec.Write(&b, "", xs); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.trace")
	err := run([]string{"-account", account, "-o", out, path}, io.Discard)
	if err == nil {
		t.Fatal("a trace with an unscrubbable body was written")
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("a refused trace left a file")
	}
	if err := run([]string{trace}, io.Discard); err == nil {
		t.Error("no -account was accepted")
	}
}

// TestScrubKeepsDates: dates stay, being what a sync orders by, and the
// check for remaining personal words allows them. Google's holiday events
// begin their UIDs with the date; a birthday is a date too. A number that
// is no date, like a phone number, is still replaced.
func TestScrubKeepsDates(t *testing.T) {
	xs := []httprec.Exchange{
		{Method: "GET", URL: "https://apidata.googleusercontent.com/caldav/v2/h/events/new-year.ics", Status: 200,
			RespBody: "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:20210101_60o30chp6so30c1g60o30dr4ck@google.com\r\n" +
				"DTSTART;VALUE=DATE:20210101\r\nSUMMARY:Lunar Festival\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"},
		{Method: "GET", URL: "https://www.googleapis.com/carddav/v1/principals/p/lists/default/c1.vcf", Status: 200,
			RespBody: "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:c1\r\nFN:Rowan Quill\r\nBDAY:1993-09-29\r\nTEL:55512345\r\nEND:VCARD\r\n"},
	}
	out := filepath.Join(t.TempDir(), "out.trace")
	if err := run([]string{"-account", account, "-keyfile", keyfile, "-o", out, writeTrace(t, xs)}, io.Discard); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, kept := range []string{`DTSTART;VALUE=DATE:20210101`, `BDAY:1993-09-29`} {
		if !strings.Contains(got, kept) {
			t.Errorf("%s was not kept:\n%s", kept, got)
		}
	}
	for _, gone := range []string{"60o30chp6so30c1g60o30dr4ck", "Lunar", "Rowan", "55512345"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s survived:\n%s", gone, got)
		}
	}
}

// TestScrubTaskListIDs: a Google task list's ID encodes a number of the
// account's. It becomes the same fake in the lists, the URLs that name
// it and the tasks' links.
func TestScrubTaskListIDs(t *testing.T) {
	const id = "MTM0NTAxNDk2NTkzMDAzNTk0ODc6MDow"
	list := "https://tasks.googleapis.com/tasks/v1/lists/" + id + "/tasks"
	xs := []httprec.Exchange{
		{Method: "GET", URL: "https://tasks.googleapis.com/tasks/v1/users/@me/lists?maxResults=100", Status: 200,
			RespBody: `{"kind":"tasks#taskLists","items":[{"kind":"tasks#taskList","id":"` + id + `","title":"My Tasks"}]}`},
		{Method: "GET", URL: list + "?maxResults=100", Status: 200,
			RespBody: `{"kind":"tasks#tasks","items":[{"kind":"tasks#task","id":"UHpLcndzRXJTdXBpUEV3bg","title":"Pay rent",` +
				`"selfLink":"https://www.googleapis.com/tasks/v1/lists/` + id + `/tasks/UHpLcndzRXJTdXBpUEV3bg"}]}`},
	}
	out := filepath.Join(t.TempDir(), "out.trace")
	if err := run([]string{"-account", account, "-keyfile", keyfile, "-o", out, writeTrace(t, xs)}, io.Discard); err != nil {
		t.Fatal(err)
	}
	got, err := httprec.Load(out)
	if err != nil || len(got) != 2 {
		t.Fatalf("scrubbed = %+v, %v", got, err)
	}
	raw, _ := os.ReadFile(out)
	if strings.Contains(string(raw), id) {
		t.Fatalf("the list ID survived:\n%s", raw)
	}
	var lists struct {
		Items []struct{ ID string } `json:"items"`
	}
	if err := json.Unmarshal([]byte(got[0].RespBody), &lists); err != nil || len(lists.Items) != 1 {
		t.Fatalf("lists = %s, %v", got[0].RespBody, err)
	}
	fake := lists.Items[0].ID
	if len(fake) != len(id) || !strings.Contains(got[1].URL, "/lists/"+fake+"/tasks") ||
		!strings.Contains(got[1].RespBody, "/lists/"+fake+"/tasks/") {
		t.Errorf("the list is not the same fake everywhere: %q, %q, %q", fake, got[1].URL, got[1].RespBody)
	}
}

// TestScrubKeepsStructure: a word that is personal in one place is still
// kept where it is syntax, an XML name or a property's, and time zone
// names stay with the dates. A contact's note says "address", which is
// part of CardDAV's address-data; an event's place names the zone's city.
func TestScrubKeepsStructure(t *testing.T) {
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:c1\r\nFN:Rowan Quill\r\nNOTE:Mailing address changed\r\n" +
		"item1.TEL;TYPE=CELL:(213) 395-8201\r\nEND:VCARD\r\n"
	event := "BEGIN:VCALENDAR\r\nX-WR-TIMEZONE:America/New_York\r\nBEGIN:VEVENT\r\nUID:e1\r\n" +
		"DTSTART;TZID=America/New_York:20261014T120000\r\nSUMMARY:Lunch\r\nLOCATION:York Harbour office\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	xs := []httprec.Exchange{
		{Method: "REPORT", URL: "https://www.googleapis.com/carddav/v1/principals/p/lists/default/",
			ReqBody: `<cr:addressbook-multiget xmlns:d="DAV:" xmlns:cr="urn:ietf:params:xml:ns:carddav">` +
				`<d:prop><cr:address-data/></d:prop><d:href>c1</d:href></cr:addressbook-multiget>`,
			Status: 207,
			RespBody: `<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav"><d:response><d:href>c1</d:href>` +
				`<d:propstat><d:prop><card:address-data>` + card + `</card:address-data></d:prop></d:propstat></d:response></d:multistatus>`},
		{Method: "GET", URL: "https://apidata.googleusercontent.com/caldav/v2/h/events/e1.ics", Status: 200, RespBody: event},
	}
	out := filepath.Join(t.TempDir(), "out.trace")
	if err := run([]string{"-account", account, "-keyfile", keyfile, "-o", out, writeTrace(t, xs)}, io.Discard); err != nil {
		t.Fatal(err)
	}
	got, err := httprec.Load(out)
	if err != nil || len(got) != 2 {
		t.Fatalf("scrubbed = %+v, %v", got, err)
	}
	if got[0].ReqBody != xs[0].ReqBody {
		t.Errorf("the request changed:\n%s", got[0].ReqBody)
	}
	for _, kept := range []string{"<card:address-data>", "</card:address-data>", "item1.TEL;TYPE=CELL:"} {
		if !strings.Contains(got[0].RespBody, kept) {
			t.Errorf("%s was not kept:\n%s", kept, got[0].RespBody)
		}
	}
	for _, kept := range []string{"X-WR-TIMEZONE:America/New_York", "DTSTART;TZID=America/New_York:20261014T120000"} {
		if !strings.Contains(got[1].RespBody, kept) {
			t.Errorf("%s was not kept:\n%s", kept, got[1].RespBody)
		}
	}
	raw, _ := os.ReadFile(out)
	for _, gone := range []string{"Mailing", "Rowan", "Harbour", "York Harbour", "Lunch"} {
		if strings.Contains(string(raw), gone) {
			t.Errorf("%s survived:\n%s", gone, raw)
		}
	}
}
