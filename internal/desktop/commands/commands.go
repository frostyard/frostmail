// Package commands is the desktop app's one command table
// (docs/specs/ui.md, Keyboard map and Command palette; plan 0015): every
// command's keys, its names in the modules where it acts, its group, and
// what it needs. The keys, the menus and the palette all read it, so a
// command has one ID, one name and one set of keys. It imports no UI: the
// window maps Key to its toolkit's keys.
package commands

import (
	"regexp"
	"strings"
)

// Command is what a shortcut, a menu item or the palette asks the focused
// pane or the app to do.
type Command string

// The commands, as app/src/lib/keymap.ts names them, and the desktop's own
// (palette's Ctrl+K, the console, Keyboard Shortcuts, About).
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
	Palette        Command = "palette"
	Settings       Command = "settings"
	Activity       Command = "activity"
	Shortcuts      Command = "shortcuts"
	About          Command = "about"
)

// Key is a key press: the key's name as a browser's KeyboardEvent.key in
// lower case ("r", "arrowleft", " ", "f10", "contextmenu"), and its
// modifiers. Meta is never bound: a press with it maps to nothing.
type Key struct {
	Name                   string
	Ctrl, Alt, Shift, Meta bool
}

// Binding binds a key to a command.
type Binding struct {
	Key
	Command Command
}

func k(name string, ctrl, alt, shift bool, c Command) Binding {
	return Binding{Key{Name: name, Ctrl: ctrl, Alt: alt, Shift: shift}, c}
}

// Bindings are the keyboard map, in the order a key is looked up.
var Bindings = []Binding{
	k("z", true, false, false, Undo),
	k("1", true, true, false, DayView),
	k("2", true, true, false, WeekView),
	k("3", true, true, false, MonthView),
	k("t", true, false, false, Today),
	k("arrowleft", true, false, false, PreviousPeriod),
	k("arrowright", true, false, false, NextPeriod),
	k("arrowleft", false, false, false, Left),
	k("arrowright", false, false, false, Right),

	k("arrowup", false, false, false, Previous),
	k("arrowdown", false, false, false, Next),
	k("arrowup", false, false, true, ExtendPrevious),
	k("arrowdown", false, false, true, ExtendNext),
	k("home", false, false, false, First),
	k("end", false, false, false, Last),
	k("a", true, false, false, SelectAll),
	k(" ", false, false, false, PageDown),
	k(" ", false, false, true, PageUp),
	k("delete", false, false, false, Delete),
	k("backspace", false, false, false, Delete),
	k("a", true, true, false, Archive),
	k("j", true, false, true, Junk),
	k("u", true, false, true, ToggleRead),
	k("l", true, false, true, ToggleFlag),
	k("f", true, true, false, FocusSearch),
	k("f", true, false, false, Find),
	k("escape", false, false, false, Escape),
	k("n", true, false, true, GetMail),
	k("2", true, false, false, ShowCalendar),
	k("3", true, false, false, ShowPeople),
	k("4", true, false, false, ShowTasks),
	k("1", true, false, false, AllInboxes),
	k("s", true, true, false, ToggleSidebar),
	k("tab", false, false, false, NextPane),
	k("tab", false, false, true, PreviousPane),
	k("contextmenu", false, false, false, ContextMenu),
	k("f10", false, false, true, ContextMenu),
	k("enter", false, false, false, Open),
	k("n", true, false, false, Compose),
	k("r", true, false, false, Reply),
	k("r", true, false, true, ReplyAll),
	k("f", true, false, true, Forward),
	k("e", true, false, true, Redirect),
	k("p", true, false, false, Print),
	k("u", true, true, false, RawSource),
	k("h", true, false, true, AllHeaders),
	k(",", true, false, false, Settings),
	k("k", true, false, false, Palette), // the suite's palette key (plan 0015, shared rule 1)
	k("p", true, false, true, Palette),
	k("j", true, false, false, Activity), // the console, as ChairLift's
}

// allowedInTextField reports whether a binding still acts while a text
// field has the keyboard: Escape, Tab, Ctrl+1–4, Ctrl+, the window's Ctrl+K
// and Ctrl+J, and Ctrl with Alt or Shift. The field keeps arrows, Space,
// Delete, Backspace, Ctrl+A and Ctrl+F.
func allowedInTextField(b Binding) bool {
	switch {
	case b.Name == "escape" || b.Name == "tab":
		return true
	case b.Ctrl && !b.Alt && !b.Shift && strings.Contains("1234,kj", b.Name) && len(b.Name) == 1:
		return true
	}
	return b.Ctrl && (b.Alt || b.Shift)
}

// For maps a key press to its command; false for none.
func For(key Key, inTextField bool) (Command, bool) {
	if key.Meta {
		return "", false
	}
	key.Name = strings.ToLower(key.Name)
	for _, b := range Bindings {
		if b.Key == key {
			if inTextField && !allowedInTextField(b) {
				return "", false
			}
			return b.Command, true
		}
	}
	return "", false
}

// Keys are a command's bindings' keys, in the map's order.
func Keys(c Command) []Key {
	var out []Key
	for _, b := range Bindings {
		if b.Command == c {
			out = append(out, b.Key)
		}
	}
	return out
}

var keyText = map[string]string{
	" ": "Space", "arrowleft": "←", "arrowright": "→", "arrowup": "↑", "arrowdown": "↓",
	"delete": "Delete", "backspace": "Backspace", "escape": "Escape", "enter": "Enter", "tab": "Tab",
	"home": "Home", "end": "End", "f10": "F10", "contextmenu": "Menu",
}

var letter = regexp.MustCompile(`^[a-z]$`)

// Parts writes a key as the palette's key caps show it: Ctrl, Alt, Shift,
// then the key ({"Ctrl", "Alt", "A"}).
func (k Key) Parts() []string {
	var out []string
	if k.Ctrl {
		out = append(out, "Ctrl")
	}
	if k.Alt {
		out = append(out, "Alt")
	}
	if k.Shift {
		out = append(out, "Shift")
	}
	name := k.Name
	if t, ok := keyText[name]; ok {
		name = t
	} else if letter.MatchString(name) {
		name = strings.ToUpper(name)
	}
	return append(out, name)
}

// String writes a key as "Ctrl+Alt+A".
func (k Key) String() string { return strings.Join(k.Parts(), "+") }

// ShortcutText writes a command's keys as the palette's rows show them,
// joined by ", " ("Delete, Backspace"); "" for a command with none.
func ShortcutText(c Command) string {
	var out []string
	for _, k := range Keys(c) {
		out = append(out, k.String())
	}
	return strings.Join(out, ", ")
}
