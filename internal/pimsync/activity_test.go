package pimsync_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/secrets"
)

// recording gives e a Manager that records activity into the returned hub.
func (e *env) recording() *activity.Hub {
	e.t.Helper()
	sec := secrets.NewFile(filepath.Join(e.t.TempDir(), "secrets.json"))
	if err := sec.Set(e.t.Context(), secrets.AccountPassword(e.acct.ID), davtest.Password); err != nil {
		e.t.Fatal(err)
	}
	hub := activity.New(nil)
	e.m = pimsync.New(e.db, sec, slog.New(slog.DiscardHandler), pimsync.Config{HTTP: e.dav.Client, Batch: 2, Activity: hub})
	return hub
}

// recordingTasks gives e a Manager with tokens that records activity.
func (e *tasksEnv) recordingTasks(tokens pimsync.TokenSource) *activity.Hub {
	e.t.Helper()
	hub := activity.New(nil)
	e.m = pimsync.New(e.db, secrets.NewFile(filepath.Join(e.t.TempDir(), "secrets.json")),
		slog.New(slog.DiscardHandler), pimsync.Config{HTTP: e.srv.Client, Tokens: tokens, Activity: hub})
	return hub
}

// wantRefused checks that hub holds one refusal, by srv with code.
func wantRefused(t *testing.T, hub *activity.Hub, srv api.ActivityServer, code int64) {
	t.Helper()
	var got []api.ActivityEntry
	for _, e := range hub.Recent(activity.MaxRecent) {
		if e.Kind == api.ActivityKindPimrefused {
			got = append(got, e)
		}
	}
	if len(got) != 1 {
		t.Fatalf("refused entries = %+v, want one", got)
	}
	e := got[0]
	if e.Level != api.ActivityLevelWarn || *e.Service != srv || *e.ErrorClass != api.ErrorClassRefused || *e.Code != code {
		t.Errorf("refused entry = %+v, want a warning, %s, refused, %d", e, srv, code)
	}
}

func failOn(method string, status int) func(*http.Request) int {
	return func(r *http.Request) int {
		if r.Method == method {
			return status
		}
		return 0
	}
}

