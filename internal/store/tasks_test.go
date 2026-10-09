package store

// CONTRACT TEST for task card T-0079 (docs/tasks). Do not edit.

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// taskFixture is two accounts' task lists: a and b (read-only) shown in
// account one (tasks on), c hidden, x in account two (tasks off).
type taskFixture struct {
	d          *DB
	one        Account
	a, b, c, x Collection
	ids        map[string]int64 // by title
}

func newTaskFixture(t *testing.T) *taskFixture {
	t.Helper()
	d, _ := openTest(t)
	ctx := t.Context()
	f := &taskFixture{d: d, ids: map[string]int64{}}
	f.one = insertAccount(t, d, sampleAccount("one@mailtest.test"))
	two := insertAccount(t, d, sampleAccount("two@mailtest.test"))
	if err := d.Tx(ctx, func(tx *Tx) error {
		if err := tx.SetService(ctx, f.one.ID, api.ServiceKindTasks, true, "https://tasks.test/"); err != nil {
			return err
		}
		return tx.SetService(ctx, two.ID, api.ServiceKindTasks, false, "https://tasks.test/")
	}); err != nil {
		t.Fatal(err)
	}
	cols := replaceCols(t, d, f.one.ID, api.CollectionKindTasklist, RemoteCollection{Href: "L1", Name: "Work"},
		RemoteCollection{Href: "L2", Name: "Shared", ReadOnly: true}, RemoteCollection{Href: "L3", Name: "Hidden"})
	f.a, f.b, f.c = cols[0], cols[1], cols[2]
	f.x = replaceCols(t, d, two.ID, api.CollectionKindTasklist, RemoteCollection{Href: "LX", Name: "Off"})[0]
	off := false
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.UpdateCollection(ctx, f.c.ID, &off, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	done := time.Date(2026, 10, 8, 14, 30, 0, 0, time.UTC)
	for _, r := range []struct {
		col Collection
		row TaskRow
	}{
		{f.a, TaskRow{UID: "p1", Title: "Report", Position: "00002", Due: "2026-10-10"}},
		{f.a, TaskRow{UID: "s1", ParentUID: "p1", Title: "Charts", Position: "00001"}},
		{f.a, TaskRow{UID: "s2", ParentUID: "p1", Title: "Data", Position: "00000", Completed: true, CompletedAt: &done}},
		{f.a, TaskRow{UID: "p2", Title: "Call Ann", Position: "00001", Due: "2026-10-20", Notes: "About the offsite", GmThrID: 1234}},
		{f.a, TaskRow{UID: "p3", Title: "Someday"}},
		{f.a, TaskRow{UID: "p4", Title: "Archive", Completed: true}},
		{f.a, TaskRow{UID: "p5", Title: "Done parent", Position: "00003", Completed: true}},
		{f.a, TaskRow{UID: "s5", ParentUID: "p5", Title: "Leftover", Position: "00009"}},
		{f.b, TaskRow{UID: "q1", Title: "Shared task", Position: "00001"}},
		{f.c, TaskRow{UID: "h1", Title: "In a hidden list"}},
		{f.x, TaskRow{UID: "x1", Title: "Tasks off"}},
	} {
		f.ids[r.row.Title] = f.put(t, r.col, r.row)
	}
	return f
}

// put stores an object with a task and returns its ID.
func (f *taskFixture) put(t *testing.T, col Collection, r TaskRow) int64 {
	t.Helper()
	ctx := t.Context()
	var id int64
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		var err error
		id, err = tx.PutObject(ctx, Object{CollectionID: col.ID, Href: col.Href + "/" + r.UID, Kind: ObjectGTask, UID: r.UID, Raw: []byte("{}")})
		if err != nil {
			return err
		}
		return tx.IndexTask(ctx, id, r)
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *taskFixture) titles(t *testing.T, filter TaskFilter) []string {
	t.Helper()
	rows, err := f.d.Tasks(t.Context(), filter)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.Title)
	}
	return out
}

