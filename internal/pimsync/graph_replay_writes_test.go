package pimsync_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davsession"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/httprec"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// TestMicrosoftWriteReplay replays the Outlook.com write session
// (internal/davsession.Microsoft), recorded on 2026-10-16 against the
// user's test account with tools/davsession: a first sync over Graph,
// then a single event made, moved, deleted and restored; a daily series
// with an occurrence moved and the move undone, an occurrence deleted,
// every occurrence changed, the series moved later (Outlook dropped its
// cancellation, which was written again) and split; an all-day event; a
// contact changed, given a photo, deleted and restored; everything deleted
// again, and a last pass that changes nothing. Outlook took every write,
// and the replay sends each request the recording did, in its order.
func TestMicrosoftWriteReplay(t *testing.T) {
	ctx := t.Context()
	trace := filepath.Join("testdata", "replay", "graph-writes.trace")
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
	recorded := traceRecorded(t, trace)
	db.Now = func() time.Time { return recorded }
	var events []api.EventEnvelope
	db.OnCommit = func(evs []api.EventEnvelope) { events = append(events, evs...) }
	email := traceAccount(t, trace)
	server := store.ServerConfig{Host: "outlook.office365.com", Port: 993, TLS: api.TLSModeTLS, Username: email}
	var acct store.Account
	err = db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		acct, err = tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindMicrosoft, Email: email, Auth: api.AuthKindOAuth2,
			IMAP: server, SMTP: server})
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
	newYork, err := time.LoadLocation(davsession.MicrosoftZone)
	if err != nil {
		t.Fatal(err)
	}
	m := pimsync.New(db, secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json")), slog.New(slog.NewTextHandler(io.Discard, nil)),
		pimsync.Config{GraphURL: srv.Rewrite(providers.GraphURL), Tokens: graphTokens{graph: "token"}, AllowHTTP: true,
			Now: func() time.Time { return recorded }, Local: newYork})
	eng := engine.New(engine.Deps{DB: db, PIM: m})
	var steps, refused []string
	var last []api.EventEnvelope
	err = davsession.Microsoft(ctx, davsession.Session{
		Calendar: eng.Calendar(),
		People:   eng.People(),
		Pass: func(ctx context.Context) error {
			events = nil
			err := m.Pass(ctx, acct.ID)
			for _, ev := range events {
				if strings.HasSuffix(ev.Event, ".refused") {
					refused = append(refused, steps[len(steps)-1]+": "+string(ev.Data))
				}
			}
			last = events
			return err
		},
		Step: func(name string) { steps = append(steps, name) },
	})
	if err != nil {
		t.Fatalf("after %q: %v", steps, err)
	}
	if n := srv.Unused(); n != 0 {
		t.Errorf("%d recorded exchanges were never asked for", n)
	}
	if len(refused) != 0 {
		t.Errorf("refused: %q", refused)
	}
	if len(last) != 0 {
		t.Errorf("the last pass changed %d things", len(last))
	}
	rows, err := db.Events(ctx, store.EventsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	// October 26–30 had no event before the session, holidays included.
	for _, r := range rows {
		if !r.Start.Before(time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)) && r.Start.Before(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("the session left %q at %s", r.Summary, r.Start)
		}
	}
}
