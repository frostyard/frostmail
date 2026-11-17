package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// Adapt is frostui.Adapt with the app's own text size: scale times the
// desktop's (Preferences.TextScale) times the usual size. Every kit text
// size (frostui.TextSize) and MyGo's own text follow the theme's FontSize,
// so a window needs nothing more. At scale 1 it is frostui.Adapt. It sets
// the size from the usual one each time, never from the one it set, so
// calling it again in a frame, or frostui.Adapt after it (which keeps a
// Frostyard theme), leaves the size as it is. Call it first in a window's
// view: MyGo resets the theme at each build pass.
func Adapt(c *ui.Context, scale float32) *ui.Theme {
	t := frostui.Adapt(c)
	if scale <= 0 || scale == 1 {
		return t
	}
	desktop := c.Preferences().TextScale
	if desktop <= 0 {
		desktop = 1
	}
	scaled := *t
	scaled.FontSize = ui.LightTheme().FontSize * desktop * scale
	c.SetTheme(&scaled)
	return &scaled
}
