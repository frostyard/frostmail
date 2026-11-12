package pim

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

const flaggedRows = 200

var errNotConnected = errors.New("not connected")

// taskWrite is one write waiting its turn on a task.
type taskWrite struct {
	client *api.Client
	what   string
	call   func(ctx context.Context, c *api.Client) error
}

// tasksState is the Tasks module's state.
type tasksState struct {
	source        taskSource
	selected      int64
	focus         string
	focusAsk      string
	showCompleted bool
	ticked        map[int64]bool
	lists         fetch[[]api.Collection]
	all           fetch[[]api.Task]
	allAt         int
	tasks         []api.Task
	flagged       *live.View
	newTitle      string
	creating      bool
	writes        map[int64][]taskWrite // each task's writes; the first is running
	focusNew      bool
	focusTitle    bool
	sources       frostui.SourceList[string]
	listUI        ui.ListState
	row           int
	lines         []taskLine
	flaggedShown  []api.MessageSummary

	paneID    int64
	paneTitle string
	paneNotes string
	// What the fields last took from the task or sent to it: a field that
	// differs from these holds the user's edit.
	paneTitleWas string
	paneNotesWas string
	dueT         time.Time
	dueOpen      bool
	skipCommit   bool
}

func (m *Module) setUpTasks() {
	st := &m.tasks
	st.source, st.row, st.focus, st.ticked = sourceToday, -1, paneTasks, map[int64]bool{}
	st.writes = map[int64][]taskWrite{}
	st.sources.Label = "Task Lists"
	st.sources.Row = func(key string) frostui.SourceRow { return m.taskSourceRow(key) }
	st.sources.OnSelect = func(key string) { m.setTaskSource(sourceOfKey(key)) }
}

func (m *Module) tasksChanged() { m.tasks.all.dirty = true }

func sourceKey(s taskSource) string {
	switch s {
	case sourceToday:
		return "today"
	case sourceAll:
		return "all"
	case sourceFlagged:
		return "flagged"
	}
	return "list:" + strconv.FormatInt(int64(s), 10)
}

func sourceOfKey(key string) taskSource {
	switch key {
	case "today":
		return sourceToday
	case "all":
		return sourceAll
	case "flagged":
		return sourceFlagged
	}
	n, _ := strconv.ParseInt(strings.TrimPrefix(key, "list:"), 10, 64)
	return taskSource(n)
}

func (m *Module) setTaskSource(s taskSource) {
	st := &m.tasks
	if st.source == s {
		return
	}
	st.source, st.selected, st.ticked = s, 0, map[int64]bool{}
}

func (m *Module) taskList(id int64) *api.Collection {
	for i := range m.tasks.lists.value {
		if m.tasks.lists.value[i].ID == id {
			return &m.tasks.lists.value[i]
		}
	}
	return nil
}

func (m *Module) listReadOnly(id int64) bool {
	l := m.taskList(id)
	return l != nil && (l.ReadOnly || m.accountReadOnly(l.AccountID))
}

func (m *Module) closeFlagged() {
	if m.tasks.flagged != nil {
		m.tasks.flagged.Close()
		m.tasks.flagged = nil
	}
}

// syncTasks asks for every task and the flagged view, which the module
// and the To-Do Bar share.
func (m *Module) syncTasks() {
	st := &m.tasks
	want(m, &st.lists, "lists", func(ctx context.Context, c *api.Client) ([]api.Collection, error) {
		kind := api.CollectionKindTasklist
		all, err := c.Account().Collections(ctx, &api.AccountCollectionsParams{Kind: &kind})
		return slices.DeleteFunc(all, func(l api.Collection) bool { return !l.Enabled }), err
	})
	want(m, &st.all, "tasks", func(ctx context.Context, c *api.Client) ([]api.Task, error) {
		return c.Tasks().List(ctx, &api.TasksListParams{Completed: ptr(true)})
	})
	if st.allAt != st.all.version {
		st.allAt = st.all.version
		st.tasks = st.all.value
	}
	if st.flagged == nil && m.client != nil && m.events != nil {
		st.flagged = live.OpenView(m.client, m.events, api.ViewQuery{Flagged: ptr(true)}, m.o.Wake)
	}
	st.flaggedShown = st.flaggedShown[:0]
	if v := st.flagged; v != nil {
		n := min(v.Count(), flaggedRows)
		v.Ensure(0, flaggedRows)
		for i := range n {
			if r, ok := v.Row(i); ok {
				st.flaggedShown = append(st.flaggedShown, r)
			}
		}
	}
}

