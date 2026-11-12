package pim

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

func (f *fixture) task(id int64) *api.Task {
	f.t.Helper()
	got, err := f.c.Tasks().Get(f.t.Context(), &api.TasksGetParams{ID: id})
	if err != nil {
		return nil
	}
	return got
}

func TestTasksSourcesCountsAndRows(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	// The task lists and the accounts that head them come from calls of
	// their own, which can answer after the tasks.
	f.until("the task lists under their account", func() bool {
		return f.tt.HasText("me@a.test") && f.tt.HasText("Work Tasks") && f.tt.HasText("Errands")
	})
	for _, want := range []string{"Today", "All Tasks", "Flagged Mail", "me@a.test", "Work Tasks", "Errands", "No Task Selected", "Today", "Q3 numbers"} {
		if !f.tt.HasText(want) {
			t.Errorf("no %q in %s", want, dump(f.tt))
		}
	}
	if f.tt.HasText("Charts") {
		t.Error("Today shows a task with no due date")
	}
	if !f.tt.HasText("Buy oat milk") || !f.tt.HasText("Yesterday") {
		t.Errorf("no overdue task: %s", dump(f.tt))
	}
	snapshot(t, f.tt, "tasks")
	f.m.setTaskSource(sourceAll)
	f.until("All Tasks", func() bool { return f.tt.HasText("Charts") })
	if f.tt.HasText("Filed") {
		t.Error("a completed task shows")
	}
	f.click("Show Completed")
	f.until("the completed", func() bool { return f.tt.HasText("Filed") })
	f.m.setTaskSource(taskSource(f.tasks))
	f.until("the list", func() bool { return f.tt.HasText("Work Tasks") && !f.tt.HasText("Buy oat milk") })
	if !f.tt.HasText("Filed") {
		t.Error("Show Completed does not carry to a list")
	}
}

func TestCircleTogglesWithoutSelecting(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.click("Complete Report")
	f.until("the write", func() bool { x := f.task(f.report); return x != nil && x.Completed })
	if f.m.tasks.selected != 0 {
		t.Error("the circle selected the row")
	}
	if !f.tt.HasText("Report") {
		t.Error("a task ticked here left before the source changed")
	}
	f.m.setTaskSource(sourceAll)
	f.tt.Frame()
	f.m.setTaskSource(sourceToday)
	f.until("gone", func() bool { return !f.tt.HasText("Report") })
}

func TestSpaceTogglesAndKeysMove(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.click("Q3 numbers")
	f.until("selected", func() bool { return f.m.tasks.selected == f.report })
	if f.m.tasks.focus != paneTasks {
		t.Errorf("focus %q", f.m.tasks.focus)
	}
	f.tt.Key(0, ui.KeySpace)
	f.until("the write", func() bool { x := f.task(f.report); return x != nil && x.Completed })
	f.tt.Key(0, ui.KeyDown)
	f.until("moved", func() bool { return f.m.tasks.selected == f.milk })
	f.tt.Key(0, ui.KeyEscape)
	f.until("cleared", func() bool { return f.m.tasks.selected == 0 })
}

func TestNewTaskCreatesAndKeepsFocus(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.tt.Key(ui.Cmd, ui.KeyN)
	f.tt.Frame()
	f.tt.Type("  Water the plants ")
	f.tt.Key(0, ui.KeyEnter)
	f.until("the task", func() bool { return f.tt.HasText("Water the plants") })
	if f.m.tasks.newTitle != "" {
		t.Error("the field did not empty")
	}
	if !f.tt.Focused("New Task") {
		t.Error("the field lost the focus")
	}
	f.tt.Type("Mistake")
	f.tt.Key(0, ui.KeyEscape)
	f.tt.Frame()
	if f.m.tasks.newTitle != "" {
		t.Error("Escape did not empty the field")
	}
	all, err := f.c.Tasks().List(t.Context(), &api.TasksListParams{})
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(x api.Task) bool { return x.Title == "Water the plants" })
	if i < 0 || all[i].Due != "2026-10-08" {
		t.Errorf("Today's new task: %+v", all)
	}
}

