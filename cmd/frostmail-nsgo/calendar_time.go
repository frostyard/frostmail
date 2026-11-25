package main

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The day and the week laid out by time (docs/specs/pim-ui.md, Changing
// events in the views): a day is a column 24 hours high, an occurrence a
// block from its start to its end, and a drag changes it in steps of 15
// minutes. The block moves, across days too; its lower edge resizes it;
// and a drag on empty time draws a new event. Nothing is written until
// the drag ends, and Escape puts everything back.

const (
	// minutePoints is a minute's height: an hour is 48 points.
	minutePoints = 0.8
	// dragStep is the minutes a drag moves by.
	dragStep = 15
	// resizeEdge is the height at a block's foot that resizes it.
	resizeEdge core.Points = 6
	// leastBlock is the least height a block is drawn at: a line of its
	// text.
	leastBlock core.Points = 16
	// hourGutter is the width of the hours' column.
	hourGutter core.Points = 52
	dayMinutes             = 24 * 60
	// firstHour is the hour a grid starts scrolled to.
	firstHour = 7
)

// stepsOf is a vertical move in points as whole steps of minutes.
func stepsOf(dy core.Points) int {
	return int(math.Round(float64(dy)/minutePoints/dragStep)) * dragStep
}

// daysOf is a horizontal move in points as whole columns of a width.
func daysOf(dx, width core.Points) int {
	if width <= 0 {
		return 0
	}
	return int(math.Round(float64(dx / width)))
}

// minuteAt is the step a pointer y points below a day's top falls in.
func minuteAt(y core.Points) int {
	m := int(math.Floor(float64(y)/minutePoints/dragStep)) * dragStep
	return max(0, min(dayMinutes-dragStep, m))
}

// atMinute is the instant of a wall time, in minutes after midnight, on a
// day.
func atMinute(day time.Time, minutes int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, minutes, 0, 0, day.Location())
}

// movedBy is a range moved by days and minutes of wall time, keeping its
// length. No move leaves its instants as they are.
func movedBy(start, end time.Time, days, minutes int, loc *time.Location) (time.Time, time.Time) {
	if days == 0 && minutes == 0 {
		return start, end
	}
	at := start.In(loc)
	next := time.Date(at.Year(), at.Month(), at.Day()+days, at.Hour(), at.Minute()+minutes, 0, 0, loc)
	return next, next.Add(end.Sub(start))
}

// resizedBy is a range whose end moved, to no less than a step after its
// start.
func resizedBy(start, end time.Time, minutes int, loc *time.Location) (time.Time, time.Time) {
	if minutes == 0 {
		return start, end
	}
	at := end.In(loc)
	next := time.Date(at.Year(), at.Month(), at.Day(), at.Hour(), at.Minute()+minutes, 0, 0, loc)
	if least := start.Add(dragStep * time.Minute); next.Before(least) {
		next = least
	}
	return start, next
}

// drawnOn is the range drawn on a day across 2 steps, in either order.
func drawnOn(day time.Time, from, to int) (time.Time, time.Time) {
	return atMinute(day, min(from, to)), atMinute(day, min(max(from, to)+dragStep, dayMinutes))
}

// placed is a timed occurrence's block in a day's column: its minutes
// from the day's start, and its lane among the blocks it overlaps.
type placed struct {
	occurrence  api.Occurrence
	from, to    int
	lane, lanes int
}

// place lays a day's timed occurrences out: each from its start to its
// end within the day, at least a step high, and side by side with those
// it overlaps.
func place(rows []api.Occurrence, day time.Time) []placed {
	next := day.AddDate(0, 0, 1)
	var out []placed
	for _, o := range rows {
		if o.AllDay || !o.Start.Before(next) || !(o.End.After(day) || o.Start.Equal(day)) {
			continue
		}
		from, to := 0, dayMinutes
		if o.Start.After(day) {
			at := o.Start.In(day.Location())
			from = at.Hour()*60 + at.Minute()
		}
		if o.End.Before(next) {
			at := o.End.In(day.Location())
			to = at.Hour()*60 + at.Minute()
		}
		out = append(out, placed{occurrence: o, from: from, to: max(to, from+dragStep)})
	}
	slices.SortStableFunc(out, func(a, b placed) int {
		if a.from != b.from {
			return a.from - b.from
		}
		return b.to - a.to
	})
	// Blocks that overlap, one through another, share their width.
	for i := 0; i < len(out); {
		end, j := out[i].to, i+1
		for j < len(out) && out[j].from < end {
			end = max(end, out[j].to)
			j++
		}
		var ends []int
		for k := i; k < j; k++ {
			lane := slices.IndexFunc(ends, func(e int) bool { return e <= out[k].from })
			if lane < 0 {
				lane = len(ends)
				ends = append(ends, 0)
			}
			ends[lane] = out[k].to
			out[k].lane = lane
		}
		for k := i; k < j; k++ {
			out[k].lanes = len(ends)
		}
		i = j
	}
	return out
}

