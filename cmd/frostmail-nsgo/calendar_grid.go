package main

import (
	"fmt"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// All-day dates are calendar dates with an exclusive end. Timed events
// are instants that may cross midnight or the local daylight-saving change.
func dayOccurrences(rows []api.Occurrence, day time.Time) []api.Occurrence {
	date := day.Format(time.DateOnly)
	next := day.AddDate(0, 0, 1)
	var events []api.Occurrence
	for _, o := range rows {
		if o.AllDay {
			if date >= o.StartDate && date < o.EndDate {
				events = append(events, o)
			}
		} else if o.Start.Before(next) && (o.End.After(day) || o.Start.Equal(day)) {
			events = append(events, o)
		}
	}
	return events
}

// monthGrid draws a month's weeks. A day's cell shows 3 of its
// occurrences and counts the rest. An occurrence that movable allows is
// dragged to another day's cell, and move is told by how many days.
func (w *workspace) monthGrid(cx *core.Scope, at time.Time, rows *core.Signal[[]api.Occurrence], openDay func(time.Time), show func(api.Occurrence), movable func(api.Occurrence) bool, move func(api.Occurrence, int)) core.Node {
	start := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, at.Location())
	days := start.AddDate(0, 1, -1).Day()
	offset := (int(start.Weekday()) + 6) % 7
	start = start.AddDate(0, 0, -offset)
	days = ((days + offset + 6) / 7) * 7
	colors := ui.ColorsOf(cx)
	cellStyle := func() core.Style {
		return core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Padding: core.All(workspaceui.SpaceXS), Gap: 3, BorderWidth: 1, BorderColor: colors.Line()}
	}
	event := func(o api.Occurrence, day time.Time) core.Node {
		label := o.Summary
		if !o.AllDay {
			label = o.Start.In(day.Location()).Format("15:04") + "  " + label
		}
		var block core.Node = base.Button{Label: o.Summary, OnPress: func(core.PressEvent) { show(o) }, StyleFunc: func() core.Style {
			return core.Style{Background: ui.Alpha(colors.Accent(), .13), Padding: core.All(workspaceui.SpaceXS), CornerRadius: ui.RadiusSmall}
		}, Children: []core.Node{core.Text{Content: label, MaxLines: 2, Style: ui.TypeCaption}}}
		if movable(o) {
			block = w.dragEvent(cx, o, day.Format(time.DateOnly), block)
		}
		return block
	}
	return core.Dynamic(func(*core.Scope) core.Node {
		occurrences := rows.Get()
		var week, grid []core.Node
		for n := 0; n < days; n++ {
			day := start.AddDate(0, 0, n)
			events := dayOccurrences(occurrences, day)
			nodes := []core.Node{ui.Button{Text: day.Format("Mon 2"), Variant: ui.ButtonGhost, OnPress: func() { openDay(day) }}}
			count := min(3, len(events))
			for _, o := range events[:count] {
				nodes = append(nodes, event(o, day))
			}
			if len(events) > count {
				nodes = append(nodes, ui.Button{Text: fmt.Sprintf("+%d more", len(events)-count), Variant: ui.ButtonGhost, OnPress: func() { openDay(day) }})
			}
			week = append(week, w.dayTarget(day, cellStyle, nodes, move))
			if len(week) == 7 {
				grid = append(grid, core.View{Style: core.Style{Direction: core.Row, MinHeight: core.Pt(98), AlignItems: core.AlignStretch}, Children: week})
				week = nil
			}
		}
		return core.Scroll{Style: ui.FillRest, Children: grid}
	})
}
