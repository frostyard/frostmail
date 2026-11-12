package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// SoftList draws a ui.List of n rows, each drawn by row, and returns the
// list: a message list, a list of people or of tasks. Its chosen rows
// take the brand's soft selection (frostui.SoftSelection) with square
// corners and a 1px accent rule on their leading edge, and keep their own
// text colors. The rows build in the theme around the list, so the
// accent they draw with (an unread dot, a check) stays the accent rather
// than the selection's tint; a row need not change its colors when it is
// chosen.
func SoftList(c *ui.Context, st *ui.ListState, n int, row func(i int)) ui.Element {
	page := c.Theme()
	soft := frostui.SoftSelection(page)
	soft.Radius = 0
	var list ui.Element
	ui.Themed(c, soft).Background(ui.Transparent).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		list = ui.List(c, st, n).Grow(1).Rows(func(r ui.ListRow) {
			e := ui.Themed(c, page).Background(ui.Transparent).AlignItems(ui.Stretch)
			if r.Selected() {
				// Painted, not an element: the row keeps the same children,
				// and with them its menu, as it is chosen.
				e.DrawOver(func(p *ui.Painter, b ui.Rect) { p.Fill(ui.Rect{X: b.X, Y: b.Y, W: 1, H: b.H}, page.Accent, 0) })
			}
			e.Children(func() { row(r.Index) })
		})
	})
	return list
}
