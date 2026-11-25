package main

import (
	"context"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/nsgo-dev/nsgo/base/dragdrop"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Changing events in the views (docs/specs/pim-ui.md): in the month and
// the week, a drag of an occurrence to another day moves it by whole
// days, keeping its times. Only an occurrence of a writable calendar that
// is no invitation moves.

// draggedEvent is the occurrence a drag carries, and the day of the cell
// it was taken from: a drop moves it by the days between that cell and the
// one it lands in, so an event of several days keeps its place under the
// pointer.
type draggedEvent struct {
	occurrence api.Occurrence
	from       string
	client     *api.Client
}

// canMove reports whether the user may drag an occurrence: in a writable
// calendar, and not an invitation.
func canMove(o api.Occurrence, writable map[int64]bool) bool {
	return writable[o.CalendarID] && o.Answer == nil
}

// dateOf reads a YYYY-MM-DD date as its midnight in UTC, for arithmetic of
// whole days.
func dateOf(date string) time.Time {
	t, _ := time.Parse(time.DateOnly, date)
	return t
}

func addDays(date string, n int) string { return dateOf(date).AddDate(0, 0, n).Format(time.DateOnly) }

// daysBetween is the whole days from one date to another.
func daysBetween(from, to string) int { return int(dateOf(to).Sub(dateOf(from)) / (24 * time.Hour)) }

// wireZone is the zone a timed event is written in: its own, or UTC where
// it has none.
func wireZone(zone string) string {
	if zone == "" || zone == "Local" {
		return "UTC"
	}
	return zone
}

// shiftedDays is what moves an occurrence by whole days, keeping its
// times: an all-day one by its dates, and a timed one by its wall time in
// loc, so that a move across a change of the clocks keeps the hour it
// shows. No move leaves its instants as they are.
func shiftedDays(o api.Occurrence, days int, loc *time.Location, zone string) api.EventFields {
	if o.AllDay {
		start, end := addDays(o.StartDate, days), addDays(o.EndDate, days)
		allDay := true
		return api.EventFields{AllDay: &allDay, StartDate: &start, EndDate: &end}
	}
	start, end := o.Start, o.End
	if days != 0 {
		at := o.Start.In(loc)
		start = time.Date(at.Year(), at.Month(), at.Day()+days, at.Hour(), at.Minute(), at.Second(), at.Nanosecond(), loc)
		end = start.Add(o.End.Sub(o.Start))
	}
	timed, startUTC, endUTC, wire := false, start.UTC(), end.UTC(), wireZone(zone)
	return api.EventFields{AllDay: &timed, Start: &startUTC, End: &endUTC, TimeZone: &wire}
}

// dragEvent makes an occurrence's block a thing to drag, from the cell of
// the day from.
func (w *workspace) dragEvent(cx *core.Scope, o api.Occurrence, from string, block core.Node) core.Node {
	source := dragdrop.Source{View: core.View{Label: "Drag " + o.Summary, Children: []core.Node{block}}, Data: func() any {
		return draggedEvent{occurrence: o, from: from, client: w.session.Client()}
	}, Preview: func(any) core.Node { return ui.Button{Text: o.Summary, Variant: ui.ButtonSecondary} }}
	if ui.Touch(cx) {
		source.Activation = dragdrop.AfterLongPress
	}
	return source
}

// dayTarget makes a day's cell a place to drop an occurrence taken from
// another day.
func (w *workspace) dayTarget(day time.Time, style func() core.Style, children []core.Node, drop func(o api.Occurrence, days int)) core.Node {
	date := day.Format(time.DateOnly)
	return core.Dynamic(func(cx *core.Scope) core.Node {
		over := core.NewSignal(cx, false)
		colors := ui.ColorsOf(cx)
		return dragdrop.Target{View: core.View{TestID: "day-" + date, Label: "Move to " + day.Format("Monday, January 2"), StyleFunc: func() core.Style {
			s := style()
			if over.Get() {
				s.Background = ui.Alpha(colors.Accent(), .12)
			}
			return s
		}, Children: children},
			Accept: func(v any) bool {
				d, ok := v.(draggedEvent)
				return ok && d.client != nil && d.client == w.session.Client() && d.from != date
			},
			OnEnter: func(dragdrop.Event) { over.Set(true) }, OnLeave: func(dragdrop.Event) { over.Set(false) },
			OnDrop: func(e dragdrop.Event) {
				over.Set(false)
				d := e.Data.(draggedEvent)
				drop(d.occurrence, daysBetween(d.from, date))
			}}
	})
}

// dropOccurrence writes a drop: the occurrence's event is read, for its
// zone and whether it is of a series, and then updated as an edit is,
// after the scope's question for a series and the notice to invitees.
func (w *workspace) dropOccurrence(o api.Occurrence, days int, loc *time.Location, load func()) {
	if days == 0 {
		return
	}
	perform(w, func(ctx context.Context, c *api.Client) (*api.CalendarEvent, error) {
		return c.Calendar().Event(ctx, &api.CalendarEventParams{ID: o.EventID, RecurrenceID: changed("", o.RecurrenceID)})
	}, func(e *api.CalendarEvent, err error) {
		if err != nil {
			w.problem.Set("Could not move the event: " + err.Error())
			return
		}
		w.updateEventFields(*e, shiftedDays(o, days, loc, e.TimeZone), load)
	})
}

// retimeOccurrence writes a drop on the day laid out by time: the
// occurrence's new start and end, in the event's zone, as an edit is
// written.
func (w *workspace) retimeOccurrence(o api.Occurrence, start, end time.Time, load func()) {
	if start.Equal(o.Start) && end.Equal(o.End) {
		return
	}
	perform(w, func(ctx context.Context, c *api.Client) (*api.CalendarEvent, error) {
		return c.Calendar().Event(ctx, &api.CalendarEventParams{ID: o.EventID, RecurrenceID: changed("", o.RecurrenceID)})
	}, func(e *api.CalendarEvent, err error) {
		if err != nil {
			w.problem.Set("Could not move the event: " + err.Error())
			return
		}
		timed, startUTC, endUTC, zone := false, start.UTC(), end.UTC(), wireZone(e.TimeZone)
		w.updateEventFields(*e, api.EventFields{AllDay: &timed, Start: &startUTC, End: &endUTC, TimeZone: &zone}, load)
	})
}
