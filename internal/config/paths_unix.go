//go:build !windows

package config

import (
	"errors"
	"path/filepath"
)

// xdg is an XDG base directory: the variable when it is absolute, else
// fallback under HOME.
func xdg(getenv func(string) string, name, fallback string) (string, error) {
	if v := getenv(name); filepath.IsAbs(v) {
		return v, nil
	}
	home := getenv("HOME")
	if home == "" {
		return "", errors.New("HOME is not set")
	}
	return filepath.Join(home, fallback), nil
}

func defaultDataDir(getenv func(string) string) (string, error) {
	dir, err := xdg(getenv, "XDG_DATA_HOME", ".local/share")
	return filepath.Join(dir, "frostmail"), err
}

func defaultCacheDir(getenv func(string) string) (string, error) {
	dir, err := xdg(getenv, "XDG_CACHE_HOME", ".cache")
	return filepath.Join(dir, "frostmail"), err
}

// DefaultSocket is maild's socket when FROSTMAIL_SOCKET is not set:
// $XDG_RUNTIME_DIR/frostmail/maild.sock. In the Flatpak (InFlatpak) it is
// $XDG_RUNTIME_DIR/app/org.frostyard.Frostmail/frostmail/maild.sock: Flatpak
// shares $XDG_RUNTIME_DIR/app/<id> between every sandbox of the app, and the
// rest of $XDG_RUNTIME_DIR is private to each sandbox, so the window, maild
// and mailctl meet only there (docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md).
// app/src-tauri/src/paths.rs must stay in step with this function.
func DefaultSocket(getenv func(string) string) (string, error) {
	run := getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(run) {
		return "", errors.New("XDG_RUNTIME_DIR is not set; set FROSTMAIL_SOCKET")
	}
	if InFlatpak(getenv) {
		run = filepath.Join(run, "app", AppID)
	}
	return filepath.Join(run, "frostmail", "maild.sock"), nil
}

// AppConfigFile is the app's own configuration, app.json:
// $XDG_CONFIG_HOME/frostmail/app.json, under HOME/.config without it. In
// the Flatpak that is ~/.var/app/org.frostyard.Frostmail/config/frostmail.
// app/src-tauri/src/paths.rs (config_dir) must stay in step with it.
func AppConfigFile(getenv func(string) string) (string, error) {
	dir, err := xdg(getenv, "XDG_CONFIG_HOME", ".config")
	return filepath.Join(dir, "frostmail", "app.json"), err
}
