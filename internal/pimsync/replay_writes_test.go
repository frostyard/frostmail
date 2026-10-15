package pimsync_test

import (
	"context"
	"errors"
	"net/url"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davsession"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/httprec"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/store"
)

// createdNames are the names of the objects a trace's session created, in
// order: the last element of each PUT's path that only creates
// (If-None-Match: *), without its extension. The replay's engine names
// its new objects with them, as the recording's did.
func createdNames(t *testing.T, trace string) []string {
	t.Helper()
	xs, err := httprec.Load(trace)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, x := range xs {
		if x.Method != "PUT" || !slices.Contains(x.ReqHeader["If-None-Match"], "*") {
			continue
		}
		u, err := url.Parse(x.URL)
		if err != nil {
			t.Fatal(err)
		}
		base := path.Base(u.Path)
		out = append(out, strings.TrimSuffix(base, path.Ext(base)))
	}
	return out
}

// TestGoogleWriteReplay replays the Gmail write session
// (internal/davsession.Google), recorded on 2026-10-15 against the
// throwaway account with tools/davsession: a first sync, then a single
// event made, moved, deleted and restored; a series changed in each
// scope, This Event restored at once; an all-day event; a contact
// changed, given a photo, deleted and restored; everything deleted again,
// and a last pass that changes nothing. Google answered every write, and
// the replay sends each request the recording did, in its order.
func TestGoogleWriteReplay(t *testing.T) {
	const trace = "testdata/replay/google-writes.trace"
	e, _ := replayEnv(t, trace, api.AccountKindGmail,
		strings.Join([]string{"https://mail.google.com/", oauth.GoogleContactsScope, oauth.GoogleCalendarScope, oauth.GoogleTasksScope}, " "))
	ctx := t.Context()
	recorded := traceRecorded(t, trace)
	e.db.Now = func() time.Time { return recorded }
	names := createdNames(t, trace)
	made := 0
	eng := engine.New(engine.Deps{DB: e.db, PIM: e.m, NewUID: func() (string, error) {
		if made == len(names) {
			return "", errors.New("the session makes more objects than the recording")
		}
		made++
		return names[made-1], nil
	}})
	var steps, refused []string
	err := davsession.Google(ctx, davsession.Session{
		Calendar: eng.Calendar(),
		People:   eng.People(),
		Pass: func(ctx context.Context) error {
			e.events = nil
			err := e.m.Pass(ctx, e.acct.ID)
			for _, name := range e.eventNames() {
				if strings.HasSuffix(name, ".refused") {
					refused = append(refused, steps[len(steps)-1]+": "+name)
				}
			}
			return err
		},
		Step: func(name string) { steps = append(steps, name) },
	})
	if err != nil {
		t.Fatalf("after %q: %v", steps, err)
	}
	if made != len(names) || len(names) != 7 {
		t.Errorf("the session made %d objects of the %d recorded", made, len(names))
	}
	if len(e.events) != 0 {
		t.Errorf("the last pass changed %v", e.eventNames())
	}
	if len(refused) != 0 {
		t.Errorf("refused: %q", refused)
	}
	rows, err := e.db.Events(ctx, store.EventsFilter{})
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
