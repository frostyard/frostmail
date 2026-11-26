package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/settings"
)

// The setting is this computer's file beside app.json: writing it on makes
// the file, a new read finds it, and writing it off removes it.
func TestCloseToTrayIsThisComputersPreference(t *testing.T) {
	config := filepath.Join(t.TempDir(), "frostmail", "app.json")
	file := filepath.Join(filepath.Dir(config), "close-to-tray")
	if settings.TrayPreference(config) {
		t.Error("close to tray is on by default")
	}
	if err := settings.WriteTrayPreference(config, true); err != nil {
		t.Fatal(err)
	}
	if !fileExists(file) || !settings.TrayPreference(config) {
		t.Error("the setting did not stay on")
	}
	if err := settings.WriteTrayPreference(config, false); err != nil {
		t.Fatal(err)
	}
	if fileExists(file) || settings.TrayPreference(config) {
		t.Error("the setting did not go off")
	}
	if err := settings.WriteTrayPreference(config, false); err != nil {
		t.Errorf("turning off a setting that is off: %v", err)
	}
}

// With no app.json there is no file to write: the setting holds for the
// run only.
func TestCloseToTrayOffWithoutAConfigFile(t *testing.T) {
	if err := settings.WriteTrayPreference("", true); err != nil {
		t.Fatal(err)
	}
	if settings.TrayPreference("") {
		t.Error("TrayPreference reports on with no app.json")
	}
}

// fileExists says whether a path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
