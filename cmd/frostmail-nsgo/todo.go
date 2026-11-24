package main

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The flagged query follows flag changes even when the main list shows another folder.
func (w *workspace) flaggedMessages(cx *core.Scope, limit int64) func() []api.MessageSummary {
	rev := core.NewSignal(cx, 0)
	var view *live.View
	closeView := func() {
		if view != nil {
			view.Close()
			view = nil
		}
	}
	cx.Cleanup(closeView)
	cx.Effect(func() {
		w.status.Get()
		closeView()
		if c := w.session.Client(); c != nil {
			yes := true
			view = live.OpenView(c, w.events, api.ViewQuery{Flagged: &yes}, func() { cx.Post(func() { invalidate(rev) }) })
			view.Ensure(0, limit)
		}
		invalidate(rev)
	})
	return func() []api.MessageSummary {
		rev.Get()
		var out []api.MessageSummary
		if view != nil {
			view.Ensure(0, limit)
			for i := range min(limit, view.Count()) {
				if m, ok := view.Row(i); ok {
					out = append(out, m)
				}
			}
		}
		return out
	}
}

func (w *workspace) flagTaskRow(m api.MessageSummary) core.Node {
	return workspaceui.Row(ui.Checkbox{Label: "Clear flag on " + m.Subject, Disabled: func() bool { return !w.writable(m.AccountID) }, OnChange: func(bool) {
		undo := w.undo
		w.change(func(ctx context.Context, c *api.Client) error {
			no := false
			return undo.Flags(ctx, c, []int64{m.ID}, api.FlagChanges{Flagged: &no})
		}, nil)
	}}, button(m.Subject, func() { w.module.Set("Mail"); w.openMessage(m.ID, false) }))
}

func (w *workspace) todoBar(cx *core.Scope) core.Node {
	now := time.Now
	upcoming, refreshCalendar := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.CalendarChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Occurrence, error) {
		t := now()
		return func(ctx context.Context, c *api.Client) ([]api.Occurrence, error) {
			return c.Calendar().Range(ctx, &api.CalendarRangeParams{From: t.Format(time.DateOnly), To: t.AddDate(0, 0, 7).Format(time.DateOnly)})
		}
	})
	due, refreshTasks := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.TasksChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Task, error) {
		end := now().AddDate(0, 0, 8).Format(time.DateOnly)
		return func(ctx context.Context, c *api.Client) ([]api.Task, error) {
			tasks, err := c.Tasks().List(ctx, &api.TasksListParams{DueBefore: &end})
			slices.SortStableFunc(tasks, func(a, b api.Task) int { return cmp.Compare(a.Due, b.Due) })
			return tasks, err
		}
	})
	var tick func()
	tick = func() { refreshCalendar(); refreshTasks(); cx.AfterFunc(time.Minute, tick) }
	cx.AfterFunc(time.Minute, tick)
	flagged := w.flaggedMessages(cx, 5)
	calendar := ui.Calendar{DefaultValue: base.DateValue{Start: now()}, OnChange: func(v base.DateValue) { w.calendarDate = v.Start; w.module.Set("Calendar") }}
	events := core.Dynamic(func(*core.Scope) core.Node {
		var items []core.Node
		for _, e := range upcoming.Get() {
			if e.Status == api.EventStatusCancelled || !e.End.After(now()) {
				continue
			}
			date := e.Start.Local().Format("Mon 15:04")
			if e.AllDay {
				date = e.StartDate
			}
			items = append(items, button(e.Summary+" · "+date, func() {
				w.calendarDate = e.Start.Local()
				w.module.Set("Calendar")
				w.showOccurrence(e, func() {})
			}))
			if len(items) == 5 {
				break
			}
		}
		if len(items) == 0 {
			return core.Text{Content: "No upcoming events", Style: ui.TypeCaption}
		}
		return workspaceui.Stack(items...)
	})
	tasks := core.Dynamic(func(*core.Scope) core.Node {
		var items []core.Node
		for _, t := range due.Get() {
			if t.Completed || t.Due == "" {
				continue
			}
			items = append(items, workspaceui.Row(ui.Checkbox{Label: "Complete " + t.Title, Disabled: func() bool { return t.ReadOnly }, OnChange: func(bool) { w.setTaskCompleted(t, true, refreshTasks) }}, button(t.Title+" · "+t.Due, func() { w.module.Set("Tasks"); w.taskDetail(t, func() {}) })))
			if len(items) == 10 {
				break
			}
		}
		for _, m := range flagged() {
			items = append(items, w.flagTaskRow(m))
		}
		if len(items) == 0 {
			return core.Text{Content: "Nothing due", Style: ui.TypeCaption}
		}
		return workspaceui.Stack(items...)
	})
	title := core.NewSignal(cx, "")
	newTask := ui.Input{Label: "New task title", Placeholder: "New task", Value: title.Get, OnChange: title.Set, OnSubmit: func(v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		w.change(func(ctx context.Context, c *api.Client) error {
			_, err := c.Tasks().Create(ctx, &api.TasksCreateParams{Title: strings.TrimSpace(v)})
			return err
		}, func() {
			cx.Post(func() {
				if title.GetUntracked() == v {
					title.Set("")
				}
				refreshTasks()
			})
		})
	}}
	return core.Scroll{Style: ui.FillRest, Children: []core.Node{workspaceui.Stack(calendar, core.Text{Content: "Upcoming", Style: ui.TypeLabel}, events, core.Text{Content: "Tasks", Style: ui.TypeLabel}, newTask, tasks)}}
}

func (w *workspace) setTaskCompleted(t api.Task, complete bool, done func()) {
	w.change(func(ctx context.Context, c *api.Client) error {
		_, err := c.Tasks().Update(ctx, &api.TasksUpdateParams{ID: t.ID, Completed: &complete})
		return err
	}, done)
}