func (m *Module) shownTasks() []api.Task {
	st := &m.tasks
	return sourceTasks(st.tasks, st.source, m.today(), st.showCompleted, st.ticked)
}

func (m *Module) taskSourceRow(key string) frostui.SourceRow {
	st := &m.tasks
	today := m.today()
	count := func(n int) string {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	switch key {
	case "today":
		return frostui.SourceRow{Title: "Today", Icon: "x-office-calendar", Count: count(openCount(st.tasks, sourceToday, today))}
	case "all":
		return frostui.SourceRow{Title: "All Tasks", Icon: "view-list", Count: count(openCount(st.tasks, sourceAll, today))}
	case "flagged":
		n := 0
		if st.flagged != nil {
			n = int(st.flagged.Count())
		}
		return frostui.SourceRow{Title: "Flagged Mail", Icon: "mail-mark-important", Count: count(n)}
	}
	s := sourceOfKey(key)
	l := m.taskList(int64(s))
	if l == nil {
		return frostui.SourceRow{}
	}
	row := frostui.SourceRow{Title: l.Name, Icon: "view-list", Count: count(openCount(st.tasks, s, today))}
	if m.listReadOnly(l.ID) {
		row.Tag, row.Spoken = "🔒", "Read-only"
	}
	return row
}

func (m *Module) tasksTitle() string {
	st := &m.tasks
	switch st.source {
	case sourceToday:
		return "Today"
	case sourceAll:
		return "All Tasks"
	case sourceFlagged:
		return "Flagged Mail"
	}
	if l := m.taskList(int64(st.source)); l != nil {
		return l.Name
	}
	return ""
}

func (m *Module) canCreateTask() bool {
	st := &m.tasks
	return st.source != sourceFlagged && (st.source < 0 || !m.listReadOnly(int64(st.source)))
}

func (m *Module) tasksHandlers() map[commands.Command]func() {
	st := &m.tasks
	list := func(f func()) func() {
		return func() {
			if st.focus == paneTasks {
				f()
			}
		}
	}
	return map[commands.Command]func(){
		commands.Compose:      func() { st.focusNew = m.canCreateTask() },
		commands.PageDown:     list(m.toggleSelected),
		commands.Delete:       list(m.deleteSelectedTask),
		commands.Open:         list(m.openSelectedTask),
		commands.Escape:       func() { st.selected = 0 },
		commands.Previous:     list(func() { m.moveTask(-1) }),
		commands.Next:         list(func() { m.moveTask(1) }),
		commands.First:        list(func() { m.moveTaskTo(0) }),
		commands.Last:         list(func() { m.moveTaskTo(1 << 30) }),
		commands.NextPane:     func() { m.cycleTasks(1) },
		commands.PreviousPane: func() { m.cycleTasks(-1) },
	}
}

func (m *Module) rowIDs() []int64 {
	st := &m.tasks
	var ids []int64
	if st.source == sourceFlagged {
		for _, r := range st.flaggedShown {
			ids = append(ids, r.ID)
		}
		return ids
	}
	for _, t := range m.shownTasks() {
		ids = append(ids, t.ID)
	}
	return ids
}

func (m *Module) moveTask(step int) {
	ids := m.rowIDs()
	if len(ids) == 0 {
		return
	}
	at := slices.Index(ids, m.tasks.selected)
	m.tasks.selected = ids[min(max(at+step, 0), len(ids)-1)]
}

func (m *Module) moveTaskTo(index int) {
	ids := m.rowIDs()
	if len(ids) > 0 {
		m.tasks.selected = ids[min(index, len(ids)-1)]
	}
}

func (m *Module) cycleTasks(step int) {
	st := &m.tasks
	panes := []string{paneSidebar, paneTasks, paneTask}
	i := max(slices.Index(panes, st.focus), 0)
	st.focusAsk = panes[(i+step+len(panes))%len(panes)]
	st.focus = st.focusAsk
}

func (m *Module) task(id int64) *api.Task {
	st := &m.tasks
	for i := range st.tasks {
		if st.tasks[i].ID == id {
			return &st.tasks[i]
		}
	}
	return nil
}

func (m *Module) taskWritable(t *api.Task) bool { return !t.ReadOnly && !m.listReadOnly(t.ListID) }

func (m *Module) toggleSelected() {
	st := &m.tasks
	if st.selected == 0 {
		return
	}
	if st.source == sourceFlagged {
		m.clearFlag(st.selected)
		return
	}
	if t := m.task(st.selected); t != nil {
		m.toggleTask(t.ID, !t.Completed)
	}
}

// toggleTask ticks a task and shows it at once; a failure refetches.
func (m *Module) toggleTask(id int64, completed bool) {
	st := &m.tasks
	t := m.task(id)
	if t == nil || !m.taskWritable(t) {
		return
	}
	st.all.gen++
	st.tasks = slices.Clone(st.tasks)
	st.tasks[slices.IndexFunc(st.tasks, func(x api.Task) bool { return x.ID == id })].Completed = completed
	if completed {
		st.ticked[id] = true
	}
	m.writeTask(id, "change the task", func(ctx context.Context, c *api.Client) error {
		_, err := c.Tasks().Update(ctx, &api.TasksUpdateParams{ID: id, Completed: &completed})
		return err
	})
}

// writeTask sends a write of a task after those already asked of it, so
// maild takes them in the user's order. A failure shows as a toast and
// refetches. Each write keeps the connection it was asked on: task IDs
// mean nothing to another daemon.
//
// The next write waits for maild's answer to the last, with no deadline of
// the app's own: a write the app stopped waiting for could still land
// after the next one and undo it. The answer comes, or the connection
// ends, and maild cancels the request's context with it.
func (m *Module) writeTask(id int64, what string, call func(ctx context.Context, c *api.Client) error) {
	st := &m.tasks
	if m.client == nil {
		m.o.Toast("Could not "+what, errNotConnected)
		st.all.dirty = true
		return
	}
	st.writes[id] = append(st.writes[id], taskWrite{m.client, what, call})
	if len(st.writes[id]) == 1 {
		m.nextTaskWrite(id)
	}
}

// nextTaskWrite runs the first write waiting on a task. An answer from a
// maild the module has left only reports a failure: that maild's queue
// went with it, and the same ID names another task now.
func (m *Module) nextTaskWrite(id int64) {
	st := &m.tasks
	w := st.writes[id][0]
	daemon := m.daemon
	go func() {
		err := w.call(context.Background(), w.client)
		m.o.Post(func() {
			if m.daemon != daemon {
				if err != nil {
					slog.Warn(w.what, "err", err)
					m.o.Toast("Could not "+w.what, err)
				}
				return
			}
			if err != nil {
				slog.Warn(w.what, "err", err)
				m.o.Toast("Could not "+w.what, err)
				st.all.dirty = true
			}
			st.writes[id] = st.writes[id][1:]
			if len(st.writes[id]) == 0 {
				delete(st.writes, id)
				return
			}
			m.nextTaskWrite(id)
		})
	}()
}

func (m *Module) clearFlag(id int64) {
	m.run("clear the flag", func(ctx context.Context, c *api.Client) error {
		return c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{id}, Changes: api.FlagChanges{FlagColor: ptr(int64(0))}})
	}, nil)
}

