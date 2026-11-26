// Package icons holds the app's own icon for its tray: the panel shows the
// app as its launcher does.
package icons

import _ "embed"

// trayPNG is the tray icon: the app's own icon. TestTrayIconIsTheAppsIcon
// keeps it in step with packaging/icons/128x128.png.
//
//go:embed tray.png
var trayPNG []byte

// Tray returns the tray icon, a PNG in color, 128 pixels on a side. The
// slice is shared: callers must not change it.
func Tray() []byte { return trayPNG }