func TestTaskOrder(t *testing.T) {
	f := newTaskFixture(t)
	// Lists in collection order; parents by position, then those without
	// one by title; each followed by its subtasks; a subtask whose parent
	// is not listed stands at the top level.
	if got := f.titles(t, TaskFilter{}); !slices.Equal(got, []string{
		"Call Ann", "Report", "Charts", "Leftover", "Someday", "Shared task",
	}) {
		t.Errorf("open tasks = %q", got)
	}
	if got := f.titles(t, TaskFilter{Completed: true}); !slices.Equal(got, []string{
		"Call Ann", "Report", "Data", "Charts", "Done parent", "Leftover", "Archive", "Someday", "Shared task",
	}) {
		t.Errorf("all tasks = %q", got)
	}
	if got := f.titles(t, TaskFilter{ListID: f.b.ID}); !slices.Equal(got, []string{"Shared task"}) {
		t.Errorf("list b = %q", got)
	}
	if got := f.titles(t, TaskFilter{ListID: f.c.ID}); len(got) != 0 {
		t.Errorf("a hidden list = %q", got)
	}
	if got := f.titles(t, TaskFilter{DueBefore: "2026-10-15"}); !slices.Equal(got, []string{"Report"}) {
		t.Errorf("due before Oct 15 = %q", got)
	}
}

func TestTaskRows(t *testing.T) {
	f := newTaskFixture(t)
	ctx := t.Context()
	rows, _ := f.d.Tasks(ctx, TaskFilter{Completed: true})
	byTitle := map[string]TaskRow{}
	for _, r := range rows {
		byTitle[r.Title] = r
	}
	call := byTitle["Call Ann"]
	if call.ObjectID != f.ids["Call Ann"] || call.UID != "p2" || call.Notes != "About the offsite" || call.Due != "2026-10-20" ||
		call.GmThrID != 1234 || call.ListID != f.a.ID || call.AccountID != f.one.ID || call.ReadOnly || call.ParentID != 0 {
		t.Errorf("Call Ann = %+v", call)
	}
	if c := byTitle["Charts"]; c.ParentID != f.ids["Report"] || c.ParentUID != "p1" {
		t.Errorf("Charts = %+v", c)
	}
	if d := byTitle["Data"]; !d.Completed || d.CompletedAt == nil || !d.CompletedAt.Equal(time.Date(2026, 10, 8, 14, 30, 0, 0, time.UTC)) {
		t.Errorf("Data = %+v", d)
	}
	if a := byTitle["Archive"]; !a.Completed || a.CompletedAt != nil {
		t.Errorf("Archive = %+v", a)
	}
	if s := byTitle["Shared task"]; !s.ReadOnly || s.ListID != f.b.ID {
		t.Errorf("Shared task = %+v", s)
	}

	one, err := f.d.Task(ctx, f.ids["Leftover"])
	if err != nil || one.Title != "Leftover" || one.ParentID != f.ids["Done parent"] {
		t.Errorf("Task = %+v, %v", one, err)
	}
	if _, err := f.d.Task(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown task: %v", err)
	}

	// Indexing again replaces the row; removing it takes it out.
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		if err := tx.IndexTask(ctx, f.ids["Call Ann"], TaskRow{UID: "p2", Title: "Call Ann back", Position: "00001"}); err != nil {
			return err
		}
		return tx.RemoveTask(ctx, f.ids["Someday"])
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.titles(t, TaskFilter{ListID: f.a.ID}); !slices.Equal(got, []string{"Call Ann back", "Report", "Charts", "Leftover"}) {
		t.Errorf("after changes = %q", got)
	}
	if again, _ := f.d.Task(ctx, f.ids["Call Ann"]); again.Notes != "" || again.GmThrID != 0 || again.Due != "" {
		t.Errorf("replaced row kept old fields: %+v", again)
	}
	if err := f.d.Tx(ctx, func(tx *Tx) error { return tx.RemoveTask(ctx, 999999) }); err != nil {
		t.Errorf("removing no task: %v", err)
	}
}
