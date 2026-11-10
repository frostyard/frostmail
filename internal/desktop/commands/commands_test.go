package commands_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/commands"
)

// key parses "Ctrl+Shift+U" the way app/src/lib/keymap.test.ts does.
func key(spec string) commands.Key {
	parts := strings.Split(spec, "+")
	name := parts[len(parts)-1]
	mods := parts[:len(parts)-1]
	if name == "Space" {
		name = " "
	}
	return commands.Key{Name: name, Ctrl: slices.Contains(mods, "Ctrl"), Alt: slices.Contains(mods, "Alt"),
		Shift: slices.Contains(mods, "Shift"), Meta: slices.Contains(mods, "Meta")}
}

// The cases of keymap.test.ts (T-0034's contract), and the desktop's keys.
func TestForOutsideTextFields(t *testing.T) {
	for spec, want := range map[string]commands.Command{
		"ArrowUp": commands.Previous, "ArrowDown": commands.Next, "Shift+ArrowUp": commands.ExtendPrevious,
		"Shift+ArrowDown": commands.ExtendNext, "Home": commands.First, "End": commands.Last,
		"Ctrl+a": commands.SelectAll, "Ctrl+A": commands.SelectAll, "Space": commands.PageDown,
		"Shift+Space": commands.PageUp, "Delete": commands.Delete, "Backspace": commands.Delete,
		"Ctrl+Alt+a": commands.Archive, "Ctrl+Shift+U": commands.ToggleRead, "Ctrl+Shift+u": commands.ToggleRead,
		"Ctrl+Shift+L": commands.ToggleFlag, "Ctrl+Alt+f": commands.FocusSearch, "Ctrl+f": commands.Find,
		"Escape": commands.Escape, "Ctrl+Shift+N": commands.GetMail, "Ctrl+1": commands.AllInboxes,
		"Ctrl+,": commands.Settings, "Ctrl+Alt+s": commands.ToggleSidebar, "Tab": commands.NextPane,
		"Shift+Tab": commands.PreviousPane, "ContextMenu": commands.ContextMenu, "Shift+F10": commands.ContextMenu,
		"Ctrl+n": commands.Compose, "Ctrl+r": commands.Reply, "Ctrl+Shift+R": commands.ReplyAll,
		"Ctrl+Shift+F": commands.Forward, "Enter": commands.Open,
		"Ctrl+k": commands.Palette, "Ctrl+Shift+P": commands.Palette, "Ctrl+j": commands.Activity,
		"Ctrl+Shift+J": commands.Junk,
	} {
		if got, ok := commands.For(key(spec), false); !ok || got != want {
			t.Errorf("%s = %q, %v; want %s", spec, got, ok, want)
		}
	}
	for _, spec := range []string{"a", "Ctrl+Shift+a", "Alt+ArrowUp", "Ctrl+ArrowDown", "Ctrl+Delete", "Shift+Delete",
		"Ctrl+Alt+Shift+a", "Meta+a", "Meta+ArrowDown", "Ctrl+Meta+1", "Ctrl+5", "Ctrl+Alt+n", "Ctrl+Alt+r", "F10", "Alt+Tab"} {
		if got, ok := commands.For(key(spec), false); ok {
			t.Errorf("%s = %s, want nothing", spec, got)
		}
	}
}

