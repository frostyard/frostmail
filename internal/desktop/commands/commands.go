// Package commands is the desktop app's list of commands
// (docs/specs/ui.md, Keyboard map and Command palette; plan 0015): every
// command's ID, keys, names in the modules where it acts, group, and what
// it needs. Table turns it into frostui's commands, whose CommandTable
// then binds the keys, fills the palette and builds the menus, so a
// command has one ID, one name and one set of keys.
package commands

import (
	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
)

// Command is what a shortcut, a menu item or the palette asks the focused
// pane or the app to do.
type Command string

// The commands, as app/src/lib/keymap.ts names them, and the desktop's
// own. The palette's and the console's come from frostui's shell.
const (
	DayView        Command = "dayView"
	WeekView       Command = "weekView"
	MonthView      Command = "monthView"
	Today          Command = "today"
	PreviousPeriod Command = "previousPeriod"
	NextPeriod     Command = "nextPeriod"
	Left           Command = "left"
	Right          Command = "right"
	Previous       Command = "previous"
	Next           Command = "next"
	ExtendPrevious Command = "extendPrevious"
	ExtendNext     Command = "extendNext"
	First          Command = "first"
	Last           Command = "last"
	SelectAll      Command = "selectAll"
	PageDown       Command = "pageDown"
	PageUp         Command = "pageUp"
	Delete         Command = "delete"
	Undo           Command = "undo"
	Archive        Command = "archive"
	Junk           Command = "junk"
	ToggleRead     Command = "toggleRead"
	ToggleFlag     Command = "toggleFlag"
	FocusSearch    Command = "focusSearch"
	Find           Command = "find"
	Escape         Command = "escape"
	GetMail        Command = "getMail"
	AllInboxes     Command = "allInboxes"
	ShowCalendar   Command = "showCalendar"
	ShowPeople     Command = "showPeople"
	ShowTasks      Command = "showTasks"
	ToggleSidebar  Command = "toggleSidebar"
	NextPane       Command = "nextPane"
	PreviousPane   Command = "previousPane"
	ContextMenu    Command = "contextMenu"
	Open           Command = "open"
	Compose        Command = "compose"
	Reply          Command = "reply"
	ReplyAll       Command = "replyAll"
	Forward        Command = "forward"
	Redirect       Command = "redirect"
	Print          Command = "print"
	RawSource      Command = "rawSource"
	AllHeaders     Command = "allHeaders"
	Settings       Command = "settings"
	Shortcuts      Command = "shortcuts"
	About          Command = "about"
)

// Binding binds a key to a command.
type Binding struct {
	frostui.KeyBinding
	Command Command
}

// in binds a key that still acts while a text field has the keyboard;
// out one the field keeps. The text field's rule of docs/specs/ui.md:
// Escape, Tab, Ctrl+1–4, Ctrl+, and Ctrl with Alt or Shift act; arrows,
// Space, Delete, Backspace, Ctrl+A and Ctrl+F stay with the field.
func in(mods ui.Modifiers, key ui.Key, c Command) Binding {
	return Binding{frostui.KeyBinding{Mods: mods, Key: key, Scope: frostui.Everywhere}, c}
}

func out(mods ui.Modifiers, key ui.Key, c Command) Binding {
	return Binding{frostui.KeyBinding{Mods: mods, Key: key, Scope: frostui.OutsideText}, c}
}

// Bindings are the keyboard map, in the order a key is looked up.
var Bindings = []Binding{
	out(ui.Cmd, ui.KeyZ, Undo),
	in(ui.Cmd|ui.Alt, ui.Key1, DayView),
	in(ui.Cmd|ui.Alt, ui.Key2, WeekView),
	in(ui.Cmd|ui.Alt, ui.Key3, MonthView),
	out(ui.Cmd, ui.KeyT, Today),
	out(ui.Cmd, ui.KeyLeft, PreviousPeriod),
	out(ui.Cmd, ui.KeyRight, NextPeriod),
	out(0, ui.KeyLeft, Left),
	out(0, ui.KeyRight, Right),
	out(0, ui.KeyUp, Previous),
	out(0, ui.KeyDown, Next),
	out(ui.Shift, ui.KeyUp, ExtendPrevious),
	out(ui.Shift, ui.KeyDown, ExtendNext),
	out(0, ui.KeyHome, First),
	out(0, ui.KeyEnd, Last),
	out(ui.Cmd, ui.KeyA, SelectAll),
	out(0, ui.KeySpace, PageDown),
	out(ui.Shift, ui.KeySpace, PageUp),
	out(0, ui.KeyDelete, Delete),
	out(0, ui.KeyBackspace, Delete),
	in(ui.Cmd|ui.Alt, ui.KeyA, Archive),
	in(ui.Cmd|ui.Shift, ui.KeyJ, Junk),
	in(ui.Cmd|ui.Shift, ui.KeyU, ToggleRead),
	in(ui.Cmd|ui.Shift, ui.KeyL, ToggleFlag),
	in(ui.Cmd|ui.Alt, ui.KeyF, FocusSearch),
	out(ui.Cmd, ui.KeyF, Find),
	in(0, ui.KeyEscape, Escape),
	in(ui.Cmd|ui.Shift, ui.KeyN, GetMail),
	in(ui.Cmd, ui.Key2, ShowCalendar),
	in(ui.Cmd, ui.Key3, ShowPeople),
	in(ui.Cmd, ui.Key4, ShowTasks),
	in(ui.Cmd, ui.Key1, AllInboxes),
	in(ui.Cmd|ui.Alt, ui.KeyS, ToggleSidebar),
	in(0, ui.KeyTab, NextPane),
	in(ui.Shift, ui.KeyTab, PreviousPane),
	out(0, ui.KeyContextMenu, ContextMenu),
	out(ui.Shift, ui.KeyF10, ContextMenu),
	out(0, ui.KeyEnter, Open),
	out(ui.Cmd, ui.KeyN, Compose),
	out(ui.Cmd, ui.KeyR, Reply),
	in(ui.Cmd|ui.Shift, ui.KeyR, ReplyAll),
	in(ui.Cmd|ui.Shift, ui.KeyF, Forward),
	in(ui.Cmd|ui.Shift, ui.KeyE, Redirect),
	out(ui.Cmd, ui.KeyP, Print),
	in(ui.Cmd|ui.Alt, ui.KeyU, RawSource),
	in(ui.Cmd|ui.Shift, ui.KeyH, AllHeaders),
	in(ui.Cmd, ui.KeyComma, Settings),
}

// Keys are a command's keys, in the map's order.
func Keys(c Command) []frostui.KeyBinding {
	var keys []frostui.KeyBinding
	for _, b := range Bindings {
		if b.Command == c {
			keys = append(keys, b.KeyBinding)
		}
	}
	return keys
}
