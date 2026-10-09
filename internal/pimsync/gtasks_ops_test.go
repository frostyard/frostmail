package pimsync_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/gtasks/gtaskstest"
	"github.com/frostyard/frostmail/internal/store"
)

func ptr[T any](v T) *T { return &v }

// serverTitles lists a server list's tasks that are not deleted, as
// "title" or "parent title > title".
func serverTitles(srv *gtaskstest.Server, list string) []string {
	byID := map[string]string{}
	all := srv.Tasks(list)
	for _, t := range all {
		byID[t.ID] = t.Title
	}
	var out []string
	for _, t := range all {
		if t.Deleted {
			continue
		}
		if t.Parent != "" {
			out = append(out, byID[t.Parent]+" > "+t.Title)
		} else {
			out = append(out, t.Title)
		}
	}
	return out
}

func TestGoogleTaskWrites(t *testing.T) {
	e := newTasksEnv(t, allScopes())
	ctx := t.Context()
	list := e.srv.AddList("Tasks")
	existing := e.srv.AddTask(list, gtaskstest.Task{Title: "Existing"})
	e.mustPass()
	tasks := engine.New(engine.Deps{DB: e.db, PIM: e.m}).Tasks()
	cols, _ := e.db.Collections(ctx, store.CollectionFilter{AccountID: e.acct.ID, Kind: api.CollectionKindTasklist})
	listID := cols[0].ID

	// New tasks, a subtask of one of them, and a change to one that Google
	// does not have yet, all before the next pass.
	made, err := tasks.Create(ctx, &api.TasksCreateParams{Title: "Call Ann", Due: ptr("2026-10-12"), ListID: &listID})
	if err != nil || made.Title != "Call Ann" || made.Due != "2026-10-12" || made.ListID != listID {
		t.Fatalf("create = %+v, %v", made, err)
	}
	parent, _ := tasks.Create(ctx, &api.TasksCreateParams{Title: "Offsite"})
	child, err := tasks.Create(ctx, &api.TasksCreateParams{Title: "Book a room", ParentID: &parent.ID})
	if err != nil || child.ParentID == nil || *child.ParentID != parent.ID {
		t.Fatalf("subtask = %+v, %v", child, err)
	}
	draft, _ := tasks.Create(ctx, &api.TasksCreateParams{Title: "Draft"})
	if _, err := tasks.Update(ctx, &api.TasksUpdateParams{ID: draft.ID, Title: ptr("Draft 2")}); err != nil {
		t.Fatal(err)
	}
	temp, _ := tasks.Create(ctx, &api.TasksCreateParams{Title: "Temp"})
	if err := tasks.Delete(ctx, &api.TasksDeleteParams{ID: temp.ID}); err != nil {
		t.Fatal(err)
	}
	rows, _ := e.db.Tasks(ctx, store.TaskFilter{})
	var local []string
	for _, r := range rows {
		local = append(local, r.Title)
	}
	if !slices.Equal(local, []string{"Draft 2", "Offsite", "Book a room", "Call Ann", "Existing"}) {
		t.Errorf("local tasks before the pass = %q", local)
	}
	e.mustPass()
	if got := serverTitles(e.srv, list); !slices.Equal(got, []string{"Draft 2", "Offsite", "Offsite > Book a room", "Call Ann", "Existing"}) {
		t.Errorf("server tasks = %q", got)
	}
	ann, _ := e.db.Task(ctx, made.ID)
	if server, ok := e.srv.Task(list, ann.UID); !ok || server.Title != "Call Ann" || server.Due != "2026-10-12T00:00:00.000Z" {
		t.Errorf("Call Ann on the server = %+v (uid %q)", server, ann.UID)
	}
	if room, _ := e.db.Task(ctx, child.ID); room.ParentID != parent.ID {
		t.Errorf("the subtask's parent = %+v", room)
	}
	if ops, _ := e.db.DuePIMOps(ctx, e.acct.ID, e.db.Now()); len(ops) != 0 {
		t.Errorf("ops left = %+v", ops)
	}

	// Completing and deleting reach Google.
	if _, err := tasks.Update(ctx, &api.TasksUpdateParams{ID: rowID(t, e, "Existing"), Completed: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Delete(ctx, &api.TasksDeleteParams{ID: made.ID}); err != nil {
		t.Fatal(err)
	}
	e.mustPass()
	if got, _ := e.srv.Task(list, existing); got.Status != "completed" {
		t.Errorf("Existing on the server = %+v", got)
	}
	if got := serverTitles(e.srv, list); slices.Contains(got, "Call Ann") {
		t.Errorf("Call Ann is still on the server: %q", got)
	}

	// A change Google refuses fails, and the server's version comes back.
	e.srv.FailWith(func(r *http.Request) int {
		if r.Method == http.MethodPatch {
			return http.StatusBadRequest
		}
		return 0
	})
	if _, err := tasks.Update(ctx, &api.TasksUpdateParams{ID: draft.ID, Title: ptr("Refused")}); err != nil {
		t.Fatal(err)
	}
	e.mustPass()
	e.srv.FailWith(nil)
	e.mustPass()
	if got, _ := e.db.Task(ctx, draft.ID); got.Title != "Draft 2" {
		t.Errorf("after a refused change = %+v", got)
	}

	// A new task Google refuses goes.
	e.srv.FailWith(func(r *http.Request) int {
		if r.Method == http.MethodPost {
			return http.StatusBadRequest
		}
		return 0
	})
	refused, err := tasks.Create(ctx, &api.TasksCreateParams{Title: "Refused"})
	if err != nil {
		t.Fatal(err)
	}
	e.mustPass()
	if _, err := e.db.Task(ctx, refused.ID); err == nil {
		t.Error("a refused new task is still here")
	}
	e.srv.FailWith(nil)
}

func rowID(t *testing.T, e *tasksEnv, title string) int64 {
	t.Helper()
	rows, _ := e.db.Tasks(t.Context(), store.TaskFilter{Completed: true})
	for _, r := range rows {
		if r.Title == title {
			return r.ObjectID
		}
	}
	t.Fatalf("no task %q", title)
	return 0
}
