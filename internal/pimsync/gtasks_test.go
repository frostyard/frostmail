package pimsync_test

// CONTRACT TEST for task card T-0080 (docs/tasks). Do not edit.

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/gtasks/gtaskstest"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// tokens hands out an access token; Invalidate makes the next one right.
type tokens struct {
	mu          sync.Mutex
	token       string
	invalidated int
}

func (f *tokens) AccessToken(context.Context, int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.token, nil
}

func (f *tokens) Invalidate(int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated++
	f.token = gtaskstest.Token
}

// tasksEnv is a Gmail account with Tasks on, granted the Tasks scope, over
// an in-process Tasks API.
type tasksEnv struct {
	t      *testing.T
	db     *store.DB
	srv    *gtaskstest.Server
	m      *pimsync.Manager
	acct   store.Account
	tokens *tokens
	events []string
}

func newTasksEnv(t *testing.T, scopes string) *tasksEnv {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := &tasksEnv{t: t, db: db, srv: gtaskstest.New(t, gtaskstest.Options{PageSize: 2}), tokens: &tokens{token: gtaskstest.Token}}
	db.OnCommit = func(evs []api.EventEnvelope) {
		for _, ev := range evs {
			e.events = append(e.events, ev.Event)
		}
	}
	server := store.ServerConfig{Host: "imap.gmail.com", Port: 993, TLS: api.TLSModeTLS, Username: "ann@gmail.example"}
	err = db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		e.acct, err = tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindGmail, Email: "ann@gmail.example",
			Auth: api.AuthKindOAuth2, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		if err := tx.SetGrantedScopes(ctx, e.acct.ID, scopes); err != nil {
			return err
		}
		return tx.SetService(ctx, e.acct.ID, api.ServiceKindTasks, true, e.srv.Base+"/")
	})
	if err != nil {
		t.Fatal(err)
	}
	e.m = pimsync.New(db, secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json")),
		slog.New(slog.NewTextHandler(io.Discard, nil)), pimsync.Config{HTTP: e.srv.Client, Tokens: e.tokens})
	return e
}

func allScopes() string { return "https://mail.google.com/ " + oauth.GoogleTasksScope }

func (e *tasksEnv) pass() error {
	e.t.Helper()
	e.events = nil
	e.srv.ResetRequests()
	return e.m.Pass(e.t.Context(), e.acct.ID)
}

func (e *tasksEnv) mustPass() {
	e.t.Helper()
	if err := e.pass(); err != nil {
		e.t.Fatalf("Pass: %v", err)
	}
}

func (e *tasksEnv) titles(f store.TaskFilter) []string {
	e.t.Helper()
	f.Completed = true
	rows, err := e.db.Tasks(e.t.Context(), f)
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.Title)
	}
	return out
}

