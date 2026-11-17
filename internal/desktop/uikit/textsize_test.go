package uikit_test

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// themeAfter runs adapt in a view and returns the theme the view ends with.
func themeAfter(prefs ui.Preferences, adapt func(c *ui.Context)) (theme ui.Theme, size float32) {
	tt := ui.NewTesterWith(func(c *ui.Context) {
		adapt(c)
		theme = *c.Theme()
		size = frostui.TextSize(c, 12)
		ui.Text(c, "x")
	}, 200, 100, ui.TesterOptions{Preferences: prefs})
	tt.Frame()
	return theme, size
}

// P-957: the app's text size scales the theme's font, so every frostui text
// size follows, in Frostyard's colors, and an own Adapt never compounds.
func TestAdaptScalesTheFontAndKeepsTheTheme(t *testing.T) {
	prefs := ui.Preferences{TextScale: 1}
	var base ui.Theme
	var baseSize float32
	base, baseSize = themeAfter(prefs, func(c *ui.Context) { frostui.Adapt(c) })
	usual := ui.LightTheme().FontSize
	if base.FontSize != usual || baseSize != 12 {
		t.Fatalf("frostui.Adapt: FontSize %v, TextSize(12) %v; want %v and 12", base.FontSize, baseSize, usual)
	}
	// Scale 1 is exactly frostui.Adapt's theme.
	one, _ := themeAfter(prefs, func(c *ui.Context) { uikit.Adapt(c, 1) })
	if one.FontSize != base.FontSize || one.Background != base.Background || one.Accent != base.Accent || one.Radius != base.Radius {
		t.Errorf("scale 1 changed the theme: %+v, want %+v", one, base)
	}
	for _, scale := range []float32{0.9, 1.15, 1.3} {
		want := usual * scale
		once, size := themeAfter(prefs, func(c *ui.Context) { uikit.Adapt(c, scale) })
		if once.FontSize != want {
			t.Errorf("scale %v: FontSize = %v, want %v", scale, once.FontSize, want)
		}
		if d := size - 12*scale; d > 0.001 || d < -0.001 {
			t.Errorf("scale %v: TextSize(12) = %v, want %v", scale, size, 12*scale)
		}
		if frostui.ColorsOf(&once) != frostui.ColorsOf(&base) || once.Background != base.Background {
			t.Errorf("scale %v: the Frostyard colors changed", scale)
		}
		twice, _ := themeAfter(prefs, func(c *ui.Context) { uikit.Adapt(c, scale); uikit.Adapt(c, scale) })
		if twice.FontSize != want {
			t.Errorf("scale %v: a second call gives FontSize %v, want %v", scale, twice.FontSize, want)
		}
		later, _ := themeAfter(prefs, func(c *ui.Context) { uikit.Adapt(c, scale); frostui.Adapt(c) })
		if later.FontSize != want {
			t.Errorf("scale %v: frostui.Adapt after gives FontSize %v, want %v", scale, later.FontSize, want)
		}
	}
}

// The desktop's own text size multiplies with the app's.
func TestAdaptMultipliesTheDesktopsTextScale(t *testing.T) {
	got, _ := themeAfter(ui.Preferences{TextScale: 1.5}, func(c *ui.Context) { uikit.Adapt(c, 1.15); uikit.Adapt(c, 1.15) })
	if want := ui.LightTheme().FontSize * 1.5 * 1.15; got.FontSize != want {
		t.Errorf("FontSize = %v, want %v", got.FontSize, want)
	}
	got, _ = themeAfter(ui.Preferences{TextScale: 1.5}, func(c *ui.Context) { uikit.Adapt(c, 1) })
	if want := ui.LightTheme().FontSize * 1.5; got.FontSize != want {
		t.Errorf("at scale 1, FontSize = %v, want the desktop's %v", got.FontSize, want)
	}
}
