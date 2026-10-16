package pimsync_test

import (
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/httprec"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// TestGraphReplayFirstSync replays the first three passes of the user's
// Outlook.com account over Microsoft Graph, scrubbed (plan 0010, Phase
// 3a): four calendars, US Holidays with its yearly series and single
// days, Birthdays as yearly series, the one contact without a photo, and
// two passes that find nothing changed (deltas only, no series read).
// Every recorded exchange is asked for, in the recorded order.
func TestGraphReplayFirstSync(t *testing.T) {
	ctx := t.Context()
	trace := filepath.Join("testdata", "replay", "graph-first-sync.trace")
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
	email := traceAccount(t, trace)
	server := store.ServerConfig{Host: "outlook.office365.com", Port: 993, TLS: api.TLSModeTLS, Username: email}
	var acct store.Account
	err = db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		acct, err = tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindMicrosoft, Email: email, Auth: api.AuthKindOAuth2,
			IMAP: server, SMTP: server, ReadOnly: true})
		if err != nil {
			return err
		}
		if err := tx.SetGrantedScopes(ctx, acct.ID, allGraphScopes); err != nil {
			return err
		}
		for _, s := range []api.ServiceKind{api.ServiceKindContacts, api.ServiceKindCalendar} {
			if err := tx.SetService(ctx, acct.ID, s, true, providers.GraphURL); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	recorded := traceRecorded(t, trace)
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	m := pimsync.New(db, secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json")), slog.New(slog.NewTextHandler(io.Discard, nil)),
		pimsync.Config{GraphURL: srv.Rewrite(providers.GraphURL), Tokens: graphTokens{graph: "token"}, AllowHTTP: true,
			Now: func() time.Time { return recorded }, Local: newYork})
	for pass := range 3 {
		if err := m.Pass(ctx, acct.ID); err != nil {
			t.Fatalf("pass %d: %v", pass+1, err)
		}

	}
	if n := srv.Unused(); n != 0 {
		t.Errorf("%d recorded exchanges were never asked for", n)
	}

	cols, err := db.Collections(ctx, store.CollectionFilter{AccountID: acct.ID})
	if err != nil {
		t.Fatal(err)
	}
	type counts struct{ objects, series, broken int }
	got := map[api.CollectionKind][]counts{}
	var readOnly []string
	for _, c := range cols {
		var k counts
		if err := db.Tx(ctx, func(tx *store.Tx) error {
			return tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(instr(raw, 'RRULE:') > 0), 0), COALESCE(SUM(parse_error <> ''), 0)
				FROM objects WHERE collection_id = ?`, c.ID).Scan(&k.objects, &k.series, &k.broken)
		}); err != nil {
			t.Fatal(err)
		}
		got[c.Kind] = append(got[c.Kind], k)
		if c.ReadOnly {
			readOnly = append(readOnly, string(c.Kind)+" "+c.Name)
		}
	}
	// Outlook's calendar and the family calendar take writes, and the
	// address book; Birthdays and US Holidays do not (canEdit).
	slices.Sort(readOnly)
	if !slices.Equal(readOnly, []string{"calendar Birthday calendar", "calendar US Xzhbbqya"}) {
		t.Errorf("read-only = %v", readOnly)
	}
	// Calendar (2017's events are outside the window), Birthdays, US
	// Holidays, the family calendar; one address book.
	wantCalendars := []counts{{0, 0, 0}, {2, 2, 0}, {32, 13, 0}, {0, 0, 0}}
	if len(got[api.CollectionKindCalendar]) != len(wantCalendars) || len(got[api.CollectionKindAddressbook]) != 1 {
		t.Fatalf("collections = %+v", got)
	}
	for i, want := range wantCalendars {
		if got[api.CollectionKindCalendar][i] != want {
			t.Errorf("calendar %d: %+v, want %+v", i+1, got[api.CollectionKindCalendar][i], want)
		}
	}
	if got[api.CollectionKindAddressbook][0] != (counts{1, 0, 0}) {
		t.Errorf("contacts: %+v", got[api.CollectionKindAddressbook][0])
	}
	// Graph's calendarView held 29 occurrences from New Year's Eve 2025
	// to January 2027; the store holds them all.
	occ, err := db.Occurrences(ctx, store.OccurrenceFilter{
		From: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), To: time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC),
		FromDate: "2025-12-31", ToDate: "2027-01-31"})
	if err != nil {
		t.Fatal(err)
	}
	if len(occ) != 31 { // and New Year's Eve 2026
		t.Errorf("occurrences = %d, want 31", len(occ))
	}
}
