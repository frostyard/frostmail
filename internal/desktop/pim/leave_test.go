package pim

import (
	"errors"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/live"
)

// On a change of maild the module forgets the last one's selections and
// undo entries: the same id means another item on the next maild.
func TestLeaveForgetsTheLastMaild(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.m.tasks.selected = f.report
	f.m.people.selected = 7
	f.m.people.undo = []string{"token"}
	f.m.cal.undo = []string{"token"}
	f.m.cal.selected = &occKey{1, ""}
	f.m.setTaskSource(sourceAll)
	conn := f.m.conn
	f.m.Leave()
	if f.m.tasks.selected != 0 || f.m.people.selected != 0 || f.m.cal.selected != nil {
		t.Errorf("selections kept: task %d, person %d, occurrence %v", f.m.tasks.selected, f.m.people.selected, f.m.cal.selected)
	}
	if len(f.m.people.undo) != 0 || len(f.m.cal.undo) != 0 {
		t.Errorf("undo kept: people %v, calendar %v", f.m.people.undo, f.m.cal.undo)
	}
	if f.m.tasks.source != sourceToday || f.m.conn == conn {
		t.Errorf("source %v, conn %d (was %d)", f.m.tasks.source, f.m.conn, conn)
	}
}

// A task write still out as the module leaves its maild answers after the
// next maild's queue holds a write of the same ID: the old answer neither
// takes that write's place nor indexes an empty queue.
func TestAWriteFromTheLastMaildLeavesTheNextQueue(t *testing.T) {
	first, second := make(chan struct{}), make(chan struct{})
	fake := &fakeTasks{holds: []chan struct{}{first, second}, updateErrs: []error{errors.New("refused")}}
	f := newFixture(t, commands.Tasks, withFakeTasks(fake))
	f.text("Report")
	old, next := "Old", "Next"
	f.m.editTask(f.report, api.TasksUpdateParams{Title: &old})
	f.until("the first write", func() bool { return len(fake.sent()) == 1 })
	f.m.Leave()
	f.until("the next maild's tasks", func() bool { return f.m.task(f.report) != nil })
	f.m.editTask(f.report, api.TasksUpdateParams{Title: &next})
	f.until("the next maild's write", func() bool { return len(fake.sent()) == 2 })
	close(first)
	// The last maild's write failed: that still shows.
	f.until("the first write's failure", func() bool { return len(f.toasts) == 1 })
	if got := len(f.m.tasks.writes[f.report]); got != 1 {
		t.Fatalf("the next maild's queue holds %d writes, want its own 1", got)
	}
	close(second)
	f.until("the queue emptying", func() bool { return len(f.m.tasks.writes) == 0 })
	if x := f.task(f.report); x == nil || x.Title != next {
		t.Errorf("maild holds %+v", x)
	}
}

// A create still out as the module leaves its maild answers after the
// next maild's New Task field holds the same text: the field keeps it.
func TestACreateFromTheLastMaildLeavesTheNewTaskField(t *testing.T) {
	for _, tc := range []struct {
		name   string
		module commands.Module
		focus  func(*fixture)
		text   func(*fixture) string
	}{
		{"tasks", commands.Tasks, func(f *fixture) { f.tt.Key(ui.Cmd, ui.KeyN); f.tt.Frame() }, func(f *fixture) string { return f.m.tasks.newTitle }},
		{"to-do bar", commands.Mail, func(f *fixture) { f.click("New Task") }, func(f *fixture) string { return f.m.todo.newTitle }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hold := make(chan struct{})
			fake := &fakeTasks{holdCreate: hold}
			f := newFixture(t, tc.module, withFakeTasks(fake))
			f.text("Report")
			tc.focus(f)
			f.tt.Type("Water")
			f.tt.Key(0, ui.KeyEnter)
			f.until("the create", func() bool { return fake.createCount() == 1 })
			f.m.Leave()
			f.until("the next maild's tasks", func() bool { return f.m.task(f.report) != nil })
			tc.focus(f)
			f.tt.Type("Water")
			f.tt.Frame()
			if got := tc.text(f); got != "Water" {
				t.Fatalf("the next maild's field holds %q", got)
			}
			close(hold)
			// The create's answer comes before the refetch its change brings.
			f.until("the task in the next maild's list", func() bool {
				return slices.ContainsFunc(f.m.tasks.tasks, func(x api.Task) bool { return x.Title == "Water" })
			})
			if got := tc.text(f); got != "Water" {
				t.Errorf("the last maild's create left the next maild's field %q", got)
			}
		})
	}
}

// An event window with changes, open as the module leaves its maild,
// closes without a question, and never writes its IDs to the next maild,
// where the same ID names another event.
func TestAChangedEventWindowWritesNothingToTheNextMaild(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	// Another maild, seeded alike: its IDs are the same.
	next := newFixture(t, commands.Calendar)
	f.text("Standup")
	o := f.occurrence("Standup", "2026-10-08", "2026-10-09")
	if x := next.occurrence("Standup", "2026-10-08", "2026-10-09"); x.EventID != o.EventID || x.CalendarID != o.CalendarID {
		t.Fatalf("the IDs do not collide: %+v, %+v", o, x)
	}
	f.m.selectOccurrence(&o)
	f.until("the selection", func() bool { return f.m.shownEvent() != nil })
	f.m.editSelected()
	key := eventWindowKey(o.EventID, o.RecurrenceID)
	f.until("the window", func() bool { w := f.m.windows[key]; return w != nil && w.loaded })
	w := f.m.windows[key]
	w.draft.summary = "Changed"
	if !w.dirty() {
		t.Fatal("the change does not count")
	}

	// The app moves to the next maild: Leave, then its connection.
	f.m.Leave()
	if f.windows[key] != nil {
		t.Error("the changed window refused to close")
	}
	c := next.srv.Dial(t)
	events := live.NewEvents()
	go events.Run(c)
	if _, err := c.Events().Subscribe(t.Context(), &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Connect(t.Context(), c, events); err != nil {
		t.Fatal(err)
	}
	f.until("the next maild", func() bool { return f.m.client == c })

	// Save, as the window would have had it stayed open.
	w.request(false)
	if w.busy {
		t.Error("Save sent the last maild's event to the next one")
	}
	f.until("the save settling", func() bool { return !w.busy })
	got, err := next.c.Calendar().Event(t.Context(), &api.CalendarEventParams{ID: o.EventID})
	if err != nil || got.Summary != "Standup" {
		t.Errorf("the next maild's event %d: %+v, %v", o.EventID, got, err)
	}
}
