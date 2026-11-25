//go:build !js

package main

import (
	"os"

	"github.com/frostyard/frostmail/internal/desktop/settings"
)

// hostMailFound reports whether this is the release Flatpak beside a host
// install's mail: the facts the Server pane reads, which read the file's
// metadata alone.
func hostMailFound() bool {
	facts := settings.FactsFrom(os.Getenv)
	return facts.Flatpak && facts.HostMail
}
