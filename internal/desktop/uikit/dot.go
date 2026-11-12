package uikit

import "github.com/egoist/mygo/ui"

// Dot is a round mark size across, of color: an unread message, a
// calendar's color. The brand's corners are nearly square (frostui's
// RadiusChip and the theme's Radius), but a dot stays round.
func Dot(c *ui.Context, size float32, color ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Radius(size / 2).Background(color).Shrink(0)
}
