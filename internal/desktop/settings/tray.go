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

// CloseToTray reports this computer's setting: a close of the main window
// hides it in the tray, while a panel shows the icon (frostui's
// Tray.CloseToTray decides). It belongs to the UI thread.
func (w *Window) CloseToTray() bool { return w.closeToTray }

// SetTray says whether the app has a tray icon now. The app calls it with
// true once its icon exists: without one, the General pane has no
// close-to-tray row. It belongs to the UI thread.
func (w *Window) SetTray(shown bool) { w.trayShown = shown }

// saveCloseToTray writes this computer's file for the setting, or removes
// it. A failure puts the switch back and says why.
func (w *Window) saveCloseToTray() {
	on := w.closeToTray
	err := WriteTrayPreference(w.o.ConfigFile, on)
	if err != nil {
		w.closeToTray = !on
		w.toast("Could not save the tray setting", err)
	}
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