func (m *Module) deleteSelectedTask() {
	st := &m.tasks
	if st.source == sourceFlagged || st.selected == 0 {
		return
	}
	id := st.selected
	t := m.task(id)
	if t == nil || !m.taskWritable(t) {
		return
	}
	gone := map[int64]bool{id: true}
	for _, x := range st.tasks {
		if x.ParentID != nil && *x.ParentID == id {
			gone[x.ID] = true
		}
	}
	shown := m.shownTasks()
	index := slices.IndexFunc(shown, func(x api.Task) bool { return x.ID == id })
	remaining := slices.DeleteFunc(slices.Clone(shown), func(x api.Task) bool { return gone[x.ID] })
	st.selected = 0
	if index >= 0 && index < len(remaining) {
		st.selected = remaining[index].ID
	} else if index-1 >= 0 && index-1 < len(remaining) {
		st.selected = remaining[index-1].ID
	}
	st.all.gen++
	st.tasks = slices.DeleteFunc(slices.Clone(st.tasks), func(x api.Task) bool { return gone[x.ID] })
	m.writeTask(id, "delete the task", func(ctx context.Context, c *api.Client) error {
		return c.Tasks().Delete(ctx, &api.TasksDeleteParams{ID: id})
	})
}

func (m *Module) openSelectedTask() {
	st := &m.tasks
	if st.selected == 0 {
		return
	}
	if st.source == sourceFlagged {
		if m.o.OpenMessage != nil {
			m.o.OpenMessage(st.selected)
		}
		return
	}
	st.focusTitle = true
}

