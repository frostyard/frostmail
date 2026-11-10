package pim

import (
	"math"
	"time"

	"github.com/frostyard/frostmail/api"
)

// The arithmetic of dragging events (docs/specs/pim-ui.md, Changing events
// in the views): 15-minute steps in the app's wall time and whole days.
const (
	dragStep    = 15
	pxPerMinute = 0.8
	dragThresh  = 4
	resizeEdge  = 6
	hourHeight  = 48
)

// span is a timed range of instants.
type span struct{ start, end time.Time }

// stepsOf is a vertical move in pixels as whole steps of minutes.
func stepsOf(dy float32) int {
	return int(math.Round(float64(dy)/pxPerMinute/dragStep)) * dragStep
}

// daysOf is a horizontal move in pixels as whole columns of a width.
func daysOf(dx, width float32) int {
	if width <= 0 {
		return 0
	}
	return int(math.Round(float64(dx / width)))
}

// minuteAt is the step a pointer y pixels below the day's top falls in.
func minuteAt(y float32) int {
	m := int(math.Floor(float64(y)/pxPerMinute/dragStep)) * dragStep
	return max(0, min(24*60-dragStep, m))
}

// moved is a range moved by days and minutes of wall time, keeping its
// length.
func moved(r span, days, minutes int, loc *time.Location) span {
	date, wall := zoned(r.start, loc)
	start := instantAt(addDays(date, days), wall+minutes, loc)
	return span{start, start.Add(r.end.Sub(r.start))}
}

// resized is a range whose end moved, at least a step after its start.
func resized(r span, minutes int, loc *time.Location) span {
	date, wall := zoned(r.end, loc)
	end := instantAt(date, wall+minutes, loc)
	least := r.start.Add(dragStep * time.Minute)
	if end.Before(least) {
		end = least
	}
	return span{r.start, end}
}

// drawn is the range drawn on a day across two steps, in either order.
func drawn(date string, from, to int, loc *time.Location) span {
	return span{instantAt(date, min(from, to), loc), instantAt(date, min(max(from, to)+dragStep, 24*60), loc)}
}

// canMove reports whether the user may drag an occurrence: in a writable
// calendar, and not an invitation.
func canMove(o *api.Occurrence, writable map[int64]bool) bool {
	return writable[o.CalendarID] && o.Answer == nil
}

func ptr[T any](v T) *T { return &v }

// dropFields is what a drop of a timed range writes, in the event's zone.
func dropFields(r span, zone string) api.EventFields {
	return api.EventFields{AllDay: ptr(false), Start: ptr(r.start.UTC()), End: ptr(r.end.UTC()), TimeZone: ptr(wireZone(zone))}
}

// shiftedDays is what moves an occurrence by whole days, keeping its
// times.
func shiftedDays(o *api.Occurrence, days int, loc *time.Location, zone string) api.EventFields {
	if o.AllDay {
		return api.EventFields{AllDay: ptr(true), StartDate: ptr(addDays(o.StartDate, days)), EndDate: ptr(addDays(o.EndDate, days))}
	}
	return dropFields(moved(span{o.Start, o.End}, days, 0, loc), zone)
}
