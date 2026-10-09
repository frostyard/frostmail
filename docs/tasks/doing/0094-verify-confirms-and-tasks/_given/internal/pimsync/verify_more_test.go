// CONTRACT TEST for task card T-0094 (docs/tasks). Do not edit.
package pimsync_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/gtasks/gtaskstest"
	"github.com/frostyard/frostmail/internal/pimsync"
)

// TestVerifyLeavesOutPhantoms: Google lists objects in a PROPFIND that its
// sync reports as deleted and a multiget cannot fetch. They are on neither
// side, however many there are; an object the server does serve is still
// missing locally until a pass fetches it.
func TestVerifyLeavesOutPhantoms(t *testing.T) {
	e := newEnv(t, davtest.Options{}, api.ServiceKindCalendar)
	ctx := t.Context()
	cal := e.dav.Calendar("work", "Work", "", "VEVENT")
	e.dav.Put(cal+"a.ics", vevent("a"))
	e.pass()
	for i := range 21 {
		e.dav.Phantom(cal + fmt.Sprintf("gone-%02d.ics", i))
	}
	e.dav.Put(cal+"new.ics", vevent("new"))
	e.dav.ResetRequests()
	checks, err := e.m.Verify(ctx, e.acct.ID)
	if err != nil || len(checks) != 1 {
		t.Fatalf("checks = %+v, %v", checks, err)
	}
	c := checks[0]
	if c.Server != 2 || c.Local != 1 || !slices.Equal(c.MissingLocally, []string{cal + "new.ics"}) ||
		len(c.MissingOnServer) != 0 || c.EtagDiffs != 0 {
		t.Errorf("check = %+v", c)
	}
	for _, r := range e.dav.Requests() {
		if r.Method != "PROPFIND" && r.Report != "calendar-multiget" {
			t.Errorf("verify sent %s %s %s", r.Method, r.Path, r.Report)
		}
	}
	e.pass()
	if checks, err := e.m.Verify(ctx, e.acct.ID); err != nil || len(checks) != 1 || !pimsync.Clean(checks[0]) ||
		checks[0].Server != 2 || checks[0].Local != 2 {
		t.Errorf("after a pass = %+v, %v", checks, err)
	}
}

// TestVerifyCalDAVTasks: a CalDAV task list is checked as a calendar is.
func TestVerifyCalDAVTasks(t *testing.T) {
	e := tasksDAVEnv(t, false)
	ctx := t.Context()
	todo := e.dav.Calendar("todo", "Reminders", "", "VTODO")
	e.dav.Put(todo+"milk.ics", vtodo("milk", "Buy milk"))
	e.pass()
	checks, err := e.m.Verify(ctx, e.acct.ID)
	if err != nil || len(checks) != 1 || !pimsync.Clean(checks[0]) || checks[0].Name != "Reminders" ||
		checks[0].Server != 1 || checks[0].Local != 1 {
		t.Fatalf("after a pass = %+v, %v", checks, err)
	}
	e.dav.Put(todo+"eggs.ics", vtodo("eggs", "Buy eggs"))
	checks, err = e.m.Verify(ctx, e.acct.ID)
	if err != nil || len(checks) != 1 || !slices.Equal(checks[0].MissingLocally, []string{todo + "eggs.ics"}) {
		t.Errorf("with a new task = %+v, %v", checks, err)
	}
}

// TestVerifyGoogleTasks: a Google task list's tasks, hidden and completed
// ones included and deleted ones not, against the stored ones by ID and
// ETag. Verify only reads.
func TestVerifyGoogleTasks(t *testing.T) {
	e := newTasksEnv(t, allScopes())
	ctx := t.Context()
	work := e.srv.AddList("Work")
	a := e.srv.AddTask(work, gtaskstest.Task{Title: "A"})
	b := e.srv.AddTask(work, gtaskstest.Task{Title: "B", Status: "completed"})
	e.srv.AddTask(work, gtaskstest.Task{Title: "Cleared", Status: "completed", Hidden: true})
	gone := e.srv.AddTask(work, gtaskstest.Task{Title: "Gone"})
	e.srv.Remove(work, gone)
	e.mustPass()
	checks, err := e.m.Verify(ctx, e.acct.ID)
	if err != nil || len(checks) != 1 || !pimsync.Clean(checks[0]) || checks[0].Name != "Work" ||
		checks[0].Server != 3 || checks[0].Local != 3 {
		t.Fatalf("after a pass = %+v, %v", checks, err)
	}

	// Another client changes A, deletes B and adds C.
	e.srv.Update(work, a, func(t *gtaskstest.Task) { t.Title = "A2" })
	e.srv.Remove(work, b)
	c := e.srv.AddTask(work, gtaskstest.Task{Title: "C"})
	e.srv.ResetRequests()
	checks, err = e.m.Verify(ctx, e.acct.ID)
	if err != nil || len(checks) != 1 {
		t.Fatalf("checks = %+v, %v", checks, err)
	}
	got := checks[0]
	if pimsync.Clean(got) || got.Server != 3 || got.Local != 3 || got.EtagDiffs != 1 ||
		!slices.Equal(got.MissingLocally, []string{c}) || !slices.Equal(got.MissingOnServer, []string{b}) {
		t.Errorf("check = %+v", got)
	}
	for _, r := range e.srv.Requests() {
		if r.Method != "GET" {
			t.Errorf("verify sent %s %s", r.Method, r.Path)
		}
	}
	e.mustPass()
	if checks, err := e.m.Verify(ctx, e.acct.ID); err != nil || len(checks) != 1 || !pimsync.Clean(checks[0]) {
		t.Errorf("after another pass = %+v, %v", checks, err)
	}
}