// createTask creates the task the New Task field holds, in the source.
func (m *Module) createTask() {
	st := &m.tasks
	if !m.canCreateTask() {
		return
	}
	p := api.TasksCreateParams{}
	switch {
	case st.source >= 0:
		p.ListID = ptr(int64(st.source))
	case st.source == sourceToday:
		p.Due = ptr(m.today())
	}
	m.submitNewTask(&st.newTitle, &st.creating, p)
}

// submitNewTask creates the task a New Task field holds. The field keeps
// its text until maild has the task, so a failure loses nothing, and it
// keeps what was typed since. While a create runs, Enter does nothing. A
// create's answer from a maild the module has left only reports a
// failure: the field is the next maild's now.
func (m *Module) submitNewTask(text *string, creating *bool, p api.TasksCreateParams) {
	sent := *text
	p.Title = strings.TrimSpace(sent)
	if p.Title == "" {
		*text = ""
		return
	}
	if *creating {
		return
	}
	c := m.client
	if c == nil {
		m.o.Toast("Could not create the task", errNotConnected)
		return
	}
	*creating = true
	daemon := m.daemon
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		_, err := c.Tasks().Create(ctx, &p)
		m.o.Post(func() {
			if m.daemon != daemon {
				if err != nil {
					slog.Warn("create the task", "err", err)
					m.o.Toast("Could not create the task", err)
				}
				return
			}
			*creating = false
			if err != nil {
				slog.Warn("create the task", "err", err)
				m.o.Toast("Could not create the task", err)
				return
			}
			if *text == sent {
				*text = ""
			}
		})
	}()
}

func (m *Module) tasksPanes(c *ui.Context) []frostui.Pane {
	m.syncTasks()
	return []frostui.Pane{
		{ID: paneSidebar, Label: "Task Lists", Width: sidebarWidth, Min: 160, Max: 320, Collapsible: true, Content: func() { m.tasksSidebar(c) }},
		{ID: paneTasks, Label: "Task list", Min: 360, Bar: func() { m.tasksBar(c) }, Content: func() { m.taskListView(c) }},
		{ID: paneTask, Label: "Task details", Width: detailWidth, Min: detailWidth, Max: detailWidth, Content: func() { m.taskPane(c) }},
	}
}

func (m *Module) tasksBar(c *ui.Context) {
	t := c.Theme()
	st := &m.tasks
	if frostui.IconButton(c, "list-add", "New Task (Ctrl+N)").Disabled(!m.canCreateTask()).Clicked() {
		st.focusNew = true
	}
	frostui.Heading(c, m.tasksTitle()).TextColor(t.Text).Grow(1).Shrink(1).MinWidth(0).SingleLine()
	if st.source == sourceAll || st.source >= 0 {
		b := ui.Button(c, "Show Completed").Checked(st.showCompleted)
		if st.showCompleted {
			b.Background(t.SurfacePressed)
		}
		if b.Clicked() {
			st.showCompleted = !st.showCompleted
		}
	}
}

func (m *Module) tasksSidebar(c *ui.Context) {
	st := &m.tasks
	sections := []frostui.SourceSection[string]{{ID: "tasks", Title: "Tasks", Items: []string{"today", "all", "flagged"}}}
	for _, a := range m.accounts.value {
		var items []string
		for _, l := range st.lists.value {
			if l.AccountID == a.ID {
				items = append(items, sourceKey(taskSource(l.ID)))
			}
		}
		if len(items) > 0 {
			sections = append(sections, frostui.SourceSection[string]{ID: fmt.Sprint("account:", a.ID), Title: a.Email, Items: items})
		}
	}
	current := sourceKey(st.source)
	if key, _, ok := st.sources.Selected(); !ok || key != current {
		st.sources.Select("", current)
	}
	ui.Column(c).Grow(1).AlignItems(ui.Stretch).Background(frostui.Chrome(c.Theme())).Children(func() {
		tree := st.sources.View(c, sections).Grow(1)
		if st.focusAsk == paneSidebar {
			st.focusAsk = ""
			tree.Focus()
		}
		if tree.FocusWithin() {
			st.focus = paneSidebar
		}
	})
}