func TestReadOnlyListHasNoNewTaskAndCircleDoesNothing(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.m.setTaskSource(taskSource(f.readTasks))
	f.until("the list", func() bool { return f.tt.HasText("Read Only Item") && f.m.taskList(f.readTasks) != nil })
	f.tt.Frame()
	if _, ok := f.tt.AccessByRole(ui.RoleTextField, "New Task"); ok {
		t.Error("a read-only list has a New Task field")
	}
	i := slices.IndexFunc(f.m.tasks.tasks, func(x api.Task) bool { return x.Title == "Read Only Item" })
	f.m.toggleTask(f.m.tasks.tasks[i].ID, true)
	if f.m.tasks.tasks[i].Completed {
		t.Error("a read-only task ticked")
	}
	f.click("Read Only Item")
	f.until("the pane", func() bool { return f.tt.HasText("Read-only") })
	if f.tt.HasText("Delete Task") {
		t.Error("a read-only task can be deleted")
	}
}

func TestTaskPaneEditsDueNotesAndDeletes(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.click("Q3 numbers")
	f.until("the pane", func() bool { return f.tt.HasText("Delete Task") })
	if !f.tt.HasText("Clear Due Date") {
		t.Errorf("no Clear: %s", dump(f.tt))
	}
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Frame()
	if !f.tt.Focused("Title") {
		t.Error("Enter did not focus the title")
	}
	f.tt.Type(" v2")
	f.tt.Key(0, ui.KeyEnter)
	f.until("the title", func() bool { x := f.task(f.report); return x != nil && x.Title == "Report v2" })
	f.click("Clear Due Date")
	f.until("cleared", func() bool { x := f.task(f.report); return x != nil && x.Due == "" })
	// With no due date it leaves Today: go to its list.
	f.m.setTaskSource(taskSource(f.tasks))
	f.tt.Frame()
	f.m.tasks.selected = f.report
	f.until("the pane", func() bool { return f.tt.HasText("Delete Task") })
	f.click("Delete Task")
	f.until("deleted", func() bool { return f.task(f.report) == nil })
	if f.task(f.charts) != nil {
		t.Error("the subtask stayed")
	}
}

// TestTaskPaneKeepsTheListName is the guard for frostyard/frostmail#282: a
// long account address gives way before the list's name, which keeps its
// whole width.
func TestTaskPaneKeepsTheListName(t *testing.T) {
	f := newFixture(t, commands.Tasks, options{email: longAddress})
	f.text("Report")
	f.click("Q3 numbers")
	f.until("the pane", func() bool { return f.tt.HasText("Delete Task") })
	if !f.tt.HasText(longAddress) {
		t.Fatalf("the List row shows no account: %s", dump(f.tt))
	}
	got, ok := f.detailText("Task details", "Work Tasks")
	if !ok {
		t.Fatalf("the task pane shows no list named Work Tasks: %s", dump(f.tt))
	}
	if want := textWidth(t, "Work Tasks"); got.W < want-0.5 {
		t.Errorf("the list's name is %v wide, and %v drawn alone: the account's address cut it", got.W, want)
	}
}

func TestEscapeInTitleRevertsAndEmptyTitleIsKept(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.click("Q3 numbers")
	f.until("the pane", func() bool { return f.tt.HasText("Delete Task") })
	f.m.tasks.focusTitle = true
	f.tt.Frame()
	f.m.tasks.paneTitle = ""
	f.tt.Frame()
	f.tt.Key(0, ui.KeyEnter)
	f.until("back", func() bool { return f.m.tasks.paneTitle == "Report" })
	f.m.tasks.focusTitle = true
	f.tt.Frame()
	f.tt.Type("zz")
	f.tt.Key(0, ui.KeyEscape)
	f.until("reverted", func() bool { return f.m.tasks.paneTitle == "Report" })
	if x := f.task(f.report); x.Title != "Report" {
		t.Errorf("wrote %q", x.Title)
	}
}

