package pimsync_test

// CONTRACT TEST for task card T-0081 (docs/tasks). Do not edit.

import (
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/store"
)

func vtodo(uid, summary string, extra ...string) []byte {
	return ics(append(append([]string{"BEGIN:VTODO", "UID:" + uid, "DTSTAMP:20261001T000000Z", "SUMMARY:" + summary}, extra...), "END:VTODO")...)
}

// tasksDAVEnv is newEnv with tasks (and, when asked, the calendar) on,
// both discovered from the calendar home.
func tasksDAVEnv(t *testing.T, calendar bool) *env {
	t.Helper()
	e := newEnv(t, davtest.Options{})
	ctx := t.Context()
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetService(ctx, e.acct.ID, api.ServiceKindTasks, true, e.dav.URL+davtest.CalendarsHome); err != nil {
			return err
		}
		if calendar {
			return tx.SetService(ctx, e.acct.ID, api.ServiceKindCalendar, true, e.dav.URL+davtest.CalendarsHome)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) taskTitles() []string {
	e.t.Helper()
	rows, err := e.db.Tasks(e.t.Context(), store.TaskFilter{Completed: true})
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		mark := ""
		if r.Completed {
			mark = " (done)"
		}
		if r.ParentID != 0 {
			mark += " (sub)"
		}
		out = append(out, r.Title+mark)
	}
	return out
}

func TestCalDAVTaskLists(t *testing.T) {
	e := tasksDAVEnv(t, false)
	ctx := t.Context()
	todo := e.dav.Calendar("todo", "Reminders", "#ff9500", "VTODO")
	work := e.dav.Calendar("work", "Work", "", "VEVENT", "VTODO")
	e.dav.Calendar("events", "Events", "", "VEVENT")
	e.dav.Put(todo+"milk.ics", vtodo("milk", "Buy milk", "X-APPLE-SORT-ORDER:2"))
	e.dav.Put(todo+"bread.ics", vtodo("bread", "Buy bread", "X-APPLE-SORT-ORDER:1", "DUE;VALUE=DATE:20261012"))
	e.dav.Put(work+"plan.ics", vtodo("plan", "Plan Q4"))
	e.dav.Put(work+"slides.ics", vtodo("slides", "Slides", "RELATED-TO;RELTYPE=PARENT:plan"))
	e.dav.Put(work+"meeting.ics", vevent("meeting"))
	e.pass()

	lists := e.collections(api.CollectionKindTasklist)
	if len(lists) != 2 || lists[0].Href != todo || lists[0].Name != "Reminders" || lists[1].Href != work {
		t.Fatalf("task lists = %+v; the events-only calendar is not one", lists)
	}
	if cals := e.collections(api.CollectionKindCalendar); len(cals) != 0 {
		t.Errorf("calendars without the calendar service = %+v", cals)
	}
	if got := e.taskTitles(); !slices.Equal(got, []string{"Buy bread", "Buy milk", "Plan Q4", "Slides (sub)"}) {
		t.Errorf("tasks = %q", got)
	}
	bread, _ := e.db.ObjectByHref(ctx, lists[0].ID, todo+"bread.ics")
	if row, err := e.db.Task(ctx, bread.ID); err != nil || row.UID != "bread" || row.Due != "2026-10-12" || row.Position != "1" {
		t.Errorf("Buy bread = %+v, %v", row, err)
	}
	// The task list keeps the event's object, unindexed, so its ETag is known.
	if obj, err := e.db.ObjectByHref(ctx, lists[1].ID, work+"meeting.ics"); err != nil || obj.Kind != store.ObjectVEvent {
		t.Errorf("the event in a task list = %+v, %v", obj, err)
	}
	if !slices.Contains(e.eventNames(), "tasks.changed") {
		t.Errorf("events = %q", e.eventNames())
	}

	// Another client completes one task and deletes another.
	e.dav.Put(todo+"milk.ics", vtodo("milk", "Buy milk", "STATUS:COMPLETED", "COMPLETED:20261009T100000Z"))
	e.dav.Delete(todo + "bread.ics")
	e.pass()
	if got := e.taskTitles(); !slices.Equal(got, []string{"Buy milk (done)", "Plan Q4", "Slides (sub)"}) {
		t.Errorf("after changes = %q", got)
	}
}

func TestCalDAVTasksBesideEvents(t *testing.T) {
	e := tasksDAVEnv(t, true)
	work := e.dav.Calendar("work", "Work", "", "VEVENT", "VTODO")
	e.dav.Put(work+"plan.ics", vtodo("plan", "Plan Q4"))
	e.dav.Put(work+"meeting.ics", vevent("meeting"))
	e.pass()
	cals, lists := e.collections(api.CollectionKindCalendar), e.collections(api.CollectionKindTasklist)
	if len(cals) != 1 || len(lists) != 1 || cals[0].Href != work || lists[0].Href != work {
		t.Fatalf("calendars %+v, task lists %+v", cals, lists)
	}
	if got := e.taskTitles(); !slices.Equal(got, []string{"Plan Q4"}) {
		t.Errorf("tasks = %q", got)
	}
	events, err := e.db.Events(e.t.Context(), store.EventsFilter{})
	if err != nil || len(events) != 1 || events[0].UID != "meeting" {
		t.Errorf("events = %+v, %v", events, err)
	}
}

func TestCalDAVTaskWritesReplay(t *testing.T) {
	e := tasksDAVEnv(t, false)
	ctx := t.Context()
	todo := e.dav.Calendar("todo", "Reminders", "", "VTODO")
	e.dav.Put(todo+"milk.ics", vtodo("milk", "Buy milk"))
	e.pass()
	lists := e.collections(api.CollectionKindTasklist)
	obj, _ := e.db.ObjectByHref(ctx, lists[0].ID, todo+"milk.ics")
	// A local change, as the tasks API makes it: the source patched and a
	// put queued on the ETag it was made on.
	obj.Raw = vtodo("milk", "Buy oat milk")
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		if _, err := tx.PutObject(ctx, obj); err != nil {
			return err
		}
		_, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: e.acct.ID, CollectionID: lists[0].ID, ObjectID: &obj.ID,
			Kind: "put", Href: obj.Href, IfMatch: obj.ETag})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	data, _, ok := e.dav.Object(todo + "milk.ics")
	if !ok || !strings.Contains(string(data), "SUMMARY:Buy oat milk") {
		t.Errorf("the server's task = %s", data)
	}
	if ops, _ := e.db.DuePIMOps(ctx, e.acct.ID, e.db.Now()); len(ops) != 0 {
		t.Errorf("ops left = %+v", ops)
	}
}
