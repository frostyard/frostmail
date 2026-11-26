package settings

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// closeToTrayFile, beside app.json, says this computer closes the main
// window to the tray. The file exists = on; it is this computer's
// preference, not maild's, which lodge shares with every client.
const closeToTrayFile = "close-to-tray"

func closeToTrayOn(configFile string) bool {
	return configFile != "" && fileExists(filepath.Join(filepath.Dir(configFile), closeToTrayFile))
}

// TrayPreference reports the persisted close-to-tray choice on this desktop.
func TrayPreference(configFile string) bool { return closeToTrayOn(configFile) }

// WriteTrayPreference persists the close-to-tray choice next to app.json.
func WriteTrayPreference(configFile string, on bool) error {
	if configFile == "" {
		// No app.json means no place for the file: the setting holds for
		// this run only, and nothing is written.
		return nil
	}
	path := filepath.Join(filepath.Dir(configFile), closeToTrayFile)
	var err error
	if on {
		err = os.MkdirAll(filepath.Dir(path), 0o700)
		if err == nil {
			err = os.WriteFile(path, nil, 0o600)
		}
	} else if err = os.Remove(path); errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	return err
}