// taskListView draws the New Task field and the rows.
func (m *Module) taskListView(c *ui.Context) {
	t := c.Theme()
	st := &m.tasks
	shown := m.shownTasks()
	var names map[int64]string
	if st.source < 0 {
		names = map[int64]string{}
		for _, l := range st.lists.value {
			names[l.ID] = l.Name
		}
	}
	st.lines = taskLines(shown, names)
	if st.selected != 0 && st.all.loaded && !st.all.pending {
		ids := m.rowIDs()
		if !slices.Contains(ids, st.selected) && (st.source != sourceFlagged || (st.flagged != nil && st.flagged.Ready())) {
			st.selected = 0
		}
	}
	ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		if m.canCreateTask() {
			m.newTaskField(c)
		}
		if st.source == sourceFlagged {
			m.flaggedList(c)
		} else {
			m.taskRows(c, shown)
		}
	})
}

func (m *Module) newTaskField(c *ui.Context) {
	t := c.Theme()
	st := &m.tasks
	ui.Row(c).Height(40).Shrink(0).Padding(0, frostui.SpaceM).Gap(10).AlignItems(ui.Center).Children(func() {
		frostui.Icon(c, "list-add").FontSize(frostui.TextSize(c, 16)).TextColor(tertiary(t))
		in := ui.TextInputBase(c.Key("new-task"), &st.newTitle).Grow(1).Shrink(1).MinWidth(0).Label("New Task").Placeholder("New Task").FontSize(frostui.TextSize(c, 13))
		if st.focusNew {
			st.focusNew = false
			in.Focus()
		}
		in.OnShortcut(0, ui.KeyEscape, func() {
			st.newTitle = ""
			st.focusAsk = paneTasks
		})
		if in.Submitted() {
			m.createTask()
		}
	})
	frostui.RowDivider(c)
}

func (m *Module) taskRows(c *ui.Context, shown []api.Task) {
	st := &m.tasks
	if st.all.loaded && len(shown) == 0 {
		centered(c, "No Tasks", 15)
		return
	}
	st.row = slices.IndexFunc(st.lines, func(l taskLine) bool { return l.task != nil && l.task.ID == st.selected })
	st.listUI.Selected = &st.row
	st.listUI.Key = func(i int) any {
		if i >= len(st.lines) {
			return i
		}
		if l := st.lines[i]; l.task != nil {
			return l.task.ID
		}
		return "header:" + strconv.FormatInt(st.lines[i].listID, 10)
	}
	st.listUI.Header = func(i int) bool { return i < len(st.lines) && st.lines[i].task == nil }
	list := uikit.SoftList(c, &st.listUI, len(st.lines), func(i int) { m.taskLine(c, st.lines[i]) }).Label(m.tasksTitle())
	m.watchList(list)
	if list.Changed() && st.row >= 0 && st.row < len(st.lines) && st.lines[st.row].task != nil {
		st.selected = st.lines[st.row].task.ID
	}
	if list.Submitted() {
		m.openSelectedTask()
	}
}

func (m *Module) watchList(list ui.Element) {
	st := &m.tasks
	if st.focusAsk == paneTasks {
		st.focusAsk = ""
		list.Focus()
	}
	if list.FocusWithin() {
		st.focus = paneTasks
	}
}

// circle draws a check circle; it reports a click.
func circle(c *ui.Context, size float32, done bool, label string, disabled bool) bool {
	t := c.Theme()
	ring := tertiary(t)
	b := ui.ButtonBase(c.Key("check")).Size(size, size).Radius(size/2).Center().Border(1.5, ring).Shrink(0).KeepFocus().Cursor(ui.CursorPointer).Label(label).Disabled(disabled).DimDisabled(false).AccessHidden(true)
	if done {
		b.Background(t.Accent).Border(1.5, t.Accent)
	}
	clicked := b.Clicked()
	b.Children(func() {
		if done {
			frostui.Icon(c, "object-select").FontSize(size - 6).TextColor(t.AccentText)
		}
	})
	return clicked
}

