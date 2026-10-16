package pimsync_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/graphtest"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// graphTokens gives mail's token and Graph's.
type graphTokens struct{ graph string }

func (g graphTokens) AccessToken(context.Context, int64) (string, error) { return "mail-token", nil }
func (g graphTokens) AccessTokenFor(_ context.Context, _ int64, audience string) (string, error) {
	if audience == oauth.AudienceGraph {
		return g.graph, nil
	}
	return "mail-token", nil
}
func (graphTokens) Invalidate(int64) {}

// graphEnv is a Microsoft account with contacts and calendars on, over
// graphtest, on a clock stopped at the probe's day.
type graphEnv struct {
	t    *testing.T
	db   *store.DB
	srv  *graphtest.Server
	m    *pimsync.Manager
	acct store.Account
}

var graphNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func newGraphEnv(t *testing.T, scopes string) *graphEnv {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := &graphEnv{t: t, db: db, srv: graphtest.Start(t, "graph-token")}
	server := store.ServerConfig{Host: "outlook.office365.com", Port: 993, TLS: api.TLSModeTLS, Username: "cy@hotmail.com"}
	err = db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		e.acct, err = tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindMicrosoft, Email: "cy@hotmail.com",
			Auth: api.AuthKindOAuth2, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		if err := tx.SetGrantedScopes(ctx, e.acct.ID, scopes); err != nil {
			return err
		}
		for _, s := range []api.ServiceKind{api.ServiceKindContacts, api.ServiceKindCalendar} {
			if err := tx.SetService(ctx, e.acct.ID, s, true, "https://graph.microsoft.com/v1.0/"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	e.m = pimsync.New(db, secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json")),
		slog.New(slog.NewTextHandler(io.Discard, nil)), pimsync.Config{
			GraphURL: e.srv.URL, Tokens: graphTokens{graph: "graph-token"}, Now: func() time.Time { return graphNow },
			Local: time.UTC,
		})
	return e
}

const allGraphScopes = oauth.MicrosoftIMAPScope + " " + oauth.MicrosoftSMTPScope + " offline_access " +
	oauth.MicrosoftContactsScope + " " + oauth.MicrosoftCalendarScope

func (e *graphEnv) pass() {
	e.t.Helper()
	if err := e.m.Pass(e.t.Context(), e.acct.ID); err != nil {
		e.t.Fatal(err)
	}
}

func (e *graphEnv) occurrences() []string {
	e.t.Helper()
	occ, err := e.db.Occurrences(e.t.Context(), store.OccurrenceFilter{
		From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		FromDate: "2026-10-01", ToDate: "2026-12-01"})
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for _, o := range occ {
		out = append(out, o.Instance.Start.UTC().Format("01-02T15:04")+" "+o.Event.Summary)
	}
	slices.Sort(out)
	return out
}

func (e *graphEnv) people() []string {
	e.t.Helper()
	rows, err := e.db.People(e.t.Context(), "")
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		photo := ""
		if r.HasPhoto {
			photo = " (photo)"
		}
		out = append(out, r.DisplayName+" "+r.Email+photo)
	}
	return out
}

const graphSingle = `{"id": "AAMk-single", "iCalUId": "single-uid", "type": "singleInstance", "subject": "Review",
	"start": {"dateTime": "2026-10-27T14:00:00.0000000", "timeZone": "UTC"}, "end": {"dateTime": "2026-10-27T15:00:00.0000000", "timeZone": "UTC"},
	"originalStartTimeZone": "America/New_York", "originalEndTimeZone": "America/New_York", "isReminderOn": true, "reminderMinutesBeforeStart": 10}`

func graphSeries(subject string) string {
	return `{"id": "AAMk-master", "iCalUId": "series-uid", "type": "seriesMaster", "subject": "` + subject + `",
	"start": {"dateTime": "2026-10-26T13:30:00.0000000", "timeZone": "UTC"}, "end": {"dateTime": "2026-10-26T13:45:00.0000000", "timeZone": "UTC"},
	"originalStartTimeZone": "Eastern Standard Time", "originalEndTimeZone": "Eastern Standard Time",
	"recurrence": {"pattern": {"type": "daily", "interval": 1}, "range": {"type": "numbered", "startDate": "2026-10-26", "recurrenceTimeZone": "Eastern Standard Time", "numberOfOccurrences": 5}},
	"cancelledOccurrences": ["OID.AAMk-master.2026-10-28"],
	"exceptionOccurrences": [{"id": "AAMk-exception", "iCalUId": "series-uid", "type": "exception", "seriesMasterId": "AAMk-master",
		"occurrenceId": "OID.AAMk-master.2026-10-27", "subject": "` + subject + `",
		"start": {"dateTime": "2026-10-27T14:00:00.0000000", "timeZone": "UTC"}, "end": {"dateTime": "2026-10-27T14:15:00.0000000", "timeZone": "UTC"},
		"originalStartTimeZone": "Eastern Standard Time", "originalEndTimeZone": "Eastern Standard Time"}]}`
}

const graphContact = `{"id": "AAMk-contact", "displayName": "Dee Example", "givenName": "Dee", "surname": "Example",
	"emailAddresses": [{"name": "Dee Example", "address": "dee@example.com"}], "mobilePhone": "+1 555 0100"}`

func TestGraphSyncsCalendarsAndContacts(t *testing.T) {
	e := newGraphEnv(t, allGraphScopes)
	ctx := t.Context()
	e.srv.AddCalendar("cal-holidays", "US Holidays", false)
	e.srv.PutEvent("cal-1", graphSingle)
	e.srv.PutEvent("cal-1", graphSeries("Standup"))
	e.srv.PutContact("contacts", graphContact, []byte("\xff\xd8\xff\xe0jpeg"))
	e.pass()

	cols, err := e.db.Collections(ctx, store.CollectionFilter{AccountID: e.acct.ID})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range cols {
		names = append(names, string(c.Kind)+" "+c.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"addressbook Contacts", "calendar Calendar", "calendar US Holidays"}) {
		t.Errorf("collections = %v", names)
	}
	want := []string{"10-26T13:30 Standup", "10-27T14:00 Review", "10-27T14:00 Standup", "10-29T13:30 Standup", "10-30T13:30 Standup"}
	if got := e.occurrences(); !slices.Equal(got, want) {
		t.Errorf("occurrences = %v, want %v", got, want)
	}
	if got := e.people(); !slices.Equal(got, []string{"Dee Example dee@example.com (photo)"}) {
		t.Errorf("people = %v", got)
	}
	// Graph's JSON is kept beside the derived source.
	obj, err := e.db.ObjectByHref(ctx, cols[1].ID, "AAMk-master")
	if err != nil {
		for _, c := range cols {
			if o, err2 := e.db.ObjectByHref(ctx, c.ID, "AAMk-master"); err2 == nil {
				obj, err = o, nil
			}
		}
	}
	if err != nil || !strings.Contains(string(obj.Remote), `"exceptionOccurrences"`) || !strings.Contains(string(obj.Raw), "RRULE:FREQ=DAILY;COUNT=5") {
		t.Errorf("series object = %+v, %v", obj, err)
	}

	// Another client renames the series and deletes the review and the
	// contact: the next pass reads deltas, and the series once.
	e.srv.PutEvent("cal-1", graphSeries("Daily standup"))
	e.srv.DeleteEvent("cal-1", "AAMk-single")
	e.srv.DeleteContact("contacts", "AAMk-contact")
	before := e.srv.Requests["GET me/events/AAMk-master"]
	e.pass()
	want = []string{"10-26T13:30 Daily standup", "10-27T14:00 Daily standup", "10-29T13:30 Daily standup", "10-30T13:30 Daily standup"}
	if got := e.occurrences(); !slices.Equal(got, want) {
		t.Errorf("after the changes: occurrences = %v, want %v", got, want)
	}
	if got := e.people(); len(got) != 0 {
		t.Errorf("after the delete: people = %v", got)
	}
	if n := e.srv.Requests["GET me/events/AAMk-master"] - before; n != 1 {
		t.Errorf("the series was read %d times in the pass, want once", n)
	}
	// A pass with nothing changed reads no series.
	before = e.srv.Requests["GET me/events/AAMk-master"]
	e.pass()
	if n := e.srv.Requests["GET me/events/AAMk-master"] - before; n != 0 {
		t.Errorf("a quiet pass read the series %d times", n)
	}
}

func TestGraphRetriesThrottledRequests(t *testing.T) {
	e := newGraphEnv(t, allGraphScopes)
	e.srv.PutEvent("cal-1", graphSingle)
	e.srv.Throttle(2)
	e.pass()
	if got := e.occurrences(); !slices.Equal(got, []string{"10-27T14:00 Review"}) {
		t.Errorf("occurrences = %v", got)
	}
}

func TestGraphResyncsAnExpiredDelta(t *testing.T) {
	e := newGraphEnv(t, allGraphScopes)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", graphSingle)
	e.pass()
	// The stored token names a round Graph no longer knows.
	cols, _ := e.db.Collections(ctx, store.CollectionFilter{AccountID: e.acct.ID, Kind: api.CollectionKindCalendar})
	tok := strings.Replace(cols[0].SyncToken, "%24deltatoken=", "%24deltatoken=9", 1)
	if err := e.db.Tx(ctx, func(tx *store.Tx) error { return tx.SetCollectionSync(ctx, cols[0].ID, tok, "") }); err != nil {
		t.Fatal(err)
	}
	e.srv.PutEvent("cal-1", graphSeries("Standup"))
	e.pass()
	if got := e.occurrences(); len(got) != 5 {
		t.Errorf("after the resync: occurrences = %v", got)
	}
}

func TestGraphWaitsForItsScopes(t *testing.T) {
	e := newGraphEnv(t, oauth.MicrosoftIMAPScope+" "+oauth.MicrosoftSMTPScope+" offline_access")
	e.srv.PutEvent("cal-1", graphSingle)
	err := e.m.Pass(t.Context(), e.acct.ID)
	if err == nil || !strings.Contains(err.Error(), "sign in again") {
		t.Errorf("a pass without Graph's scopes = %v, want sign in again", err)
	}
	if n := e.srv.Requests["GET me/calendars"]; n != 0 {
		t.Errorf("Graph was asked %d times without a grant", n)
	}
}

func TestGraphVerify(t *testing.T) {
	e := newGraphEnv(t, allGraphScopes)
	ctx := t.Context()
	e.srv.PutEvent("cal-1", graphSingle)
	e.srv.PutEvent("cal-1", graphSeries("Standup"))
	e.srv.PutContact("contacts", graphContact, nil)
	e.pass()
	checks, err := e.m.Verify(ctx, e.acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 2 {
		t.Fatalf("checks = %+v", checks)
	}
	for _, c := range checks {
		if !pimsync.Clean(c) {
			t.Errorf("after a pass, %q differs: %+v", c.Name, c)
		}
	}
	// Another client adds an event and deletes the contact; no pass yet.
	e.srv.PutEvent("cal-1", strings.Replace(graphSingle, "AAMk-single", "AAMk-new", 1))
	e.srv.DeleteContact("contacts", "AAMk-contact")
	checks, err = e.m.Verify(ctx, e.acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]api.CollectionCheck{}
	for _, c := range checks {
		byName[c.Name] = c
	}
	if c := byName["Calendar"]; !slices.Equal(c.MissingLocally, []string{"AAMk-new"}) || len(c.MissingOnServer) != 0 {
		t.Errorf("calendar check = %+v", c)
	}
	if c := byName["Contacts"]; !slices.Equal(c.MissingOnServer, []string{"AAMk-contact"}) {
		t.Errorf("contacts check = %+v", c)
	}
}
