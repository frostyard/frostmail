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
	// Unread draws the dot in the row's gutter; UnreadLabel names it.
	Unread      bool
	UnreadLabel string
	// Mark, when not zero, draws a flag of this color in the gutter;
	// MarkLabel names it.
	Mark      ui.Color
	MarkLabel string
	// Icons follow the title.
	Icons []SummaryIcon
	// Count is a pill at the end of the subtitle's line, "" for none.
	Count string
	// Photo, when set, is drawn in a square before the text.
	Photo *ui.Bitmap
	// Selected marks the row as chosen with a 1px accent rule on its
	// leading edge, over the tint SummaryList gives chosen rows.
	Selected bool
	// Actions draws the row's buttons while the pointer is over it. They
	// duplicate the row's commands, so assistive technology does not see
	// them.
	Actions func()
}

// SummaryList draws a ui.List of n summary rows, each drawn by row with
// SummaryRow, and returns the list. Its chosen rows take the soft
// selection (frostui.SoftSelection) with square corners, as the brand's
// message list has them, and keep their own text colors; the rows build
// in the theme around the list, so that their accent (the unread dot)
// stays the accent rather than the selection's tint.
func SummaryList(c *ui.Context, st *ui.ListState, n int, row func(i int)) ui.Element {
	page := c.Theme()
	soft := frostui.SoftSelection(page)
	soft.Radius = 0
	var list ui.Element
	ui.Themed(c, soft).Background(ui.Transparent).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		list = ui.List(c, st, n, func(i int) {
			ui.Themed(c, page).Background(ui.Transparent).AlignItems(ui.Stretch).Children(func() { row(i) })
		}).Grow(1)
	})
	return list
}

// SummaryRow draws a list row of a fixed height: the title and the date,
// the subtitle, and up to 2 lines of preview, over a hairline. A row with
// no title yet, as one still loading, keeps the height and shows nothing.
// Nothing it holds draws outside it: its text column can be narrower than
// its text, which then wraps or ends with an ellipsis, and the row clips
// what still would (frostyard/frostmail#267).
func SummaryRow(c *ui.Context, s Summary) ui.Element {
	t := c.Theme()
	row := ui.Row(c).Height(SummaryHeight).Padding(frostui.SpaceS, frostui.SpaceM).Gap(frostui.SpaceS).AlignItems(ui.Start).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Clip()
	hovered := row.Hovered()
	row.Children(func() {
		if s.Selected {
			ui.Box(c).Absolute().Left(0).Top(0).Bottom(0).Width(1).Background(t.Accent).IgnoreInput(true)
		}
		ui.Column(c).Width(10).Shrink(0).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Height(18).Center().Children(func() {
				if s.Unread {
					Dot(c, 8, t.Accent).Label(s.UnreadLabel)
				}
			})
			if s.MarkLabel != "" {
				frostui.Icon(c, "mail-mark-important").FontSize(frostui.TextSize(c, 10)).TextColor(s.Mark).Label(s.MarkLabel)
			}
		})
		if s.Photo != nil {
			ui.Avatar(c, s.Title, s.Photo).Size(32, 32).Radius(t.Radius)
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Strong(c, s.Title).Grow(1).Shrink(1).MinWidth(0).SingleLine()
				for _, ic := range s.Icons {
					frostui.Icon(c, ic.Name).FontSize(frostui.TextSize(c, 12)).TextColor(t.TextMuted).Label(ic.Label)
				}
				frostui.Caption(c, s.Date).SingleLine()
			})
			ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Body(c, s.Subtitle).Grow(1).Shrink(1).MinWidth(0).SingleLine()
				if s.Count != "" {
					frostui.Pill(c, s.Count, frostui.Neutral)
				}
			})
			frostui.Muted(c, s.Preview).Shrink(1).MinWidth(0).MaxLines(2)
		})
		if s.Actions != nil && hovered {
			ui.Row(c).Absolute().Right(frostui.SpaceS).Bottom(frostui.SpaceS).Gap(frostui.SpaceXS).AccessHidden(true).Children(s.Actions)
		}
	})
	return row
}
