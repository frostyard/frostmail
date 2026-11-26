package settings

import (
	"os"
	"path/filepath"

	"github.com/frostyard/frostmail/internal/config"
)

// ServerFacts are the Server pane's local facts: no request to maild, and
// none to lodge, so the pane says the same while either is unreachable.
type ServerFacts struct {
	// Flatpak is true in this app's Flatpak: a change of mode stops or
	// starts its maild.
	Flatpak bool
	// LocalData is true when maild's data directory holds a database.
	LocalData bool
	// HostMail is true when a Frostmail installed outside Flatpak has mail
	// here, which the move note says how to bring in.
	HostMail bool
}

// FactsFrom reads the facts from the environment, as the shell did. Only
// the files' metadata is read: the database belongs to maild.
func FactsFrom(getenv func(string) string) ServerFacts {
	f := ServerFacts{Flatpak: config.InFlatpak(getenv)}
	if p, err := config.Resolve(func(key string) string {
		// The socket is not asked for: a missing runtime directory must
		// not hide the data directory.
		if key == "FROSTMAIL_SOCKET" {
			return "unused"
		}
		// A relative data directory names one in maild's own working
		// directory, which the app cannot know.
		if v := getenv(key); key != "FROSTMAIL_DATA_DIR" || filepath.IsAbs(v) {
			return v
		}
		return ""
	}); err == nil {
		f.LocalData = fileExists(p.DB)
	}
	// In the Flatpak XDG_DATA_HOME is the app's own, so only HOME says
	// where a host install keeps its mail; the development bundle sets
	// FROSTMAIL_SOCKET, because its maild is the host's.
	if home := getenv("HOME"); f.Flatpak && getenv("FROSTMAIL_SOCKET") == "" && filepath.IsAbs(home) {
		f.HostMail = fileExists(filepath.Join(home, ".local", "share", "frostmail", "frostmail.db"))
	}
	return f
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
