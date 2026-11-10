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

// SummaryIcon is a small mark after a row's title.
type SummaryIcon struct{ Name, Label string }

// Summary is what one row of a list of items shows: mail, notes or feeds.
type Summary struct {
	Title, Date, Subtitle, Preview string
	// Unread draws the dot in the row's gutter.
	Unread bool
	// Mark, when not zero, draws a flag of this color in the gutter;
	// MarkLabel names it.
	Mark      ui.Color
	MarkLabel string
	// Icons follow the title.
	Icons []SummaryIcon
	// Count is a badge at the end of the subtitle's line, "" for none.
	Count string
	// Photo, when set, is drawn in a circle before the text.
	Photo *ui.Bitmap
	// Actions draws the row's buttons while the pointer is over it. They
	// duplicate the row's commands, so assistive technology does not see
	// them.
	Actions func()
}

// SummaryRow draws a list row of a fixed height: the title and the date,
// the subtitle, and up to 2 lines of preview. A row with no title yet, as
// one still loading, keeps the height and shows nothing.
func SummaryRow(c *ui.Context, s Summary) ui.Element {
	t := c.Theme()
	row := ui.Row(c).Height(SummaryHeight).Padding(frostui.SpaceS, frostui.SpaceM).Gap(frostui.SpaceS).AlignItems(ui.Start)
	hovered := row.Hovered()
	row.Children(func() {
		ui.Column(c).Width(10).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Height(18).Center().Children(func() {
				if s.Unread {
					ui.Box(c).Size(8, 8).Radius(4).Background(t.Accent).Label("Unread")
				}
			})
			if s.MarkLabel != "" {
				frostui.Icon(c, "mail-mark-important").FontSize(10).TextColor(s.Mark).Label(s.MarkLabel)
			}
		})
		if s.Photo != nil {
			ui.Avatar(c, s.Title, s.Photo).Size(32, 32)
		}
		ui.Column(c).Grow(1).Gap(2).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Strong(c, s.Title).Grow(1).SingleLine()
				for _, ic := range s.Icons {
					frostui.Icon(c, ic.Name).FontSize(12).TextColor(t.TextMuted).Label(ic.Label)
				}
				frostui.Caption(c, s.Date).SingleLine()
			})
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Body(c, s.Subtitle).Grow(1).SingleLine()
				if s.Count != "" {
					ui.Badge(c, s.Count)
				}
			})
			frostui.Muted(c, s.Preview).MaxLines(2)
		})
		if s.Actions != nil && hovered {
			ui.Row(c).Absolute().Right(frostui.SpaceS).Bottom(frostui.SpaceS).Gap(frostui.SpaceXS).AccessHidden(true).Children(s.Actions)
		}
	})
	return row
}