func (m *Module) taskLine(c *ui.Context, l taskLine) {
	t := c.Theme()
	if l.task == nil {
		ui.Row(c).Height(28).Padding(0, frostui.SpaceM).AlignItems(ui.Center).Background(t.Background).Children(func() {
			frostui.Label(c, l.header)
		})
		return
	}
	task := l.task
	today := m.today()
	indent := float32(frostui.SpaceM)
	if l.level == 1 {
		indent += 28
	}
	muted := t.TextMuted
	sep := t.Border
	ui.Row(c).MinHeight(36).Padding(frostui.SpaceS, frostui.SpaceM, frostui.SpaceS, indent).Gap(10).AlignItems(ui.Start).Label(task.Title).Checked(task.Completed).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X + 40, Y: r.Y + r.H - 1, W: r.W - 40, H: 1}, sep, 0)
	}).Children(func() {
		readOnly := !m.taskWritable(task)
		if circle(c, 18, task.Completed, "Complete "+task.Title, readOnly) {
			m.toggleTask(task.ID, !task.Completed)
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			name := frostui.Body(c, task.Title).Wrap()
			if task.Completed {
				name.TextColor(tertiary(t))
			}
			due := dueText(task.Due, today)
			note, _, _ := strings.Cut(task.Notes, "\n")
			if due != "" || note != "" {
				ui.Row(c).Children(func() {
					if due != "" {
						line := ui.Text(c, due).FontSize(frostui.TextSize(c, 12)).TextColor(muted).SingleLine()
						if isOverdue(task, today) {
							line.TextColor(flagColor(t, 1))
						}
					}
					if due != "" && note != "" {
						ui.Text(c, " · ").FontSize(frostui.TextSize(c, 12)).TextColor(muted)
					}
					if note != "" {
						ui.Text(c, note).FontSize(frostui.TextSize(c, 12)).TextColor(muted).Shrink(1).MinWidth(0).SingleLine()
					}
				})
			}
		})
		if task.MessageID != nil {
			frostui.Icon(c, "mail-unread").FontSize(frostui.TextSize(c, 14)).TextColor(muted).Label("From mail")
		}
	})
}

func (m *Module) flaggedList(c *ui.Context) {
	st := &m.tasks
	if st.flagged != nil && st.flagged.Ready() && st.flagged.Count() == 0 {
		centered(c, "No Flagged Mail", 15)
		return
	}
	rows := st.flaggedShown
	st.row = slices.IndexFunc(rows, func(r api.MessageSummary) bool { return r.ID == st.selected })
	st.listUI.Selected = &st.row
	st.listUI.Key = func(i int) any {
		if i < len(rows) {
			return rows[i].ID
		}
		return i
	}
	st.listUI.Header = nil
	list := uikit.SoftList(c, &st.listUI, len(rows), func(i int) { m.flaggedRow(c, rows[i]) }).Label("Flagged Mail")
	m.watchList(list)
	if list.Changed() && st.row >= 0 && st.row < len(rows) {
		st.selected = rows[st.row].ID
	}
	if list.Submitted() {
		m.openSelectedTask()
	}
}

func subjectOf(r api.MessageSummary) string {
	if r.Subject != "" {
		return r.Subject
	}
	return "No Subject"
}

func (m *Module) flaggedRow(c *ui.Context, r api.MessageSummary) {
	ui.Row(c).MinHeight(36).Padding(frostui.SpaceS, frostui.SpaceM).Gap(10).AlignItems(ui.Start).Label(subjectOf(r)).Children(func() {
		if circle(c, 18, false, "Clear Flag "+subjectOf(r), false) {
			m.clearFlag(r.ID)
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			frostui.Body(c, subjectOf(r)).SingleLine()
			frostui.Muted(c, mail.DisplayName(r.From)+" · "+mail.ListDate(r.Date, m.now())).SingleLine()
		})
		frostui.Icon(c, fmt.Sprintf("%s%d", icons.FlagPrefix, max(r.Flags.FlagColor, 1))).FontSize(frostui.TextSize(c, 14)).Label("Flagged")
	})
}

