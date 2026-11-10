package settings

import (
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// option is one choice of a drop-down.
type option[T comparable] struct {
	value T
	label string
}

// choose draws a drop-down of options, showing current, named name, and
// calls change with a new choice. A value not among the options shows
// the first.
func choose[T comparable](c *ui.Context, name string, current T, options []option[T], disabled bool, change func(T)) ui.Element {
	labels := make([]string, len(options))
	shown := ""
	for i, o := range options {
		labels[i] = o.label
		if o.value == current {
			shown = o.label
		}
	}
	if shown == "" && len(labels) > 0 {
		shown = labels[0]
	}
	s := ui.Select(c, &shown, labels).Label(name).Disabled(disabled)
	if s.Changed() {
		for _, o := range options {
			if o.label == shown {
				change(o.value)
				break
			}
		}
	}
	return s
}

// alert shows a request's error, under what failed.
func alert(c *ui.Context, text string) {
	if text == "" {
		return
	}
	frostui.Problem(c, text).Role(ui.RoleAlert)
}

// note is a line of help under a control.
func note(c *ui.Context, text string) ui.Element {
	return frostui.Caption(c, text).Wrap()
}

// listRow is a row of a list beside a form: a title, a line under it when
// there is one, chosen or not. Key it with c.Key. A row's name is its
// title.
func listRow(c *ui.Context, title, subtitle string, chosen, warn bool) ui.Element {
	t := c.Theme()
	row := ui.ButtonBase(c).Padding(6, frostui.SpaceM).Justify(ui.Start).Cursor(ui.CursorPointer).Label(title)
	if chosen {
		row.Background(t.SurfaceHover).Current(ui.CurrentItem)
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			frostui.Strong(c, title).SingleLine()
			if subtitle != "" {
				line := frostui.Caption(c, subtitle).SingleLine()
				if warn {
					line.TextColor(t.Danger)
				}
			}
		})
	})
	return row
}

// listHeight is the most a list over a form takes before it scrolls: about
// 35% of the window's height (docs/specs/settings-ui.md, Window).
const listHeight = 196

// listBox holds the rows of a list over a form, with its name.
func listBox(c *ui.Context, name string, rows func()) ui.Element {
	t := c.Theme()
	return ui.Scroll(c).MaxHeight(listHeight).Radius(frostui.RadiusChip).Border(1, t.Border).Background(frostui.Panel(t)).
		Role(ui.RoleList).Label(name).Children(func() {
		ui.Column(c).AlignItems(ui.Stretch).Children(rows)
	})
}

// addRemove is the bar under a list: a button that adds, one that removes.
func addRemove(c *ui.Context, addLabel, removeLabel string, addOff, removeOff bool) (add, remove bool) {
	ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
		add = frostui.IconButton(c, "list-add", addLabel).Disabled(addOff).Clicked()
		remove = frostui.IconButton(c, "list-remove", removeLabel).Disabled(removeOff).Clicked()
	})
	return add, remove
}

// buttons is a form's row of buttons, at its end.
func buttons(c *ui.Context, fn func()) {
	ui.Row(c).Gap(frostui.SpaceS).Justify(ui.End).Children(fn)
}

// validEmail matches ^[^@\s]+@[^@\s]+$, as the forms' checks do.
func validEmail(s string) bool {
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || domain == "" || strings.Contains(domain, "@") {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsSpace)
}
