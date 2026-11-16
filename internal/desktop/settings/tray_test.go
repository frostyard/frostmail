package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/settings"
)

// trayFile is this computer's close-to-tray file, beside app.json.
func (f *fixture) trayFile() string {
	return filepath.Join(filepath.Dir(f.config), "close-to-tray")
}

// TestCloseToTrayRowNeedsATray: the row shows only once the app has an
// icon to close to, and it is off until chosen.
func TestCloseToTrayRowNeedsATray(t *testing.T) {
	f := newFixture(t, setup{})
	f.general()
	f.notSeen("Close to tray")
	f.notSeen("This Computer")
	f.win.SetTray(true)
	f.sees("This Computer")
	f.sees("Close to tray")
	if f.toggled("Close to tray") {
		t.Error("close to tray is on by default")
	}
	if f.win.CloseToTray() {
		t.Error("CloseToTray reports on by default")
	}
}

// TestCloseToTrayIsThisComputersPreference: the switch writes the file,
// a new window over the same app.json reads it, and turning it off
// removes the file.
func TestCloseToTrayIsThisComputersPreference(t *testing.T) {
	f := newFixture(t, setup{})
	f.win.SetTray(true)
	f.general()
	f.sees("Close to tray")
	f.toggle("Close to tray")
	f.until("the setting's file", func() bool { return fileExists(f.trayFile()) })
	if !f.win.CloseToTray() {
		t.Error("CloseToTray reports off after the switch was turned on")
	}
	// The setting is this computer's, so a new window reads it afresh.
	other := settings.New(settings.Options{Post: f.loop.Post, ConfigFile: f.config, Socket: "/local.sock"})
	if !other.CloseToTray() {
		t.Error("a new window reads close to tray as off")
	}
	f.toggle("Close to tray")
	f.until("the file gone", func() bool { return !fileExists(f.trayFile()) })
	if f.win.CloseToTray() {
		t.Error("CloseToTray reports on after the switch was turned off")
	}
	if f.toggled("Close to tray") {
		t.Error("the switch still shows on")
	}
}

// TestCloseToTrayOffWithoutAConfigFile: with no app.json there is no file
// to write.
func TestCloseToTrayOffWithoutAConfigFile(t *testing.T) {
	w := settings.New(settings.Options{Post: func(func()) {}, ConfigFile: ""})
	if w.CloseToTray() {
		t.Error("CloseToTray reports on with no app.json")
	}
	w.SetTray(true)
	if w.CloseToTray() {
		t.Error("SetTray turned the setting on")
	}
}

// fileExists says whether a path exists, for the tests' waits.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