// timedDrag is a drag in progress on the grid.
type timedDrag struct {
	// key names the dragged occurrence; empty for a drawn block.
	key string
	// column is the day the drag began in.
	column int
	// days and minutes are how far a block moved; resize is how far its
	// end did.
	days, minutes, resize int
	// drawFrom and drawTo are the steps a drawn block spans.
	drawFrom, drawTo int
	drawing          bool
}

func occurrenceKey(o api.Occurrence) string {
	return fmt.Sprintf("%d/%s", o.EventID, o.RecurrenceID)
}

// timeRange writes a block's times: "09:15 – 10:15".
func timeRange(from, to int) string {
	return fmt.Sprintf("%02d:%02d – %02d:%02d", from/60, from%60, (to%dayMinutes)/60, to%60)
}

// gridActions are what the time grid does with a press and a drop.
type gridActions struct {
	openDay  func(time.Time)
	show     func(api.Occurrence)
	create   func(start, end time.Time)
	movable  func(api.Occurrence) bool
	moveDays func(api.Occurrence, int)
	retime   func(o api.Occurrence, start, end time.Time)
}

// timeGrid draws days side by side, each laid out by time, under a strip
// of their dates and all-day occurrences.
func (w *workspace) timeGrid(cx *core.Scope, start time.Time, days int, rows *core.Signal[[]api.Occurrence], g gridActions) core.Node {
	colors := ui.ColorsOf(cx)
	drag := core.NewSignalFunc(cx, (*timedDrag)(nil), func(_, _ *timedDrag) bool { return false })
	cancelled := false
	columnWidth := core.NewSignal(cx, core.Points(0))
	loc := start.Location()
	// Escape during a drag puts everything back and writes nothing.
	cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, func() {
		if drag.GetUntracked() != nil {
			cancelled = true
			drag.Set(nil)
		}
	})
	touch := ui.Touch(cx)
	block := func(p placed, column int, day time.Time) core.Node {
		o := p.occurrence
		key := occurrenceKey(o)
		movable := g.movable(o) && !touch
		span := func() (from, to int, lifted bool) {
			from, to = p.from, p.to
			if d := drag.Get(); d != nil && d.key == key && d.column == column {
				return from + d.minutes, to + d.minutes + d.resize, true
			}
			return from, to, false
		}
		// A block under half an hour has room for 1 line: its title while it
		// rests, and its times while a drag changes them.
		short := p.to-p.from < 2*dragStep
		lines := 2
		if short {
			lines = 1
		}
		label := core.Text{MaxLines: lines, Style: ui.TypeCaption, ContentFunc: func() string {
			from, to, lifted := span()
			if short && !lifted {
				return o.Summary
			}
			return timeRange(from, to) + "  " + o.Summary
		}}
		face := base.Button{Label: o.Summary, OnPress: func(core.PressEvent) { g.show(o) }, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Overflow: core.Hidden},
			StyleFunc: func() core.Style {
				s := core.Style{Background: ui.Alpha(colors.Accent(), .16), Padding: core.All(workspaceui.SpaceXS), CornerRadius: ui.RadiusSmall}
				if short {
					s.Padding = core.Symmetric(0, workspaceui.SpaceXS)
				}
				return s
			}, Children: []core.Node{label}}
		view := core.View{TestID: "block-" + key, Style: core.Style{Position: core.Absolute, Left: core.Pct(100 * float32(p.lane) / float32(p.lanes)), Width: core.Pct(100 / float32(p.lanes))},
			StyleFunc: func() core.Style {
				from, to, lifted := span()
				// A block is never lower than a line of its text.
				s := core.Style{Top: core.Pt(core.Points(float64(from) * minutePoints)), Height: core.Pt(max(core.Points(float64(to-from)*minutePoints), leastBlock))}
				if d := drag.Get(); lifted {
					s.Opacity = core.Alpha(.6)
					if d.days != 0 {
						// The block is drawn in the day it moves to.
						s.Opacity = core.Alpha(.15)
					}
				}
				return s
			}, Children: []core.Node{face}}
		if !movable {
			return view
		}
		view.OnPan = func(e core.PanEvent) {
			switch e.Phase {
			case core.GestureBegan:
				cancelled = false
				drag.Set(&timedDrag{key: key, column: column})
			case core.GestureChanged:
				if !cancelled {
					drag.Set(&timedDrag{key: key, column: column, minutes: stepsOf(e.Translation.Y), days: daysOf(e.Translation.X, columnWidth.GetUntracked())})
				}
			default:
				d := drag.GetUntracked()
				drag.Set(nil)
				if cancelled || e.Phase != core.GestureEnded || d == nil {
					return
				}
				// A drag that ends where it began is a press.
				if d.minutes == 0 && d.days == 0 {
					g.show(o)
					return
				}
				from, to := movedBy(o.Start, o.End, d.days, d.minutes, loc)
				g.retime(o, from, to)
			}
		}
		// The block's foot resizes it: the end moves, the start stays.
		view.Children = append(view.Children, core.View{TestID: "resize-" + key, Label: "Resize " + o.Summary,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Height: core.Pt(resizeEdge)},
			OnPan: func(e core.PanEvent) {
				switch e.Phase {
				case core.GestureBegan:
					cancelled = false
					drag.Set(&timedDrag{key: key, column: column})
				case core.GestureChanged:
					if !cancelled {
						drag.Set(&timedDrag{key: key, column: column, resize: max(stepsOf(e.Translation.Y), dragStep-(p.to-p.from))})
					}
				default:
					d := drag.GetUntracked()
					drag.Set(nil)
					if cancelled || e.Phase != core.GestureEnded || d == nil || d.resize == 0 {
						return
					}
					from, to := resizedBy(o.Start, o.End, d.resize, loc)
					g.retime(o, from, to)
				}
			}})
		return view
	}
	// ghost is a block drawn in the day a drag moves it to.
	ghost := func(column int, day time.Time) core.Node {
		return core.Dynamic(func(*core.Scope) core.Node {
			d := drag.Get()
			if d == nil {
				return nil
			}
			at := func(from, to int, text string) core.Node {
				return core.View{TestID: "drag-ghost", Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0),
					Top: core.Pt(core.Points(float64(from) * minutePoints)), Height: core.Pt(core.Points(float64(max(to-from, dragStep)) * minutePoints)), Opacity: core.Alpha(.6), Overflow: core.Hidden},
					StyleFunc: func() core.Style {
						return core.Style{Background: ui.Alpha(colors.Accent(), .28), Padding: core.All(workspaceui.SpaceXS), CornerRadius: ui.RadiusSmall}
					}, Children: []core.Node{core.Text{Content: text, MaxLines: 2, Style: ui.TypeCaption}}}
			}
			if d.drawing {
				if d.column != column {
					return nil
				}
				from, to := min(d.drawFrom, d.drawTo), min(max(d.drawFrom, d.drawTo)+dragStep, dayMinutes)
				return at(from, to, timeRange(from, to)+"  New event")
			}
			if d.days == 0 || d.column+d.days != column {
				return nil
			}
			for _, p := range place(rows.Get(), start.AddDate(0, 0, d.column)) {
				if occurrenceKey(p.occurrence) == d.key {
					return at(p.from+d.minutes, p.to+d.minutes, timeRange(p.from+d.minutes, p.to+d.minutes)+"  "+p.occurrence.Summary)
				}
			}
			return nil
		})
	}
	column := func(n int, day time.Time) core.Node {
		// The empty time of a day: a press makes an event of an hour there,
		// and a drag draws one.
		var drawStart int
		ground := core.View{TestID: "time-" + day.Format(time.DateOnly), Label: "Empty time on " + day.Format("Monday, January 2"),
			Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Top: core.Pt(0), Bottom: core.Pt(0)},
			OnPress: func(e core.PressEvent) {
				at := minuteAt(e.Position.Y) / 60 * 60
				g.create(atMinute(day, at), atMinute(day, at+60))
			}}
		if !touch {
			ground.OnPan = func(e core.PanEvent) {
				switch e.Phase {
				case core.GestureBegan:
					cancelled = false
					drawStart = minuteAt(e.Position.Y - e.Translation.Y)
					drag.Set(&timedDrag{column: n, drawing: true, drawFrom: drawStart, drawTo: minuteAt(e.Position.Y)})
				case core.GestureChanged:
					if !cancelled {
						drag.Set(&timedDrag{column: n, drawing: true, drawFrom: drawStart, drawTo: minuteAt(e.Position.Y)})
					}
				default:
					d := drag.GetUntracked()
					drag.Set(nil)
					if cancelled || e.Phase != core.GestureEnded || d == nil {
						return
					}
					g.create(drawnOn(day, d.drawFrom, d.drawTo))
				}
			}
		}
		return core.Dynamic(func(*core.Scope) core.Node {
			children := []core.Node{ground}
			for hour := 1; hour < 24; hour++ {
				children = append(children, core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Top: core.Pt(core.Points(float64(hour*60) * minutePoints)), Height: core.Pt(1)},
					PointerEvents: core.PointerEventsNone, StyleFunc: ui.Filled(colors.Line)})
			}
			for _, p := range place(rows.Get(), day) {
				children = append(children, block(p, n, day))
			}
			children = append(children, ghost(n, day))
			return core.View{TestID: "column-" + day.Format(time.DateOnly), OnLayout: func(b core.Bounds) { columnWidth.Set(b.Size.Width) },
				Style:     core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Height: core.Pt(core.Points(dayMinutes * minutePoints)), Borders: core.Edges{Left: 1}},
				StyleFunc: func() core.Style { return core.Style{BorderColor: colors.Line()} }, Children: children}
		})
	}
	strip := core.Dynamic(func(*core.Scope) core.Node {
		cells := []core.Node{core.View{Style: core.Style{Width: core.Pt(hourGutter), FlexShrink: 0}}}
		for n := 0; n < days; n++ {
			day := start.AddDate(0, 0, n)
			date := day.Format(time.DateOnly)
			nodes := []core.Node{ui.Button{Text: day.Format("Mon 2"), Variant: ui.ButtonGhost, OnPress: func() { g.openDay(day) }}}
			for _, o := range dayOccurrences(rows.Get(), day) {
				if !o.AllDay {
					continue
				}
				var chip core.Node = base.Button{Label: o.Summary, OnPress: func(core.PressEvent) { g.show(o) }, StyleFunc: func() core.Style {
					return core.Style{Background: ui.Alpha(colors.Accent(), .13), Padding: core.All(workspaceui.SpaceXS), CornerRadius: ui.RadiusSmall}
				}, Children: []core.Node{core.Text{Content: o.Summary, MaxLines: 1, Style: ui.TypeCaption}}}
				if days > 1 && g.movable(o) {
					chip = w.dragEvent(cx, o, date, chip)
				}
				nodes = append(nodes, chip)
			}
			style := func() core.Style {
				return core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Padding: core.All(workspaceui.SpaceXS), Gap: 3, Borders: core.Edges{Left: 1, Bottom: 1}, BorderColor: colors.Line()}
			}
			if days > 1 {
				cells = append(cells, w.dayTarget(day, style, nodes, g.moveDays))
			} else {
				cells = append(cells, core.View{StyleFunc: style, Children: nodes})
			}
		}
		return core.View{TestID: "all-day", Style: core.Style{Direction: core.Row, AlignItems: core.AlignStretch, FlexShrink: 0}, Children: cells}
	})
	hours := make([]core.Node, 24)
	for hour := range hours {
		hours[hour] = core.View{Style: core.Style{Height: core.Pt(core.Points(60 * minutePoints)), AlignItems: core.AlignEnd, Padding: core.Edges{Right: workspaceui.SpaceXS}},
			Children: []core.Node{figures(cx, fmt.Sprintf("%02d:00", hour))}}
	}
	body := []core.Node{core.View{Style: core.Style{Width: core.Pt(hourGutter), FlexShrink: 0}, Children: hours}}
	for n := 0; n < days; n++ {
		body = append(body, column(n, start.AddDate(0, 0, n)))
	}
	return core.View{TestID: "time-grid", Style: ui.FillRest, Children: []core.Node{strip,
		core.Scroll{Style: ui.FillRest, ContentOffset: core.Point{Y: core.Points(firstHour * 60 * minutePoints)}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart}, Children: body}}}}}
}
