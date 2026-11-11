// Package icons holds the few icons the desktop app draws that frostui's
// set has no name for: a star, a muted bell, a smart mailbox, and the
// flags in their seven colors. Everything else comes from the desktop's
// theme or frostui's embedded set, so the app has its icons on Windows
// too.
package icons

import "fmt"

// FlagPrefix starts the name of a flag's icon; the color's number, 1 to 7,
// follows.
const FlagPrefix = "frostmail-flag-"

// flagColors are the flags' colors (docs/specs/ui.md, Tokens: Flags), the
// same in light and dark for the sidebar's small marks.
var flagColors = [7]string{"#ff3b30", "#ff9500", "#ffcc00", "#34c759", "#007aff", "#af52de", "#8e8e93"}

const svgOpen = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">`

// Embedded returns the icons by name, for frostui.NewIcons.
func Embedded() map[string][]byte {
	out := map[string][]byte{
		"starred":                []byte(svgOpen + `<path d="M12 3l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3l-5.6 2.9 1.1-6.2L3 9.6l6.2-.9z" fill="currentColor"/></svg>`),
		"notifications-disabled": []byte(svgOpen + `<path d="M4 4l16 16M18 16V10a6 6 0 0 0-9.2-5M6.3 7.6A6 6 0 0 0 6 10v6l-2 2h14M10 21h4"/></svg>`),
		"folder-saved-search":    []byte(svgOpen + `<path d="M3 18V6a1 1 0 0 1 1-1h5l2 2h9a1 1 0 0 1 1 1v3M11 19H4a1 1 0 0 1-1-1"/><circle cx="17" cy="16" r="3"/><path d="M19.2 18.2 22 21"/></svg>`),
	}
	for i, color := range flagColors {
		out[fmt.Sprintf("%s%d", FlagPrefix, i+1)] = []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path d="M5 22V4c4-2 7 2 14 0v10c-7 2-10-2-14 0" fill="` + color + `" stroke="` + color + `" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	}
	return out
}
