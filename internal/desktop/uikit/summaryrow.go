// Package uikit holds UI patterns meant for frostui (plan 0015). It
// imports only the standard library, MyGo, purego and frostui, so a
// pattern can move there by changing an import path.
package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// SummaryHeight is the height of a summary row with the usual 2 lines of
// preview.
const SummaryHeight = 84

// SummaryPreviewLines is how many lines of preview SummaryRow shows, and
// summaryLine the height of one.
const (
	SummaryPreviewLines = 2
	summaryLine         = 16
)

// SummaryHeightFor is the height of a summary row with lines of preview:
// SummaryHeight for 2, and a line's height more or less for each other.
func SummaryHeightFor(lines int) float32 {
	return SummaryHeight + float32(max(lines, 0)-SummaryPreviewLines)*summaryLine
}

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
	// Actions draws the row's buttons while the pointer is over it. They
	// duplicate the row's commands, so assistive technology does not see
	// them.
	Actions func()
}

// SummaryRow draws a list row of a fixed height: the title and the date,
// the subtitle, and up to 2 lines of preview, over a hairline. Draw a list
// of them with SoftList, which shows the choice. A row with
// no title yet, as one still loading, keeps the height and shows nothing.
// Nothing it holds draws outside it: its text column can be narrower than
// its text, which then wraps or ends with an ellipsis, and the row clips
// what still would (frostyard/frostmail#267).
func SummaryRow(c *ui.Context, s Summary) ui.Element {
	return SummaryRowLines(c, s, SummaryPreviewLines)
}

// SummaryRowLines is SummaryRow with lines of preview, none for 0: the
// row's height follows (SummaryHeightFor), the same for every row of a
// list whatever its text.
func SummaryRowLines(c *ui.Context, s Summary, lines int) ui.Element {
	t := c.Theme()
	row := ui.Row(c).Height(SummaryHeightFor(lines)).Padding(frostui.SpaceS, frostui.SpaceM).Gap(frostui.SpaceS).AlignItems(ui.Start).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Clip()
	hovered := row.Hovered()
	row.Children(func() {
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
			if lines > 0 {
				frostui.Muted(c, s.Preview).Shrink(1).MinWidth(0).MaxLines(lines)
			}
		})
		if s.Actions != nil && hovered {
			ui.Row(c).Absolute().Right(frostui.SpaceS).Bottom(frostui.SpaceS).Gap(frostui.SpaceXS).AccessHidden(true).Children(s.Actions)
		}
	})
	return row
}
