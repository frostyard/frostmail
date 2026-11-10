package pim

import (
	"cmp"
	"slices"

	"github.com/frostyard/frostmail/api"
)

// taskSource is what the task list shows: sourceToday, sourceAll,
// sourceFlagged, or a list's ID.
type taskSource int64

const (
	sourceToday   taskSource = -1
	sourceAll     taskSource = -2
	sourceFlagged taskSource = -3
)

// taskLine is one line of the task list: a list's header, or a task.
type taskLine struct {
	header string
	listID int64
	task   *api.Task
	level  int
}

// dueText names a due date from today.
func dueText(due, today string) string {
	switch due {
	case "":
		return ""
	case today:
		return "Today"
	case addDays(today, 1):
		return "Tomorrow"
	case addDays(today, -1):
		return "Yesterday"
	}
	if due[:4] == today[:4] {
		return dateOf(due).Format("Mon, Jan 2")
	}
	return dateOf(due).Format("Jan 2, 2006")
}

// isOverdue tells whether an open task was due before today.
func isOverdue(t *api.Task, today string) bool {
	return !t.Completed && t.Due != "" && t.Due < today
}

// sourceTasks picks a source's tasks, keeping their order. Completed tasks
// stay out unless ticked here, or Show Completed is on for a list or All
// Tasks.
func sourceTasks(all []api.Task, s taskSource, today string, showCompleted bool, ticked map[int64]bool) []api.Task {
	if s == sourceFlagged {
		return nil
	}
	var out []api.Task
	for _, t := range all {
		belongs := s == sourceAll || (s == sourceToday && t.Due != "" && t.Due <= today) || (s >= 0 && t.ListID == int64(s))
		if belongs && (!t.Completed || ticked[t.ID] || (showCompleted && s != sourceToday)) {
			out = append(out, t)
		}
	}
	return out
}

// openCount counts a source's open tasks.
func openCount(all []api.Task, s taskSource, today string) int {
	return len(sourceTasks(all, s, today, false, nil))
}

// taskLines lays tasks out: a subtask is at level 1 when its parent shows
// before it. With names, a header comes before each list's first task.
func taskLines(tasks []api.Task, names map[int64]string) []taskLine {
	var lines []taskLine
	seen := map[int64]bool{}
	previous := int64(-1)
	for i := range tasks {
		t := &tasks[i]
		if names != nil && t.ListID != previous {
			lines = append(lines, taskLine{header: names[t.ListID], listID: t.ListID})
		}
		level := 0
		if t.ParentID != nil && seen[*t.ParentID] {
			level = 1
		}
		lines = append(lines, taskLine{task: t, level: level})
		seen[t.ID] = true
		previous = t.ListID
	}
	return lines
}

// dueSoon is the To-Do bar's tasks: open ones due within seven days of
// today or earlier, by due date then their order, at most limit.
func dueSoon(all []api.Task, today string, limit int) []api.Task {
	end := addDays(today, 7)
	var out []api.Task
	for _, t := range all {
		if !t.Completed && t.Due != "" && t.Due <= end {
			out = append(out, t)
		}
	}
	slices.SortStableFunc(out, func(a, b api.Task) int { return cmp.Compare(a.Due, b.Due) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