func (f *fixture) seedFlagged(n int) (ids []int64) {
	f.t.Helper()
	ctx := f.t.Context()
	err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, f.acct, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		hs := make([]store.MessageHeader, 0, n)
		for i := 1; i <= n; i++ {
			d := time.Date(2026, 10, 7, 9, i, 0, 0, time.UTC)
			hs = append(hs, store.MessageHeader{UID: uint32(i), Subject: "Flagged " + string(rune('A'+i-1)), MessageID: string(rune('A'+i-1)) + "@x",
				From: store.Address{Name: "Ann", Addr: "ann@x.test"}, Date: d, InternalDate: d, Flags: store.Flags{Flagged: true, Color: 2}})
		}
		_, err = tx.InsertHeaders(ctx, f.acct, mbs[0].ID, hs)
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return nil
}

// fakeMessages applies flags in the store, as sync does.
type fakeMessages struct {
	api.MessageService
	srv **rpctest.Server
}

func (s fakeMessages) SetFlags(ctx context.Context, p *api.MessageSetFlagsParams) error {
	change := store.FlagChange{Seen: p.Changes.Seen, Flagged: p.Changes.Flagged}
	if p.Changes.FlagColor != nil {
		color := int(*p.Changes.FlagColor)
		change.Color = &color
	}
	return (*s.srv).DB.Tx(ctx, func(tx *store.Tx) error {
		changed, err := tx.ChangeFlags(ctx, p.IDs, change)
		if err != nil {
			return err
		}
		return tx.Emit(ctx, api.MessageChanged{IDs: changed})
	})
}

func withFakeMessages() options {
	var srv *rpctest.Server
	o := options{Options: rpctest.Options{Services: func(s *api.Services) { s.Message = fakeMessages{s.Message, &srv} }}}
	o.onStart = func(s *rpctest.Server) { srv = s }
	return o
}

func TestFlaggedMailListClearsAndOpens(t *testing.T) {
	f := newFixture(t, commands.Tasks, withFakeMessages())
	f.seedFlagged(2)
	f.text("Report")
	f.m.setTaskSource(sourceFlagged)
	f.until("the row", func() bool { return f.tt.HasText("Flagged A") })
	if f.tt.HasText("Show Completed") {
		t.Error("Flagged Mail shows Show Completed")
	}
	f.click("Flagged B")
	f.until("the pane", func() bool { return f.tt.HasText("Open in Mail") })
	id := f.m.tasks.selected
	f.tt.Key(0, ui.KeyEnter)
	f.until("opened", func() bool { return slices.Contains(f.opened, id) })
	f.click("Clear Flag")
	f.until("the row leaving", func() bool { return !f.tt.HasText("Flagged B") })
	f.m.tasks.selected = 0
	f.click("Clear Flag Flagged A")
	f.until("empty", func() bool { return f.tt.HasText("No Flagged Mail") })
}

func TestToDoBar(t *testing.T) {
	f := newFixture(t, commands.Mail, withFakeMessages())
	f.seedFlagged(7)
	f.text("Report")
	for _, want := range []string{"Upcoming", "Tasks", "Review", "Sync", "Gym", "Buy oat milk", "Flagged G", "New Task"} {
		f.until(want, func() bool { return f.tt.HasText(want) })
	}
	if f.tt.HasText("Flagged A") {
		t.Error("more than 5 flagged rows")
	}
	if f.tt.HasText("Standup") {
		t.Error("an ended occurrence shows")
	}
	snapshot(t, f.tt, "todobar")
	f.click("Friday, October 9, 2026")
	f.until("Calendar", func() bool { return slices.Contains(f.shown, commands.Calendar) && f.m.cal.date == "2026-10-09" })
	f.module = commands.Mail
	f.tt.Frame()
	f.click("Buy oat milk")
	f.until("Tasks", func() bool {
		return slices.Contains(f.shown, commands.Tasks) && f.m.tasks.selected == f.milk && f.m.tasks.source == taskSource(f.errands)
	})
	f.module = commands.Mail
	f.tt.Frame()
	f.click("Complete Report")
	f.until("the write", func() bool { x := f.task(f.report); return x != nil && x.Completed })
	f.click("Flagged G")
	f.until("opened", func() bool { return len(f.opened) == 1 })
	f.click("Clear Flag Flagged G")
	f.until("the row leaving", func() bool { return !f.tt.HasText("Flagged G") })
}