// taskPane draws the selected task or flagged message.
//
// A field takes the input that came since the last frame as it is built.
// So when the selection or the source has moved on from the task the
// fields edit, the pane shows that task for one more frame: its fields
// take what was typed, or what an input method committed, before they
// are committed against it and bound to the next task.
func (m *Module) taskPane(c *ui.Context) {
	t := c.Theme()
	st := &m.tasks
	leaving := st.paneID != 0 && (st.source == sourceFlagged || st.selected != st.paneID)
	pane := ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Focusable().TabStop(false).FocusRing(false).Background(t.Background).Label("Task details")
	if st.focusAsk == paneTask {
		st.focusAsk = ""
		pane.Focus()
	}
	if pane.FocusWithin() {
		st.focus = paneTask
	}
	// Each task's fields are its own, so input meant for one never
	// reaches another's.
	details := func(task *api.Task) {
		ui.Scroll(c.Key(task.ID)).Grow(1).Fill().Children(func() {
			ui.Column(c).Padding(20).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() { m.taskDetails(c, task) })
		})
	}
	pane.Children(func() {
		if leaving {
			if task := m.task(st.paneID); task != nil {
				focusTitle := st.focusTitle // the next task's
				st.focusTitle = false
				details(task)
				st.focusTitle = focusTitle
				return
			}
		}
		if st.source == sourceFlagged {
			for _, r := range st.flaggedShown {
				if r.ID == st.selected {
					m.flaggedPane(c, r)
					return
				}
			}
			centered(c, "No Task Selected", 13)
			return
		}
		task := m.task(st.selected)
		if task == nil {
			centered(c, "No Task Selected", 13)
			return
		}
		details(task)
	})
	if leaving {
		m.leaveTaskPane()
		c.AnimationFrame()
	}
}

func (m *Module) flaggedPane(c *ui.Context, r api.MessageSummary) {
	t := c.Theme()
	ui.Column(c).Padding(20).Gap(frostui.SpaceS).AlignItems(ui.Start).Children(func() {
		ui.Text(c, subjectOf(r)).FontSize(frostui.TextSize(c, 17)).FontWeight(600).Wrap().Role(ui.RoleHeading)
		frostui.Body(c, mail.DisplayName(r.From)+" · "+mail.ListDate(r.Date, m.now())).TextColor(t.TextMuted)
		if textButton(c.Key("open"), "Open in Mail", t.Accent, 13).Clicked() && m.o.OpenMessage != nil {
			m.o.OpenMessage(r.ID)
		}
		if textButton(c.Key("clear"), "Clear Flag", t.Accent, 13).Clicked() {
			m.clearFlag(r.ID)
		}
	})
}

func (m *Module) editTask(id int64, p api.TasksUpdateParams) {
	st := &m.tasks
	t := m.task(id)
	if t == nil || !m.taskWritable(t) {
		return
	}
	p.ID = id
	st.all.gen++
	st.tasks = slices.Clone(st.tasks)
	x := &st.tasks[slices.IndexFunc(st.tasks, func(v api.Task) bool { return v.ID == id })]
	if p.Title != nil {
		x.Title = *p.Title
	}
	if p.Notes != nil {
		x.Notes = *p.Notes
	}
	if p.Due != nil {
		x.Due = *p.Due
	}
	m.writeTask(id, "change the task", func(ctx context.Context, c *api.Client) error {
		_, err := c.Tasks().Update(ctx, &p)
		return err
	})
}

// leaveTaskPane commits the pane's fields against the task they were
// editing, once another selection or source has taken the pane and the
// fields have taken their last input (taskPane).
func (m *Module) leaveTaskPane() {
	st := &m.tasks
	if st.paneID == 0 || (st.source != sourceFlagged && st.selected == st.paneID) {
		return
	}
	m.CommitTaskPane()
}

// TaskPaneActive reports whether task fields still belong to a task.
func (m *Module) TaskPaneActive() bool { return m.tasks.paneID != 0 }

// CommitTaskPane saves the fields after their last draw, before a module
// switch removes them. Selection stays, so returning shows the same task.
func (m *Module) CommitTaskPane() {
	st := &m.tasks
	if st.paneID == 0 {
		return
	}
	id := st.paneID
	st.paneID = 0
	t := m.task(id)
	if t == nil {
		return
	}
	var p api.TasksUpdateParams
	if v := strings.TrimSpace(st.paneTitle); v != "" && st.paneTitle != st.paneTitleWas && v != t.Title {
		p.Title = &v
	}
	if v := st.paneNotes; v != st.paneNotesWas && v != t.Notes {
		p.Notes = &v
	}
	if p.Title != nil || p.Notes != nil {
		m.editTask(id, p)
	}
}

