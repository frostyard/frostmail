package pim

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/icons"
)

// todoState is the To-Do Bar's state.
type todoState struct {
	month    miniMonthState
	mini     fetch[[]api.Occurrence]
	upcoming fetch[[]api.Occurrence]
	newTitle string
	creating bool
	busyKey  string
	busy     map[string]bool
}

// ToDoBar draws Mail's right-hand pane: 280 wide, drawn by the caller
// beside the reader.
func (m *Module) ToDoBar(c *ui.Context) {
	m.frame(c)
	m.syncTasks()
	m.loadCalendars()
	st := &m.todo
	today := m.today()
	days := monthGrid(m.cal.date, m.clock.weekStart)
	minute := m.now().Truncate(time.Minute).String()
	m.loadRange(&st.mini, days[0], addDays(days[41], 1), minute)
	m.loadRange(&st.upcoming, today, addDays(today, 8), minute)
	if key := fmt.Sprint(st.mini.version, "|", days[0], "|", m.zone); key != st.busyKey {
		st.busyKey = key
		st.busy = busyDates(st.mini.value, days, m.clock.loc)
	}
	t := c.Theme()
	ui.Scroll(c).Grow(1).Fill().Background(frostui.Chrome(t)).Role(ui.RoleRegion).Label("To-Do Bar").Children(func() {
		ui.Column(c).AlignItems(ui.Stretch).Children(func() {
			m.miniMonth(c, &st.month, m.cal.date, st.busy, func(date string) {
				m.cal.date = date
				m.show(commands.Calendar)
			})
			m.todoUpcoming(c)
			m.todoTasks(c)
		})
	})
}

func (m *Module) todoUpcoming(c *ui.Context) {
	t := c.Theme()
	occs := nextOccurrences(m.todo.upcoming.value, m.now(), m.clock.loc, 5)
	ui.Column(c).Padding(frostui.SpaceL, frostui.SpaceM, 0, frostui.SpaceM).AlignItems(ui.Stretch).Children(func() {
		sectionHeading(c, "Upcoming").Padding(0, 0, frostui.SpaceS, 0)
		if len(occs) == 0 {
			ui.Text(c, "No Upcoming Events").FontSize(12).TextColor(tertiary(t))
			return
		}
		m.upcomingRows(c, occs)
	})
}

func (m *Module) todoTasks(c *ui.Context) {
	t := c.Theme()
	st := &m.todo
	today := m.today()
	tasks := dueSoon(m.tasks.tasks, today, 10)
	flagged := m.tasks.flaggedShown
	if len(flagged) > 5 {
		flagged = flagged[:5]
	}
	ui.Column(c).Padding(frostui.SpaceL, 0, frostui.SpaceM, 0).AlignItems(ui.Stretch).Children(func() {
		sectionHeading(c, "Tasks").Padding(0, frostui.SpaceM)
		ui.Row(c).Height(32).Padding(0, frostui.SpaceM).Gap(10).AlignItems(ui.Center).Children(func() {
			frostui.Icon(c, "list-add").FontSize(16).TextColor(tertiary(t))
			in := ui.TextInputBase(c.Key("todo-new"), &st.newTitle).Grow(1).Shrink(1).MinWidth(0).Label("New Task").Placeholder("New Task").FontSize(13)
			if in.Submitted() {
				m.submitNewTask(&st.newTitle, &st.creating, api.TasksCreateParams{})
			}
		})
		ui.Column(c).AlignItems(ui.Stretch).Role(ui.RoleList).Label("Due").Children(func() {
			for i := range tasks {
				m.todoTask(c.Key(tasks[i].ID), &tasks[i], today)
			}
			for i := range flagged {
				m.todoMessage(c.Key([2]any{"message", flagged[i].ID}), flagged[i])
			}
		})
		if len(tasks) == 0 && len(flagged) == 0 {
			ui.Text(c, "Nothing Due").FontSize(12).TextColor(tertiary(t)).Padding(0, frostui.SpaceM)
		}
	})
}

func (m *Module) todoTask(c *ui.Context, task *api.Task, today string) {
	t := c.Theme()
	ui.Row(c).Height(32).Padding(0, frostui.SpaceM).Gap(10).AlignItems(ui.Center).Role(ui.RoleListItem).Children(func() {
		if circle(c, 16, task.Completed, false, "Complete "+task.Title, !m.taskWritable(task)) {
			m.toggleTask(task.ID, !task.Completed)
		}
		if textButton(c.Key("title"), task.Title, t.Text, 13).Grow(1).Shrink(1).MinWidth(0).Clicked() {
			m.showTask(task)
		}
		due := ui.Text(c, dueText(task.Due, today)).FontSize(12).TextColor(t.TextMuted).SingleLine()
		if isOverdue(task, today) {
			due.TextColor(flagColor(t, 1))
		}
	})
}

// showTask opens a task in Tasks, with its list and the task selected.
func (m *Module) showTask(task *api.Task) {
	m.setTaskSource(taskSource(task.ListID))
	m.tasks.selected = task.ID
	m.show(commands.Tasks)
}

func (m *Module) todoMessage(c *ui.Context, r api.MessageSummary) {
	t := c.Theme()
	ui.Row(c).Height(32).Padding(0, frostui.SpaceM).Gap(10).AlignItems(ui.Center).Role(ui.RoleListItem).Children(func() {
		if circle(c, 16, false, false, "Clear Flag "+subjectOf(r), false) {
			m.clearFlag(r.ID)
		}
		if textButton(c.Key("subject"), subjectOf(r), t.Text, 13).Grow(1).Shrink(1).MinWidth(0).Clicked() && m.o.OpenMessage != nil {
			m.o.OpenMessage(r.ID)
		}
		frostui.Icon(c, fmt.Sprintf("%s%d", icons.FlagPrefix, max(r.Flags.FlagColor, 1))).FontSize(12).Label("Flagged")
	})
}