func TestToDoBarAtMostFiveUpcoming(t *testing.T) {
	f := newFixture(t, commands.Mail)
	f.text("Report")
	for i := range 8 {
		start := testNow.Add(time.Duration(i+1) * 24 * time.Hour)
		summary := "Later " + string(rune('A'+i))
		if _, err := f.c.Calendar().Create(t.Context(), &api.CalendarCreateParams{CalendarID: &f.work, Fields: api.EventFields{Summary: &summary, AllDay: ptr(false), Start: &start, End: ptr(start.Add(time.Hour)), TimeZone: ptr("UTC")}}); err != nil {
			t.Fatal(err)
		}
	}
	f.until("refetch", func() bool { return len(f.m.todo.upcoming.value) > 5 })
	f.tt.Frame()
	if got := len(nextOccurrences(f.m.todo.upcoming.value, f.now, time.UTC, 5)); got != 5 {
		t.Errorf("%d upcoming", got)
	}
	f.now = f.now.Add(time.Hour)
	f.tt.Frame()
}

func TestToDoBarNothingDue(t *testing.T) {
	f := newFixture(t, commands.Mail, options{empty: true})
	f.until("empty", func() bool { return f.tt.HasText("Nothing Due") && f.tt.HasText("No Upcoming Events") })
}

func TestTabCyclesTheTaskPanes(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.tt.Key(0, ui.KeyTab)
	f.tt.Frame()
	if f.m.tasks.focus != paneTask {
		t.Errorf("Tab went to %q", f.m.tasks.focus)
	}
}

func TestDeleteSelectsTheNextRow(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.click("Q3 numbers")
	f.until("selected", func() bool { return f.m.tasks.selected == f.report })
	f.tt.Key(0, ui.KeyDelete)
	f.until("deleted", func() bool { return f.task(f.report) == nil })
	if f.m.tasks.selected != f.milk {
		t.Errorf("selected %d, want %d", f.m.tasks.selected, f.milk)
	}
}

func TestAFailedToggleRefetches(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.m.toggleTask(f.report, true)
	f.until("the write", func() bool { x := f.task(f.report); return x != nil && x.Completed })
	if !f.m.task(f.report).Completed {
		t.Error("not shown at once")
	}
	// An unknown task's write fails and the list asks again.
	f.m.tasks.tasks = append(slices.Clone(f.m.tasks.tasks), api.Task{ID: 9999, ListID: f.tasks, Title: "Ghost"})
	f.m.toggleTask(9999, true)
	f.until("the refetch", func() bool { return f.m.task(9999) == nil && len(f.toasts) == 1 })
}

func TestToDoBarNewTaskGoesToTheDefaultList(t *testing.T) {
	f := newFixture(t, commands.Mail)
	f.text("Report")
	f.click("New Task")
	f.tt.Type("From the bar")
	f.tt.Key(0, ui.KeyEnter)
	f.until("the task", func() bool {
		all, err := f.c.Tasks().List(t.Context(), &api.TasksListParams{})
		if err != nil {
			t.Fatal(err)
		}
		return slices.ContainsFunc(all, func(x api.Task) bool { return x.Title == "From the bar" })
	})
}

// fakeTasks passes calls on to maild, noting them; a test can fail or
// hold some.
type fakeTasks struct {
	api.TasksService
	mu         sync.Mutex
	updates    []api.TasksUpdateParams
	holds      []chan struct{} // the nth Update waits for the nth, when set
	updateErrs []error         // the nth Update fails with the nth, when set
	creates    int
	createErr  error         // the next Create fails with it
	holdCreate chan struct{} // Create waits for it, when set
}

