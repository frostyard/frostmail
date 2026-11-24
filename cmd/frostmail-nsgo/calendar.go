package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workspace) calendar(cx *core.Scope) core.Node {
	now := time.Now()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	month := core.NewSignal(cx, first)
	query := ""
	rows, load := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.CalendarChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Occurrence, error) {
		start, q := month.GetUntracked(), query
		return func(ctx context.Context, c *api.Client) ([]api.Occurrence, error) {
			if q != "" {
				return c.Calendar().Search(ctx, &api.CalendarSearchParams{Query: q})
			}
			return c.Calendar().Range(ctx, &api.CalendarRangeParams{From: start.Format(time.DateOnly), To: start.AddDate(0, 1, 0).Format(time.DateOnly)})
		}
	})
	step := func(n int) { month.Set(month.GetUntracked().AddDate(0, n, 0)); load() }
	return core.View{Style: ui.FillRest, Children: []core.Node{workspaceui.Row(ui.Button{Text: "Previous month", OnPress: func() { step(-1) }}, core.Text{ContentFunc: func() string { return month.Get().Format("January 2006") }, Style: ui.TypeTitle}, ui.Button{Text: "Next month", OnPress: func() { step(1) }}, ui.Button{Text: "Today", OnPress: func() { month.Set(first); load() }}, ui.Button{Text: "New event", OnPress: func() { w.eventForm(api.CalendarEvent{}, nil, load) }}), ui.Input{Label: "Search events", Placeholder: "Search calendars", OnSubmit: func(v string) { query = strings.TrimSpace(v); load() }}, collectionList(rows, "Calendar events", func(o api.Occurrence) string { return o.Summary }, func(o api.Occurrence) string {
		if o.AllDay {
			return o.StartDate + " · All day"
		}
		return o.Start.Local().Format("Mon Jan 2 · 15:04")
	}, func(o api.Occurrence) {
		perform(w, func(ctx context.Context, c *api.Client) (*api.CalendarEvent, error) {
			return c.Calendar().Event(ctx, &api.CalendarEventParams{ID: o.EventID, RecurrenceID: changed("", o.RecurrenceID)})
		}, func(e *api.CalendarEvent, err error) {
			if err != nil {
				w.problem.Set(err.Error())
				return
			}
			w.eventDetail(*e, load)
		})
	})}}
}
func (w *workspace) scopeFor(e api.CalendarEvent, f func(*api.EventScope)) {
	if !e.Recurring {
		f(nil)
		return
	}
	w.show(func(*core.Scope) core.Node {
		var actions []core.Node
		for _, x := range []struct {
			label string
			v     api.EventScope
		}{{"This occurrence", api.EventScopeThis}, {"This and future occurrences", api.EventScopeFuture}, {"Entire series", api.EventScopeAll}} {
			actions = append(actions, ui.Button{Text: x.label, OnPress: func() { f(&x.v) }})
		}
		return w.dialog("Which events?", actions)
	})
}
func (w *workspace) eventDetail(e api.CalendarEvent, load func()) {
	w.show(func(*core.Scope) core.Node {
		when := e.Start.Local().Format(time.DateTime) + " – " + e.End.Local().Format(time.DateTime)
		if e.AllDay {
			when = e.StartDate + " – " + e.EndDate + " (exclusive)"
		}
		nodes := []core.Node{core.Text{Content: when, Style: ui.TypeLabel}, core.Text{Content: e.Location}, core.Text{Content: e.Description, Selectable: core.SelectableOn}}
		for _, a := range e.Attendees {
			nodes = append(nodes, core.Text{Content: a.Email, Style: ui.TypeCaption})
		}
		if !e.ReadOnly {
			nodes = append(nodes, workspaceui.Row(ui.Button{Text: "Edit event", OnPress: func() { w.scopeFor(e, func(scope *api.EventScope) { w.eventForm(e, scope, load) }) }}, ui.Button{Text: "Delete event", Variant: ui.ButtonGhost, OnPress: func() {
				w.scopeFor(e, func(scope *api.EventScope) {
					w.confirm("Delete event?", e.Summary, "Delete", func() {
						w.writeEvent(func(ctx context.Context, c *api.Client, notify *bool) error {
							_, err := c.Calendar().Delete(ctx, &api.CalendarDeleteParams{ID: e.ID, RecurrenceID: changed("", e.RecurrenceID), Scope: scope, Notify: notify})
							return err
						}, load)
					})
				})
			}}))
		}
		return w.dialog(e.Summary, nodes)
	})
}
func (w *workspace) writeEvent(op func(context.Context, *api.Client, *bool) error, load func()) {
	if w.acting.GetUntracked() {
		return
	}
	w.acting.Set(true)
	perform(w, func(ctx context.Context, c *api.Client) (struct{}, error) { return struct{}{}, op(ctx, c, nil) }, func(_ struct{}, err error) {
		w.acting.Set(false)
		if errors.Is(err, api.ErrNotifyRequired) {
			w.confirm("Notify attendees?", "This change sends a calendar notice to attendees.", "Send notice", func() {
				w.change(func(ctx context.Context, c *api.Client) error { yes := true; return op(ctx, c, &yes) }, load)
			})
			return
		}
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		w.closePanel()
		load()
	})
}
func (w *workspace) eventForm(e api.CalendarEvent, scope *api.EventScope, load func()) {
	loc := time.Local
	if e.TimeZone != "" {
		if v, err := time.LoadLocation(e.TimeZone); err == nil {
			loc = v
		}
	}
	start, end := e.Start, e.End
	if e.ID == 0 {
		start = time.Now().Truncate(time.Hour).Add(time.Hour)
		end = start.Add(time.Hour)
	}
	const layout = "2006-01-02 15:04"
	from, to := start.In(loc).Format(layout), end.In(loc).Format(layout)
	if e.AllDay {
		from, to = e.StartDate, e.EndDate
	}
	zone := e.TimeZone
	if zone == "" {
		zone = loc.String()
	}
	w.form("Event", []editField{{"Title", e.Summary, false}, {"Location", e.Location, false}, {"Description", e.Description, true}, {"Start (date or date and time)", from, false}, {"End (exclusive)", to, false}, {"Time zone", zone, false}, {"Recurrence rule", e.Recurrence, false}}, func(v []string) error {
		if strings.TrimSpace(v[0]) == "" {
			return fmt.Errorf("Enter an event title")
		}
		fields := api.EventFields{Summary: changed(e.Summary, v[0]), Location: changed(e.Location, v[1]), Description: changed(e.Description, v[2]), Rrule: changed(e.Recurrence, v[6])}
		if e.ID == 0 || v[3] != from || v[4] != to || v[5] != zone {
			all := len(v[3]) == len(time.DateOnly)
			fields.AllDay = &all
			if all {
				a, err := time.Parse(time.DateOnly, v[3])
				if err != nil {
					return err
				}
				b, err := time.Parse(time.DateOnly, v[4])
				if err != nil {
					return err
				}
				if !b.After(a) {
					return fmt.Errorf("End must follow start")
				}
				fields.StartDate = &v[3]
				fields.EndDate = &v[4]
			} else {
				z, err := time.LoadLocation(v[5])
				if err != nil {
					return err
				}
				a, err := time.ParseInLocation(layout, v[3], z)
				if err != nil {
					return err
				}
				b, err := time.ParseInLocation(layout, v[4], z)
				if err != nil {
					return err
				}
				if !b.After(a) {
					return fmt.Errorf("End must follow start")
				}
				fields.Start = &a
				fields.End = &b
				name := v[5]
				if name == "Local" {
					name = "UTC"
				}
				fields.TimeZone = &name
			}
		}
		w.writeEvent(func(ctx context.Context, c *api.Client, notify *bool) error {
			if e.ID == 0 {
				_, err := c.Calendar().Create(ctx, &api.CalendarCreateParams{Fields: fields, Notify: notify})
				return err
			}
			_, err := c.Calendar().Update(ctx, &api.CalendarUpdateParams{ID: e.ID, RecurrenceID: changed("", e.RecurrenceID), Scope: scope, Fields: fields, Notify: notify})
			return err
		}, load)
		return nil
	})
}