func (m *Module) taskDetails(c *ui.Context, task *api.Task) {
	t := c.Theme()
	st := &m.tasks
	readOnly := !m.taskWritable(task)
	if st.paneID != task.ID {
		st.paneID, st.paneTitle, st.paneNotes, st.dueOpen = task.ID, task.Title, task.Notes, false
		st.paneTitleWas, st.paneNotesWas = task.Title, task.Notes
	}
	title := ui.TextInput(c.Key("title"), &st.paneTitle).Label("Title").FontSize(frostui.TextSize(c, 17)).FontWeight(600).ReadOnly(readOnly)
	if st.focusTitle {
		st.focusTitle = false
		title.Focus()
	}
	if !title.Focused() && !title.FocusWithin() && st.paneTitle != task.Title && !title.Submitted() && !title.FocusLeft() {
		st.paneTitle, st.paneTitleWas = task.Title, task.Title // the server's newer title
	}
	commitTitle := func() {
		if v := strings.TrimSpace(st.paneTitle); v == "" {
			st.paneTitle = task.Title
		} else if v != task.Title {
			m.editTask(task.ID, api.TasksUpdateParams{Title: &v})
		}
		st.paneTitleWas = st.paneTitle
	}
	title.OnShortcut(0, ui.KeyEscape, func() {
		st.paneTitle, st.paneTitleWas = task.Title, task.Title
		st.skipCommit = true
		st.focusAsk = paneTasks
	})
	if title.Submitted() {
		commitTitle()
	} else if title.FocusLeft() {
		if st.skipCommit {
			st.skipCommit = false
		} else {
			commitTitle()
		}
	}
	if readOnly {
		ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
			frostui.Icon(c, "changes-prevent").FontSize(frostui.TextSize(c, 12)).TextColor(tertiary(t))
			ui.Text(c, "Read-only").FontSize(frostui.TextSize(c, 12)).TextColor(tertiary(t))
		})
	}
	labeled(c, "Due", 80, func() { m.dueRow(c, task, readOnly) })
	if l := m.taskList(task.ListID); l != nil {
		account := m.accountEmail(l.AccountID)
		labeled(c, "List", 80, func() {
			nameAndAccount(c, t, l.Name, account, nil, 0)
		})
	}
	if task.Completed && task.CompletedAt != nil {
		at := task.CompletedAt.In(m.clock.loc)
		labeled(c, "Completed", 80, func() { frostui.Body(c, at.Format("Jan 2, 2006")+", "+m.clock.clockTime(at)) })
	}
	if task.MessageID != nil {
		id := *task.MessageID
		labeled(c, "From mail", 80, func() {
			if textButton(c, "Open Message", t.Accent, 13).Clicked() && m.o.OpenMessage != nil {
				m.o.OpenMessage(id)
			}
		})
	}
	frostui.Muted(c, "Notes")
	notes := ui.TextArea(c.Key("notes"), &st.paneNotes).Label("Notes").FontSize(frostui.TextSize(c, 13)).MinHeight(120).ReadOnly(readOnly)
	if notes.FocusLeft() && st.paneNotes != task.Notes {
		v := st.paneNotes
		st.paneNotesWas = v
		m.editTask(task.ID, api.TasksUpdateParams{Notes: &v})
	} else if !notes.Focused() && !notes.FocusWithin() && st.paneNotes != task.Notes {
		st.paneNotes, st.paneNotesWas = task.Notes, task.Notes
	}
	if !readOnly && textButton(c.Key("delete"), "Delete Task", flagColor(t, 1), 13).Clicked() {
		st.selected = task.ID
		m.deleteSelectedTask()
	}
}

func (m *Module) dueRow(c *ui.Context, task *api.Task, readOnly bool) {
	st := &m.tasks
	ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
		if task.Due != "" {
			st.dueT = dateOf(task.Due)
			if ui.DateInput(c.Key("due"), &st.dueT).Label("Due").Disabled(readOnly).Changed() {
				v := st.dueT.Format(time.DateOnly)
				m.editTask(task.ID, api.TasksUpdateParams{Due: &v})
			}
			if !readOnly && frostui.IconButton(c.Key("clear"), "window-close", "Clear Due Date").Clicked() {
				v := ""
				m.editTask(task.ID, api.TasksUpdateParams{Due: &v})
			}
			return
		}
		b := ui.Button(c.Key("due"), "None").Label("Due").Disabled(readOnly)
		if b.Clicked() {
			st.dueOpen = !st.dueOpen
			st.dueT = dateOf(m.today())
		}
		ui.Popover(c, b, &st.dueOpen, func() {
			if ui.Calendar(c, &st.dueT).Changed() {
				v := st.dueT.Format(time.DateOnly)
				st.dueOpen = false
				m.editTask(task.ID, api.TasksUpdateParams{Due: &v})
			}
		})
	})
}