func (s *fakeTasks) Update(ctx context.Context, p *api.TasksUpdateParams) (*api.Task, error) {
	s.mu.Lock()
	s.updates = append(s.updates, *p)
	n := len(s.updates)
	var hold chan struct{}
	var err error
	if n <= len(s.holds) {
		hold = s.holds[n-1]
	}
	if n <= len(s.updateErrs) {
		err = s.updateErrs[n-1]
	}
	s.mu.Unlock()
	if hold != nil {
		<-hold
	}
	if err != nil {
		return nil, err
	}
	return s.TasksService.Update(ctx, p)
}

func (s *fakeTasks) Create(ctx context.Context, p *api.TasksCreateParams) (*api.Task, error) {
	s.mu.Lock()
	s.creates++
	err, hold := s.createErr, s.holdCreate
	s.createErr = nil
	s.mu.Unlock()
	if hold != nil {
		<-hold
	}
	if err != nil {
		return nil, err
	}
	return s.TasksService.Create(ctx, p)
}

func (s *fakeTasks) sent() []api.TasksUpdateParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.updates)
}

func (s *fakeTasks) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

func withFakeTasks(fake *fakeTasks) options {
	return options{Options: rpctest.Options{Services: func(s *api.Services) {
		fake.TasksService = s.Tasks
		s.Tasks = fake
	}}}
}

func TestLeavingATaskCommitsItsEdits(t *testing.T) {
	title := func(f *fixture) {
		f.m.tasks.focusTitle = true
		f.tt.Frame()
	}
	notes := func(f *fixture) {
		r, ok := f.tt.Find("Notes")
		if !ok {
			f.t.Fatalf("no Notes in %s", dump(f.tt))
		}
		f.tt.ClickAt(r.X+r.W/2, r.Y+r.H+40)
		f.tt.Frame()
	}
	titled := func(x *api.Task) bool { return strings.Contains(x.Title, "zz") }
	noted := func(x *api.Task) bool { return strings.Contains(x.Notes, "zz") }
	another := func(f *fixture) { f.click("Buy oat milk") }
	source := func(f *fixture) { f.click("All Tasks") }
	flagged := func(f *fixture) { f.m.setTaskSource(sourceFlagged) }
	// Another device moves the task out of Today: the selection ends with
	// the field still focused.
	moved := func(f *fixture) {
		if _, err := f.c.Tasks().Update(f.t.Context(), &api.TasksUpdateParams{ID: f.report, Due: ptr("2026-10-20")}); err != nil {
			f.t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		focus func(*fixture)
		leave func(*fixture)
		saved func(*api.Task) bool
	}{
		{"title for another task", title, another, titled},
		{"notes for another task", notes, another, noted},
		{"title for another source", title, source, titled},
		{"notes for another source", notes, source, noted},
		{"title for Flagged Mail", title, flagged, titled},
		{"title as the task leaves the view", title, moved, titled},
		{"notes as the task leaves the view", notes, moved, noted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, commands.Tasks)
			f.text("Report")
			f.click("Q3 numbers")
			f.until("the pane", func() bool { return f.tt.HasText("Delete Task") && f.m.tasks.paneID == f.report })
			tc.focus(f)
			f.tt.Type("zz")
			f.tt.Frame()
			tc.leave(f)
			f.until("the write", func() bool { x := f.task(f.report); return x != nil && tc.saved(x) })
			f.until("the pane leaving", func() bool { return f.m.tasks.paneID != f.report })
			if x := f.task(f.milk); x.Title != "Buy oat milk" || x.Notes != "" {
				t.Errorf("the next task took the edit: %+v", x)
			}
		})
	}
}

func TestLeavingATaskWritesNoStaleTitle(t *testing.T) {
	fake := &fakeTasks{}
	f := newFixture(t, commands.Tasks, withFakeTasks(fake))
	f.text("Report")
	f.click("Q3 numbers")
	f.until("the pane", func() bool { return f.m.tasks.paneID == f.report })
	// A refetch brings a newer title as the selection moves: the field
	// holds the old one, which the user did not type.
	f.m.tasks.tasks = slices.Clone(f.m.tasks.tasks)
	i := slices.IndexFunc(f.m.tasks.tasks, func(x api.Task) bool { return x.ID == f.report })
	f.m.tasks.tasks[i].Title = "Newer"
	f.m.tasks.selected = f.milk
	f.until("the pane", func() bool { return f.m.tasks.paneID == f.milk })
	if got := fake.sent(); len(got) != 0 {
		t.Errorf("wrote %+v", got)
	}
}