func TestGoogleTasksSync(t *testing.T) {
	e := newTasksEnv(t, allScopes())
	ctx := t.Context()
	work := e.srv.AddList("Work")
	home := e.srv.AddList("Home")
	e.srv.AddTask(work, gtaskstest.Task{Title: "Archive", Status: "completed"})
	report := e.srv.AddTask(work, gtaskstest.Task{Title: "Report", Notes: "Q4", Due: "2026-10-09T00:00:00.000Z"})
	e.srv.AddTask(work, gtaskstest.Task{Title: "Charts", Parent: report})
	e.srv.AddTask(work, gtaskstest.Task{Title: "Reply to Bob", Links: []gtaskstest.Link{
		{Type: "email", Description: "Re: offsite", Link: "https://mail.google.com/mail/#all/18c3f2a0b1d4e5f6"}}})
	gone := e.srv.AddTask(home, gtaskstest.Task{Title: "Gone"})
	e.srv.Remove(home, gone)
	e.srv.AddTask(home, gtaskstest.Task{Title: "Groceries"})
	e.mustPass()

	lists, err := e.db.Collections(ctx, store.CollectionFilter{AccountID: e.acct.ID, Kind: api.CollectionKindTasklist})
	if err != nil || len(lists) != 2 || lists[0].Href != work || lists[0].Name != "Work" || lists[1].Name != "Home" {
		t.Fatalf("lists = %+v, %v", lists, err)
	}
	if got := e.titles(store.TaskFilter{}); !slices.Equal(got, []string{"Reply to Bob", "Report", "Charts", "Archive", "Groceries"}) {
		t.Errorf("tasks = %q", got)
	}
	rows, _ := e.db.Tasks(ctx, store.TaskFilter{Completed: true})
	byTitle := map[string]store.TaskRow{}
	for _, r := range rows {
		byTitle[r.Title] = r
	}
	if r := byTitle["Report"]; r.UID != report || r.Notes != "Q4" || r.Due != "2026-10-09" || r.Position == "" || r.ListID != lists[0].ID {
		t.Errorf("Report = %+v", r)
	}
	if r := byTitle["Charts"]; r.ParentUID != report || r.ParentID != byTitle["Report"].ObjectID {
		t.Errorf("Charts = %+v", r)
	}
	if r := byTitle["Archive"]; !r.Completed || r.CompletedAt == nil {
		t.Errorf("Archive = %+v", r)
	}
	if r := byTitle["Reply to Bob"]; r.GmThrID != 0x18c3f2a0b1d4e5f6 {
		t.Errorf("Reply to Bob's thread = %x", r.GmThrID)
	}
	obj, err := e.db.ObjectByHref(ctx, lists[0].ID, report)
	if err != nil || obj.Kind != store.ObjectGTask || obj.UID != report || !strings.Contains(string(obj.Raw), `"title":"Report"`) {
		t.Errorf("Report's object = %+v, %v", obj, err)
	}
	if !slices.Contains(e.events, "tasks.changed") {
		t.Errorf("events = %q", e.events)
	}

	// A pass without changes asks from the newest update seen and changes
	// nothing.
	e.mustPass()
	var since string
	for _, r := range e.srv.Requests() {
		if strings.HasSuffix(r.Path, "/lists/"+work+"/tasks") {
			since = r.Query
		}
	}
	if !strings.Contains(since, "updatedMin=") || slices.Contains(e.events, "tasks.changed") {
		t.Errorf("an idle pass: query %q, events %q", since, e.events)
	}

	// Another client's changes arrive.
	e.srv.Update(work, report, func(t *gtaskstest.Task) { t.Title = "Report v2" })
	e.srv.Remove(work, byTitle["Archive"].UID)
	e.srv.AddTask(home, gtaskstest.Task{Title: "Pharmacy"})
	e.mustPass()
	if got := e.titles(store.TaskFilter{}); !slices.Equal(got, []string{"Reply to Bob", "Report v2", "Charts", "Pharmacy", "Groceries"}) {
		t.Errorf("after changes = %q", got)
	}
	if !slices.Contains(e.events, "tasks.changed") {
		t.Errorf("events after changes = %q", e.events)
	}

	// A list removed on the server goes with its tasks.
	e.srv.RemoveList(home)
	e.mustPass()
	if got := e.titles(store.TaskFilter{}); !slices.Equal(got, []string{"Reply to Bob", "Report v2", "Charts"}) {
		t.Errorf("after the list went = %q", got)
	}
}

func TestGoogleTasksPendingLeftAlone(t *testing.T) {
	e := newTasksEnv(t, allScopes())
	ctx := t.Context()
	list := e.srv.AddList("Tasks")
	id := e.srv.AddTask(list, gtaskstest.Task{Title: "Mine"})
	e.mustPass()
	cols, _ := e.db.Collections(ctx, store.CollectionFilter{AccountID: e.acct.ID, Kind: api.CollectionKindTasklist})
	obj, _ := e.db.ObjectByHref(ctx, cols[0].ID, id)
	// A local change waits to be written (later than now, so this pass
	// does not replay it); the server's version must not replace it.
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: e.acct.ID, CollectionID: cols[0].ID, ObjectID: &obj.ID,
			Kind: "tasks.patch", Href: id, Payload: `{"title":"Mine, edited"}`, NextTryAt: time.Now().Add(time.Hour)})
		if err != nil {
			return err
		}
		return tx.IndexTask(ctx, obj.ID, store.TaskRow{UID: id, Title: "Mine, edited"})
	}); err != nil {
		t.Fatal(err)
	}
	e.srv.Update(list, id, func(t *gtaskstest.Task) { t.Title = "Theirs" })
	e.mustPass()
	if got := e.titles(store.TaskFilter{}); !slices.Equal(got, []string{"Mine, edited"}) {
		t.Errorf("tasks = %q", got)
	}
}

func TestGoogleTasksAuth(t *testing.T) {
	e := newTasksEnv(t, allScopes())
	e.srv.AddList("Tasks")
	e.tokens.token = "stale"
	e.mustPass()
	if e.tokens.invalidated != 1 {
		t.Errorf("invalidated %d times; a refused token is replaced once", e.tokens.invalidated)
	}

	without := newTasksEnv(t, "https://mail.google.com/")
	without.srv.AddList("Tasks")
	if err := without.pass(); err == nil || !strings.Contains(err.Error(), "sign in") {
		t.Errorf("without the Tasks scope: %v", err)
	}
	if len(without.srv.Requests()) != 0 {
		t.Errorf("asked the API without the scope: %+v", without.srv.Requests())
	}
}
