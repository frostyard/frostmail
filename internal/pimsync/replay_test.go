package pimsync_test

import (
	"bufio"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/httprec"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// replayEnv replays a recorded session (docs/design/testing.md, DAV and
// Tasks recordings) to a new account with the recording's address, its
// services turned on at the provider's URLs and not yet discovered, as
// adding an account leaves them.
func replayEnv(t *testing.T, trace string, kind api.AccountKind, scopes string) (*env, *httprec.Server) {
	t.Helper()
	ctx := t.Context()
	xs, err := httprec.Load(trace)
	if err != nil {
		t.Fatal(err)
	}
	srv := httprec.Serve(t, xs)
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := &env{t: t, db: db}
	db.OnCommit = func(evs []api.EventEnvelope) { e.events = append(e.events, evs...) }
	email := traceAccount(t, trace)
	dav := providers.ForKind(kind).DAV
	server := store.ServerConfig{Host: "imap.example", Port: 993, TLS: api.TLSModeTLS, Username: email}
	err = db.Tx(ctx, func(tx *store.Tx) error {
		e.acct, err = tx.InsertAccount(ctx, store.Account{Kind: kind, Email: email, Auth: api.AuthKindOAuth2, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		if err := tx.SetGrantedScopes(ctx, e.acct.ID, scopes); err != nil {
			return err
		}
		starts := map[api.ServiceKind]string{api.ServiceKindContacts: dav.Contacts, api.ServiceKindCalendar: dav.Calendar}
		if dav.Tasks == "google" {
			starts[api.ServiceKindTasks] = providers.GoogleTasksURL
		}
		for s, u := range starts {
			if err := tx.SetService(ctx, e.acct.ID, s, true, srv.Rewrite(u)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	recorded := time.Date(2026, 10, 9, 17, 37, 0, 0, time.UTC)
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	e.m = pimsync.New(db, secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json")), slog.New(slog.NewTextHandler(io.Discard, nil)),
		pimsync.Config{Tokens: &tokens{token: "token"}, AllowHTTP: true, Now: func() time.Time { return recorded }, Local: newYork})
	return e, srv
}

// traceAccount is the address a scrubbed trace's "# account" line names.
func traceAccount(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	lines := bufio.NewScanner(f)
	lines.Buffer(nil, 1<<24)
	for lines.Scan() {
		if email, ok := strings.CutPrefix(lines.Text(), "# account "); ok {
			return email
		}
	}
	t.Fatalf("%s names no account", path)
	return ""
}

func replayPass(e *env) {
	e.t.Helper()
	e.events = nil
	if err := e.m.Pass(e.t.Context(), e.acct.ID); err != nil {
		e.t.Fatalf("Pass: %v", err)
	}
}

// TestGoogleReplay replays a new Gmail account's first sync of its
// contacts, calendars and tasks, recorded on 2026-10-09, then a pass with
// nothing changed. Google refuses a CardDAV sync-collection without a
// token (400), so every pass lists the address book; its US holidays
// calendar is read-only, cut short (507) on its first sync and continued
// from the token, and fetched 100 objects at a time.
func TestGoogleReplay(t *testing.T) {
	e, _ := replayEnv(t, "testdata/replay/google.trace", api.AccountKindGmail,
		strings.Join([]string{"https://mail.google.com/", oauth.GoogleContactsScope, oauth.GoogleCalendarScope, oauth.GoogleTasksScope}, " "))
	ctx := t.Context()
	replayPass(e)

	books := e.collections(api.CollectionKindAddressbook)
	if len(books) != 1 || books[0].ReadOnly || !books[0].IsDefault {
		t.Fatalf("address books = %+v", books)
	}
	etags, err := e.db.ObjectETags(ctx, books[0].ID)
	if err != nil || len(etags) != 1 {
		t.Fatalf("contacts = %v, %v", etags, err)
	}
	for href := range etags {
		o, _ := e.db.ObjectByHref(ctx, books[0].ID, href)
		c, err := e.db.Contact(ctx, o.ID)
		if err != nil || c.GivenName == "" || c.FamilyName == "" || c.Organization == "" || len(c.Emails) != 1 {
			t.Errorf("the contact = %+v, %v", c, err)
		}
	}

	var own, holidays store.Collection
	for _, c := range e.collections(api.CollectionKindCalendar) {
		if c.ReadOnly {
			holidays = c
		} else {
			own = c
		}
	}
	for _, c := range []struct {
		col  store.Collection
		want int
	}{{own, 6}, {holidays, 317}} {
		if etags, err := e.db.ObjectETags(ctx, c.col.ID); err != nil || len(etags) != c.want {
			t.Errorf("calendar %q has %d events, want %d (%v)", c.col.Href, len(etags), c.want, err)
		}
	}
	if !own.IsDefault || !strings.HasSuffix(holidays.Href, "%40virtual/events/") {
		t.Errorf("calendars: own %+v, holidays %+v", own, holidays)
	}
	// Start times only: the summaries are davrec's fakes. Holidays fall on
	// 09-07, 10-12 and 10-31; the account's own events are a birthday Google
	// made from the contact (09-29, yearly), a 10:30 New York meeting, an
	// all-day day and an all-day span, Wednesdays at noon in New York, and
	// 14:00 in Los Angeles.
	starts := func(from, to string) []string {
		var out []string
		for _, o := range e.occurrences(from, to) {
			out = append(out, strings.Fields(o)[0])
		}
		return out
	}
	if got := starts("2026-09-01", "2026-11-01"); !slices.Equal(got, []string{"09-07", "09-29",
		"10-12", "10-12T14:30", "10-13", "10-14T16:00", "10-15", "10-19T21:00", "10-21T16:00", "10-28T16:00", "10-31"}) {
		t.Errorf("occurrences = %q", got)
	}

	lists := e.collections(api.CollectionKindTasklist)
	if len(lists) != 1 || lists[0].Name != "My Tasks" || !lists[0].IsDefault {
		t.Fatalf("task lists = %+v", lists)
	}
	rows, err := e.db.Tasks(ctx, store.TaskFilter{Completed: true})
	if err != nil || len(rows) != 2 || rows[0].Position >= rows[1].Position || rows[0].Completed || rows[1].Completed {
		t.Errorf("tasks = %+v, %v", rows, err)
	}

	// The second pass asks what the recording did and changes nothing, though
	// Tasks answers updatedMin with the task updated at that very time.
	replayPass(e)
	if len(e.events) != 0 {
		t.Errorf("the second pass changed %v", e.eventNames())
	}
}
