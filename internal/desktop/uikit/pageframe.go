package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// PageColor is the ground of every page Document makes, whatever the
// window's theme: pageStyle's white.
func PageColor() ui.Color { return ui.RGB(255, 255, 255) }

// PageFrame draws f, a page of HTML the app did not write, height high,
// as a sheet set in the window: the page keeps the white ground Document
// gives it, on which its colors, its security rules and its printing
// depend, and sits inside a hairline on the window's panel, with room
// around it, so that in a dark theme it reads as a page placed in the
// window rather than a hole in it. The box behind the page is white as
// well, so where the native view has not painted (before its first
// frame, or below a page shorter than its measured height) the window
// shows white rather than its dark ground (frostyard/frostmail#269). It
// returns the page's element, for its label and focus.
func PageFrame(c *ui.Context, f Frame, height float32) ui.Element {
	t := c.Theme()
	var page ui.Element
	ui.Column(c).Margin(0, frostui.SpaceXL, frostui.SpaceL, frostui.SpaceXL).Padding(frostui.SpaceM).
		Background(frostui.Panel(t)).Border(1, t.Border).Radius(frostui.RadiusCard).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Background(PageColor()).Border(1, t.Border).AlignItems(ui.Stretch).Children(func() {
			page = f.View(c).Height(height).AlignSelf(ui.Stretch)
		})
	})
	return page
}