func TestActivityOfAPass(t *testing.T) {
	e := newContactEnv(t)
	hub := e.recording()
	e.dav.Put(e.book+"bob.vcf", []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:bob\r\nFN:Bob\r\nEND:VCARD\r\n"))
	e.pass()
	got := hub.Recent(1)
	if len(got) != 1 || got[0].Kind != api.ActivityKindPimsynced || *got[0].Service != api.ActivityServerCarddav ||
		*got[0].Changed != 1 || got[0].Level != api.ActivityLevelInfo {
		t.Fatalf("a pass with news = %+v", got)
	}
	e.pass()
	got = hub.Recent(1)
	if len(got) != 1 || got[0].Kind != api.ActivityKindPimsynced || *got[0].Changed != 0 || got[0].Level != api.ActivityLevelDebug {
		t.Fatalf("a quiet pass = %+v", got)
	}
}

func TestActivityOfAFailedPass(t *testing.T) {
	e := newTasksEnv(t, allScopes())
	hub := e.recordingTasks(e.tokens)
	e.srv.AddList("Tasks")
	e.srv.FailWith(failOn(http.MethodGet, http.StatusInternalServerError))
	_ = e.pass()
	got := hub.Recent(1)
	if len(got) != 1 || got[0].Kind != api.ActivityKindFailed || *got[0].Service != api.ActivityServerGtasks || *got[0].Code != 500 {
		t.Fatalf("a failed pass = %+v", got)
	}
}

// reauthTokens refuses every token: the provider refused the grant.
type reauthTokens struct{}

func (reauthTokens) AccessToken(context.Context, int64) (string, error) {
	return "", fmt.Errorf("%w: revoked", oauth.ErrReauth)
}
func (reauthTokens) Invalidate(int64) {}

// A pass that needs a new sign-in says so, whichever way it learned.
func TestActivityOfAPassThatNeedsASignIn(t *testing.T) {
	for _, c := range []struct {
		name   string
		scopes string
		tokens pimsync.TokenSource
	}{
		{"the grant lacks the scope", "https://mail.google.com/", &tokens{token: "t"}},
		{"the provider refused the grant", allScopes(), reauthTokens{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newTasksEnv(t, c.scopes)
			hub := e.recordingTasks(c.tokens)
			_ = e.pass()
			got := hub.Recent(1)
			if len(got) != 1 || got[0].Kind != api.ActivityKindSigninneeded || *got[0].Service != api.ActivityServerGtasks ||
				*got[0].ErrorClass != api.ErrorClassAuth {
				t.Fatalf("entries = %+v, want one sign-in entry", got)
			}
		})
	}
}

func TestActivityOfARefusedContactEdit(t *testing.T) {
	e := newContactEnv(t)
	hub := e.recording()
	if _, err := e.people.Update(t.Context(), &api.PeopleUpdateParams{ID: e.contact("Ann Smith"), Fields: api.ContactFields{Title: ptr2("COO")}}); err != nil {
		t.Fatal(err)
	}
	e.dav.FailWith(failOn(http.MethodPut, http.StatusForbidden))
	e.pass()
	e.dav.FailWith(nil)
	wantRefused(t, hub, api.ActivityServerCarddav, 403)
}

func TestActivityOfARefusedContactDelete(t *testing.T) {
	e := newContactEnv(t)
	hub := e.recording()
	if _, err := e.people.Delete(t.Context(), &api.PeopleDeleteParams{ID: e.contact("Ann Smith")}); err != nil {
		t.Fatal(err)
	}
	e.dav.FailWith(failOn(http.MethodDelete, http.StatusForbidden))
	e.pass()
	e.dav.FailWith(nil)
	wantRefused(t, hub, api.ActivityServerCarddav, 403)
}

func TestActivityOfARefusedEventEdit(t *testing.T) {
	e := newEventEnv(t)
	hub := e.recording()
	if _, err := e.cal.Update(t.Context(), &api.CalendarUpdateParams{ID: e.event("Planning"), Fields: api.EventFields{Summary: ptr2("Planning Q4")}}); err != nil {
		t.Fatal(err)
	}
	e.dav.FailWith(failOn(http.MethodPut, http.StatusForbidden))
	e.pass()
	e.dav.FailWith(nil)
	wantRefused(t, hub, api.ActivityServerCaldav, 403)
}

func TestActivityOfARefusedEventDelete(t *testing.T) {
	e := newEventEnv(t)
	hub := e.recording()
	if _, err := e.cal.Delete(t.Context(), &api.CalendarDeleteParams{ID: e.event("Planning")}); err != nil {
		t.Fatal(err)
	}
	e.dav.FailWith(failOn(http.MethodDelete, http.StatusForbidden))
	e.pass()
	e.dav.FailWith(nil)
	wantRefused(t, hub, api.ActivityServerCaldav, 403)
}

func TestActivityOfARefusedCalDAVTask(t *testing.T) {
	e := tasksDAVEnv(t, false)
	hub := e.recording()
	todo := e.dav.Calendar("todo", "Reminders", "", "VTODO")
	e.dav.Put(todo+"milk.ics", vtodo("milk", "Buy milk"))
	e.pass()
	tasks := engine.New(engine.Deps{DB: e.db, PIM: e.m}).Tasks()
	if _, err := tasks.Update(t.Context(), &api.TasksUpdateParams{ID: rowIDOf(t, e, "Buy milk"), Title: ptr("Buy oat milk")}); err != nil {
		t.Fatal(err)
	}
	e.dav.FailWith(failOn(http.MethodPut, http.StatusForbidden))
	e.pass()
	e.dav.FailWith(nil)
	wantRefused(t, hub, api.ActivityServerCaldav, 403)
}

func TestActivityOfARefusedGoogleTask(t *testing.T) {
	e := newTasksEnv(t, allScopes())
	hub := e.recordingTasks(e.tokens)
	e.srv.AddList("Tasks")
	e.mustPass()
	tasks := engine.New(engine.Deps{DB: e.db, PIM: e.m}).Tasks()
	if _, err := tasks.Create(t.Context(), &api.TasksCreateParams{Title: "Refused"}); err != nil {
		t.Fatal(err)
	}
	e.srv.FailWith(failOn(http.MethodPost, http.StatusBadRequest))
	e.mustPass()
	e.srv.FailWith(nil)
	wantRefused(t, hub, api.ActivityServerGtasks, 400)
}

// A failure that will be retried is no refusal.
func TestActivityOfARetriedWriteIsNoRefusal(t *testing.T) {
	e := newContactEnv(t)
	hub := e.recording()
	if _, err := e.people.Delete(t.Context(), &api.PeopleDeleteParams{ID: e.contact("Ann Smith")}); err != nil {
		t.Fatal(err)
	}
	e.dav.FailWith(failOn(http.MethodDelete, http.StatusServiceUnavailable))
	e.pass()
	e.dav.FailWith(nil)
	for _, x := range hub.Recent(activity.MaxRecent) {
		if x.Kind == api.ActivityKindPimrefused {
			t.Errorf("a 503 recorded %+v", x)
		}
	}
}

func TestActivityCountsDAVCollectionsAcrossBatchesAndDeletions(t *testing.T) {
	e := newContactEnv(t)
	hub := e.recording() // two objects per multiget
	other := e.dav.AddressBook("other", "Other")
	for _, book := range []string{e.book, other} {
		for i := range 5 {
			card := fmt.Sprintf("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:person%d\r\nFN:Person %d\r\nEND:VCARD\r\n", i, i)
			e.dav.Put(fmt.Sprintf("%sperson%d.vcf", book, i), []byte(card))
		}
	}
	e.dav.Delete(e.book + "ann.vcf")
	e.pass()
	got := hub.Recent(1)
	if len(got) != 1 || got[0].Changed == nil || *got[0].Changed != 2 {
		t.Fatalf("pass = %+v, want two changed collections", got)
	}
	e.pass()
	got = hub.Recent(1)
	if len(got) != 1 || got[0].Changed == nil || *got[0].Changed != 0 {
		t.Fatalf("quiet pass = %+v, want no changed collections", got)
	}
}
