package mainwin

import (
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/internal/desktop/commands"
)

var keyNames = map[string]ui.Key{
	"a": ui.KeyA, "e": ui.KeyE, "f": ui.KeyF, "h": ui.KeyH, "j": ui.KeyJ, "k": ui.KeyK, "l": ui.KeyL,
	"n": ui.KeyN, "p": ui.KeyP, "r": ui.KeyR, "s": ui.KeyS, "t": ui.KeyT, "u": ui.KeyU, "z": ui.KeyZ,
	"1": ui.Key1, "2": ui.Key2, "3": ui.Key3, "4": ui.Key4, ",": ui.KeyComma, " ": ui.KeySpace,
	"arrowleft": ui.KeyLeft, "arrowright": ui.KeyRight, "arrowup": ui.KeyUp, "arrowdown": ui.KeyDown,
	"home": ui.KeyHome, "end": ui.KeyEnd, "delete": ui.KeyDelete, "backspace": ui.KeyBackspace,
	"escape": ui.KeyEscape, "enter": ui.KeyEnter, "tab": ui.KeyTab, "f10": ui.KeyF10, "contextmenu": ui.KeyContextMenu,
}

// keyOf maps the command table's key to MyGo's.
func keyOf(k commands.Key) (ui.Modifiers, ui.Key, bool) {
	key, ok := keyNames[k.Name]
	if !ok {
		return 0, 0, false
	}
	var mods ui.Modifiers
	if k.Ctrl {
		mods |= ui.Cmd
	}
	if k.Alt {
		mods |= ui.Alt
	}
	if k.Shift {
		mods |= ui.Shift
	}
	return mods, key, true
}
