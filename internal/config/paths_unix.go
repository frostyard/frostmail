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
// $XDG_RUNTIME_DIR/frostmail/maild.sock.
func DefaultSocket(getenv func(string) string) (string, error) {
	run := getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(run) {
		return "", errors.New("XDG_RUNTIME_DIR is not set; set FROSTMAIL_SOCKET")
	}
	return filepath.Join(run, "frostmail", "maild.sock"), nil
}