func TestWritesToOneTaskRunInOrder(t *testing.T) {
	hold := make(chan struct{})
	fake := &fakeTasks{holds: []chan struct{}{hold}}
	f := newFixture(t, commands.Tasks, withFakeTasks(fake))
	f.text("Report")
	first, second, milk := "First", "Second", "Milk"
	f.m.editTask(f.report, api.TasksUpdateParams{Title: &first})
	f.m.editTask(f.report, api.TasksUpdateParams{Title: &second})
	f.m.toggleTask(f.report, true)
	f.until("the first write", func() bool { return len(fake.sent()) == 1 })
	// Another task does not wait.
	f.m.editTask(f.milk, api.TasksUpdateParams{Title: &milk})
	f.until("the other task", func() bool { x := f.task(f.milk); return x != nil && x.Title == milk })
	if got := fake.sent(); len(got) != 2 || got[1].ID != f.milk {
		t.Fatalf("while the first write ran, sent %+v", got)
	}
	close(hold)
	f.until("every write", func() bool { return len(f.m.tasks.writes) == 0 })
	var titles []string
	var last api.TasksUpdateParams
	for _, p := range fake.sent() {
		if p.ID != f.report {
			continue
		}
		last = p
		if p.Title != nil {
			titles = append(titles, *p.Title)
		}
	}
	if !slices.Equal(titles, []string{first, second}) || last.Completed == nil {
		t.Errorf("titles %v, last %+v", titles, last)
	}
	if x := f.task(f.report); x.Title != second || !x.Completed {
		t.Errorf("maild holds %+v", x)
	}
}

func TestATaskWriteWhileDisconnectedSaysSo(t *testing.T) {
	f := newFixture(t, commands.Tasks)
	f.text("Report")
	f.m.Disconnect()
	f.until("the disconnect", func() bool { return f.m.client == nil })
	f.m.toggleTask(f.report, true)
	if len(f.toasts) != 1 || len(f.m.tasks.writes) != 0 {
		t.Errorf("toasts %q, writes %v", f.toasts, f.m.tasks.writes)
	}
}

func TestNewTaskKeepsItsTextUntilCreated(t *testing.T) {
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
			fake := &fakeTasks{createErr: errors.New("refused"), holdCreate: hold}
			f := newFixture(t, tc.module, withFakeTasks(fake))
			f.text("Report")
			tc.focus(f)
			f.tt.Type("Water")
			f.tt.Key(0, ui.KeyEnter)
			f.until("the create", func() bool { return fake.createCount() == 1 })
			if got := tc.text(f); got != "Water" {
				t.Errorf("while the create ran, the field holds %q", got)
			}
			f.tt.Key(0, ui.KeyEnter)
			f.tt.Frame()
			close(hold)
			f.until("the failure", func() bool { return len(f.toasts) == 1 })
			if got := tc.text(f); got != "Water" {
				t.Errorf("after the failure, the field holds %q", got)
			}
			if got := fake.createCount(); got != 1 {
				t.Errorf("Enter during a create sent %d creates", got)
			}
			f.tt.Key(0, ui.KeyEnter)
			f.until("the field emptying", func() bool { return tc.text(f) == "" })
			all, err := f.c.Tasks().List(t.Context(), &api.TasksListParams{})
			if err != nil {
				t.Fatal(err)
			}
			if n := len(slices.DeleteFunc(all, func(x api.Task) bool { return x.Title != "Water" })); n != 1 {
				t.Errorf("%d tasks named Water", n)
			}
		})
	}
}

