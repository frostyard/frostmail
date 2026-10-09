package pimsync_test

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/store"
)

func TestCalDAVTaskWrites(t *testing.T) {
	e := tasksDAVEnv(t, false)
	ctx := t.Context()
	todo := e.dav.Calendar("todo", "Reminders", "", "VTODO")
	e.dav.Put(todo+"milk.ics", vtodo("milk", "Buy milk"))
	e.pass()
	tasks := engine.New(engine.Deps{DB: e.db, PIM: e.m}).Tasks()
	milk := rowIDOf(t, e, "Buy milk")

	// A new task edited twice, and a change to one the server has, before
	// the next pass: one creation and one put go.
	made, err := tasks.Create(ctx, &api.TasksCreateParams{Title: "Call Ann", Due: ptr("2026-10-12")})
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Call Ann back", "Call Ann about the offsite"} {
		if _, err := tasks.Update(ctx, &api.TasksUpdateParams{ID: made.ID, Title: ptr(title)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tasks.Update(ctx, &api.TasksUpdateParams{ID: milk, Completed: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Update(ctx, &api.TasksUpdateParams{ID: milk, Notes: ptr("Oat")}); err != nil {
		t.Fatal(err)
	}
	ops, _ := e.db.DuePIMOps(ctx, e.acct.ID, e.db.Now())
	if len(ops) != 2 {
		t.Errorf("queued = %+v; one put per object", ops)
	}
	e.pass()
	row, _ := e.db.Task(ctx, made.ID)
	obj, _ := e.db.GetObject(ctx, made.ID)
	data, etag, ok := e.dav.Object(obj.Href)
	if !ok || !strings.Contains(string(data), "SUMMARY:Call Ann about the offsite") || !strings.Contains(string(data), "DUE;VALUE=DATE:20261012") ||
		obj.ETag != etag || row.Title != "Call Ann about the offsite" {
		t.Errorf("the new task on the server = %s (local %+v, etag %q vs %q)", data, row, obj.ETag, etag)
	}
	if data, _, _ := e.dav.Object(todo + "milk.ics"); !strings.Contains(string(data), "STATUS:COMPLETED") || !strings.Contains(string(data), "DESCRIPTION:Oat") {
		t.Errorf("milk on the server = %s", data)
	}

	// Deleting reaches the server; a task the server never had is only
	// dropped.
	if err := tasks.Delete(ctx, &api.TasksDeleteParams{ID: made.ID}); err != nil {
		t.Fatal(err)
	}
	temp, _ := tasks.Create(ctx, &api.TasksCreateParams{Title: "Temp"})
	if err := tasks.Delete(ctx, &api.TasksDeleteParams{ID: temp.ID}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	if _, _, ok := e.dav.Object(obj.Href); ok {
		t.Error("the deleted task is still on the server")
	}
	for _, r := range e.dav.Requests() {
		if r.Method == "PUT" {
			t.Errorf("a put for a task never written: %+v", r)
		}
	}

	// Another client's change wins over a local one made on the old ETag.
	if _, err := tasks.Update(ctx, &api.TasksUpdateParams{ID: milk, Title: ptr("Buy milk today")}); err != nil {
		t.Fatal(err)
	}
	e.dav.Put(todo+"milk.ics", vtodo("milk", "Buy almond milk"))
	e.pass()
	e.pass()
	if got, _ := e.db.Task(ctx, milk); got.Title != "Buy almond milk" {
		t.Errorf("after a conflicting change = %+v", got)
	}
}

func rowIDOf(t *testing.T, e *env, title string) int64 {
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
