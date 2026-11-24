package main

import "github.com/nsgo-dev/nsgo/core"

// The choice of appearance is this device's, not the account's: it lives in
// the client's storage, not in maild's settings. No entry is the system's.
const appearanceKey = "appearance"

func savedAppearance(cx *core.Scope) core.Appearance {
	switch v, _ := cx.Storage().GetItem(appearanceKey); v {
	case "light":
		return core.AppearanceLight
	case "dark":
		return core.AppearanceDark
	}
	return core.AppearanceSystem
}

// saveAppearance keeps the choice the General pane made. The kit's control
// has already shown it.
func (w *workspace) saveAppearance(a core.Appearance) {
	w.appearance.Set(a)
	var err error
	switch a {
	case core.AppearanceLight:
		err = w.cx.Storage().SetItem(appearanceKey, "light")
	case core.AppearanceDark:
		err = w.cx.Storage().SetItem(appearanceKey, "dark")
	default:
		w.cx.Storage().RemoveItem(appearanceKey)
	}
	if err != nil {
		w.problem.Set(err.Error())
	}
}