// The next write to a task waits for maild's answer to the last, however
// long it takes: a write the app stopped waiting for could still land in
// maild after the next one and undo it.
func TestATaskWriteWaitsForTheLastAnswer(t *testing.T) {
	hold := make(chan struct{})
	fake := &fakeTasks{holds: []chan struct{}{hold}}
	f := newFixture(t, commands.Tasks, withFakeTasks(fake))
	f.text("Report")
	first, second := "First", "Second"
	deadline := make(chan bool, 1)
	f.m.writeTask(f.report, "change the task", func(ctx context.Context, c *api.Client) error {
		_, has := ctx.Deadline()
		deadline <- has
		_, err := c.Tasks().Update(ctx, &api.TasksUpdateParams{ID: f.report, Title: &first})
		return err
	})
	f.m.editTask(f.report, api.TasksUpdateParams{Title: &second})
	f.until("the first write", func() bool { return len(fake.sent()) == 1 })
	if <-deadline {
		t.Error("the write has a deadline of the app's own: past it, the next write goes while maild still runs this one")
	}
	f.tt.Frame()
	if got := fake.sent(); len(got) != 1 {
		t.Fatalf("while the first write ran, sent %+v", got)
	}
	close(hold)
	f.until("both writes", func() bool { return len(f.m.tasks.writes) == 0 })
	if got := fake.sent(); len(got) != 2 || got[0].Title == nil || *got[0].Title != first || got[1].Title == nil || *got[1].Title != second {
		t.Errorf("sent %+v", got)
	}
	if x := f.task(f.report); x == nil || x.Title != second {
		t.Errorf("maild holds %+v", x)
	}
}

// An edit and a change of the selection or the source in one frame, as an
// input method's commit and a click: the field takes the edit before the
// pane moves on, so it is saved to the task it was typed into, and the
// next task's fields never hold it.
func TestAnEditInTheFrameOfASwitchStaysWithItsTask(t *testing.T) {
	title := func(f *fixture) {
		f.m.tasks.focusTitle = true
		f.tt.Frame()
	}
	notes := func(f *fixture) {
		r, ok := f.tt.Find("Notes")
		if !ok {
			f.t.Fatalf("no Notes in %s", dump(f.tt))
		}
		f.tt.ClickAt(r.X+r.W/2, r.Y+r.H+40)
		f.tt.Frame()
	}
	titled := func(x *api.Task) bool { return strings.Contains(x.Title, "zz") }
	noted := func(x *api.Task) bool { return strings.Contains(x.Notes, "zz") }
	another := func(f *fixture) { f.m.tasks.selected = f.milk }
	source := func(f *fixture) { f.m.setTaskSource(sourceAll) }
	for _, tc := range []struct {
		name  string
		focus func(*fixture)
		leave func(*fixture)
		saved func(*api.Task) bool
		next  int64
	}{
		{"title for another task", title, another, titled, 0},
		{"notes for another task", notes, another, noted, 0},
		{"title for another source", title, source, titled, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, commands.Tasks)
			f.text("Report")
			f.click("Q3 numbers")
			f.until("the pane", func() bool { return f.tt.HasText("Delete Task") && f.m.tasks.paneID == f.report })
			tc.focus(f)
			// The switch comes in the frame that brings the typed text.
			f.loop.Post(func() { tc.leave(f) })
			f.tt.Type("zz")
			f.until("the write", func() bool { x := f.task(f.report); return x != nil && tc.saved(x) })
			next := f.milk
			if tc.next < 0 {
				next = 0
			}
			f.until("the next pane", func() bool { return f.m.tasks.paneID == next && len(f.m.tasks.writes) == 0 })
			if next != 0 && (f.m.tasks.paneTitle != "Buy oat milk" || f.m.tasks.paneNotes != "") {
				t.Errorf("the next task's fields hold %q, %q", f.m.tasks.paneTitle, f.m.tasks.paneNotes)
			}
			if x := f.task(f.milk); x.Title != "Buy oat milk" || x.Notes != "" {
				t.Errorf("the next task took the edit: %+v", x)
			}
		})
	}
}