func TestForInATextField(t *testing.T) {
	for spec, want := range map[string]commands.Command{
		"Escape": commands.Escape, "Tab": commands.NextPane, "Shift+Tab": commands.PreviousPane,
		"Ctrl+Shift+N": commands.GetMail, "Ctrl+1": commands.AllInboxes, "Ctrl+,": commands.Settings,
		"Ctrl+Alt+f": commands.FocusSearch, "Ctrl+Alt+s": commands.ToggleSidebar, "Ctrl+Shift+U": commands.ToggleRead,
		"Ctrl+Shift+F": commands.Forward, "Ctrl+k": commands.Palette, "Ctrl+Shift+P": commands.Palette,
		"Ctrl+j": commands.Activity,
	} {
		if got, ok := commands.For(key(spec), true); !ok || got != want {
			t.Errorf("%s in a field = %q, %v; want %s", spec, got, ok, want)
		}
	}
	for _, spec := range []string{"ArrowUp", "ArrowDown", "Home", "End", "Space", "Delete", "Backspace",
		"Ctrl+a", "Ctrl+f", "ContextMenu", "Shift+ArrowDown", "Enter", "Ctrl+n", "Ctrl+r"} {
		if got, ok := commands.For(key(spec), true); ok {
			t.Errorf("%s in a field = %s; it belongs to the field", spec, got)
		}
	}
}

func TestShortcutText(t *testing.T) {
	for c, want := range map[commands.Command]string{
		commands.Delete: "Delete, Backspace", commands.Archive: "Ctrl+Alt+A", commands.PageDown: "Space",
		commands.Palette: "Ctrl+K, Ctrl+Shift+P", commands.PreviousPeriod: "Ctrl+←", commands.ContextMenu: "Menu, Shift+F10",
		commands.Shortcuts: "",
	} {
		if got := commands.ShortcutText(c); got != want {
			t.Errorf("ShortcutText(%s) = %q, want %q", c, got, want)
		}
	}
}

func TestItemsNameAndDisable(t *testing.T) {
	items := commands.Items(commands.Mail, 0)
	byCommand := map[commands.Command]commands.Item{}
	for _, it := range items {
		byCommand[it.Command] = it
	}
	if it := byCommand[commands.Compose]; it.Name != "New Message" || it.Reason != "" {
		t.Errorf("compose in Mail = %+v", it)
	}
	if it := byCommand[commands.Reply]; it.Reason != "Select a message first" {
		t.Errorf("reply with nothing selected = %+v", it)
	}
	if _, ok := byCommand[commands.DayView]; ok {
		t.Error("Mail lists Day View")
	}
	if items[0].Command != commands.Compose {
		t.Errorf("first item = %s, want the entries' order", items[0].Command)
	}
	two := map[commands.Command]commands.Item{}
	for _, it := range commands.Items(commands.Mail, 2) {
		two[it.Command] = it
	}
	if two[commands.Reply].Reason != "" || two[commands.Print].Reason != "Select one message first" {
		t.Errorf("with 2 selected: reply %+v, print %+v", two[commands.Reply], two[commands.Print])
	}
	cal := commands.Items(commands.Calendar, 0)
	if !slices.ContainsFunc(cal, func(it commands.Item) bool { return it.Command == commands.Compose && it.Name == "New Event" }) {
		t.Error("Calendar has no New Event")
	}
	if !slices.ContainsFunc(cal, func(it commands.Item) bool {
		return it.Command == commands.Delete && it.Reason == "Select an event first"
	}) {
		t.Error("Calendar's Delete does not ask for an event")
	}
}

// Every bound command but the moves has a palette entry, and every entry
// names at least one module and a group.
func TestEveryCommandHasItsPlace(t *testing.T) {
	moves := []commands.Command{commands.Previous, commands.Next, commands.ExtendPrevious, commands.ExtendNext,
		commands.First, commands.Last, commands.PageDown, commands.PageUp, commands.Left, commands.Right,
		commands.NextPane, commands.PreviousPane, commands.Open, commands.Escape, commands.ContextMenu, commands.Palette}
	listed := map[commands.Command]bool{}
	for _, e := range commands.Entries {
		if listed[e.Command] {
			t.Errorf("%s is listed twice", e.Command)
		}
		listed[e.Command] = true
		if len(e.Names) == 0 || e.Group == "" {
			t.Errorf("%s has no name or no group", e.Command)
		}
	}
	for _, b := range commands.Bindings {
		if !listed[b.Command] && !slices.Contains(moves, b.Command) {
			t.Errorf("%s (%s) has a key but no palette entry", b.Command, b.Key)
		}
	}
}
