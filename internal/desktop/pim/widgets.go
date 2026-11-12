package pim

import (
	"hash/fnv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// flagColors are the flags' colors, light and dark (docs/specs/ui.md,
// Tokens: Flags).
var flagColors = [7][2]string{
	{"#ff3b30", "#ff453a"}, {"#ff9500", "#ff9f0a"}, {"#ffcc00", "#ffd60a"}, {"#34c759", "#30d158"},
	{"#007aff", "#0a84ff"}, {"#af52de", "#bf5af2"}, {"#8e8e93", "#98989d"},
}

func flagColor(t *ui.Theme, color int64) ui.Color {
	if color < 1 || color > 7 {
		color = 1
	}
	shade := 0
	if t.Dark {
		shade = 1
	}
	return ui.Hex(flagColors[color-1][shade])
}

func tertiary(t *ui.Theme) ui.Color { return t.TextMuted.Alpha(0.7) }

// calColor is a collection's color, else the accent.
func calColor(t *ui.Theme, hex string) ui.Color {
	if hex == "" {
		return t.Accent
	}
	return ui.Hex(hex)
}

// paneDot is the color dot before a pane row's name, and paneDotRoom the
// room it and its gap take, which the row's account starts after.
const (
	paneDot     = 8
	paneDotRoom = paneDot + 6
)

// nameAndAccount is a pane row's name and the account it belongs to, on 2
// lines: the name, and the account's address under it. lead draws what
// comes before the name, as a calendar's color dot, and room is the width
// it and its gap take, so the address starts under the name
// (frostyard/frostmail#282).
//
// The address has the line to itself: beside the name it took the name's
// width, and no flex shrink factor keeps a text whole while the row is
// narrower than its texts.
func nameAndAccount(c *ui.Context, t *ui.Theme, name, account string, lead func(), room float32) {
	ui.Column(c).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Tooltip(name + " · " + account).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if lead != nil {
				lead()
			}
			frostui.Body(c, name).Shrink(1).MinWidth(0).SingleLine()
		})
		ui.Column(c).Padding(0, 0, 0, room).Children(func() {
			ui.Text(c, account).FontSize(frostui.TextSize(c, 12)).TextColor(tertiary(t)).Shrink(1).MinWidth(0).SingleLine()
		})
	})
}

// sectionHeading is a block's heading: the brand's uppercase label, named
// as written.
func sectionHeading(c *ui.Context, text string) ui.Element {
	return frostui.Label(c, text).Role(ui.RoleHeading).SingleLine()
}

// avatarTones are the initials' fills, picked by an address.
var avatarTones = []string{"#ef4444", "#f59e0b", "#10b981", "#3b82f6", "#8b5cf6", "#ec4899", "#14b8a6"}

func avatarTone(email string) ui.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(email)))
	return ui.Hex(avatarTones[h.Sum32()%uint32(len(avatarTones))])
}

// avatar is a person's square, at the theme's Radius as the brand's
// avatars are: a photo, else initials on the address's tone.
func avatar(c *ui.Context, size float32, name, email string, photo *ui.Bitmap) ui.Element {
	if photo != nil {
		return ui.Image(c, photo).Size(size, size).Radius(c.Theme().Radius).Shrink(0).Fit(ui.Cover).Label(name)
	}
	label := name
	if label == "" {
		label = email
	}
	font := frostui.TextSize(c, 13)
	if size >= 64 {
		font = frostui.TextSize(c, 20)
	}
	return ui.Box(c).Size(size, size).Radius(c.Theme().Radius).Background(avatarTone(email)).Center().Shrink(0).AccessHidden(true).Children(func() {
		ui.Text(c, frostui.Initials(label)).FontSize(font).FontWeight(600).TextColor(ui.RGB(255, 255, 255)).SingleLine()
	})
}

// centered is an empty pane's line; size is at the platform's usual text
// size (frostui.TextSize).
func centered(c *ui.Context, text string, size float32) {
	t := c.Theme()
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Background(t.Background).Children(func() {
		ui.Text(c, text).FontSize(frostui.TextSize(c, size)).TextColor(t.TextMuted)
	})
}

// textButton is a link-styled button; size is at the platform's usual
// text size (frostui.TextSize).
func textButton(c *ui.Context, label string, color ui.Color, size float32) ui.Element {
	return ui.ButtonBase(c).Justify(ui.Start).Cursor(ui.CursorPointer).Label(label).Children(func() {
		ui.Text(c, label).FontSize(frostui.TextSize(c, size)).TextColor(color).Shrink(1).MinWidth(0).SingleLine()
	})
}

// addButton is a text button at the start of its column.
func addButton(c *ui.Context, label string, color ui.Color) ui.Element {
	return textButton(c, label, color, 12).AlignSelf(ui.Start)
}

// kvRow is a pane's label and value row.
func kvRow(c *ui.Context, label string, width float32, value func()) {
	ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Start).Children(func() {
		frostui.Muted(c, label).Width(width).Shrink(0).TextAlign(ui.End)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(value)
	})
}

// labeled is kvRow keyed by its label.
func labeled(c *ui.Context, label string, width float32, value func()) {
	kvRow(c.Key(label), label, width, value)
}

// sheetFrame draws a 420-wide question over a window: the time sheet's
// frame. It reports whether the sheet was dismissed.
func sheetFrame(c *ui.Context, title string, body func()) (dismissed bool) {
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(420).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Role(ui.RoleDialog).Label(title).Children(func() {
			frostui.Strong(c, title)
			body()
		})
	})
	return !open
}
