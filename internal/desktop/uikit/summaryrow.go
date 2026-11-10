// Package uikit holds UI patterns meant for frostui (plan 0015). It
// imports only the standard library, MyGo, purego and frostui, so a
// pattern can move there by changing an import path.
package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// SummaryHeight is a summary row's height.
const SummaryHeight = 84

// Summary is what one row of a list of items shows: mail, notes or feeds.
type Summary struct {
	Title, Date, Subtitle, Preview string
	// Unread draws the dot in the row's gutter.
	Unread bool
}

// SummaryRow draws a list row of a fixed height: the title and the date,
// the subtitle, and up to 2 lines of preview. A row with no title yet, as
// one still loading, keeps the height and shows nothing.
func SummaryRow(c *ui.Context, s Summary) ui.Element {
	t := c.Theme()
	row := ui.Row(c).Height(SummaryHeight).Padding(frostui.SpaceS, frostui.SpaceM).Gap(frostui.SpaceS).AlignItems(ui.Start)
	row.Children(func() {
		ui.Box(c).Width(10).Height(18).Center().Children(func() {
			if s.Unread {
				ui.Box(c).Size(8, 8).Radius(4).Background(t.Accent).Label("Unread")
			}
		})
		ui.Column(c).Grow(1).Gap(2).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Strong(c, s.Title).Grow(1).SingleLine()
				frostui.Caption(c, s.Date).SingleLine()
			})
			frostui.Body(c, s.Subtitle).SingleLine()
			frostui.Muted(c, s.Preview).MaxLines(2)
		})
	})
	return row
}
