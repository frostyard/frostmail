// Package config resolves where maild keeps its files (docs/design/overview.md#configuration).
package config

import (
	"errors"
	"path/filepath"
)

// AppID is the reverse-DNS application ID (frostyard/core ADR-0016).
const AppID = "org.frostyard.Frostmail"

// Paths are maild's file locations.
type Paths struct {
	DataDir  string // database and blob store; must be on a local filesystem
	CacheDir string // decoded message parts served to the UI
	Socket   string // the RPC socket
	DB       string // DataDir/frostmail.db
	Blobs    string // DataDir/blobs
}

// Resolve computes Paths from the environment, following the XDG base
// directory spec. FROSTMAIL_DATA_DIR, FROSTMAIL_CACHE_DIR and FROSTMAIL_SOCKET
// override the defaults, for tests and for running a second maild.
func Resolve(getenv func(string) string) (Paths, error) {
	home := getenv("HOME")
	xdg := func(name, fallback string) (string, error) {
		if v := getenv(name); filepath.IsAbs(v) {
			return v, nil
		}
		if home == "" {
			return "", errors.New("HOME is not set")
		}
		return filepath.Join(home, fallback), nil
	}
	var p Paths
	var err error
	if p.DataDir = getenv("FROSTMAIL_DATA_DIR"); p.DataDir == "" {
		if p.DataDir, err = xdg("XDG_DATA_HOME", ".local/share"); err != nil {
			return Paths{}, err
		}
		p.DataDir = filepath.Join(p.DataDir, "frostmail")
	}
	if p.CacheDir = getenv("FROSTMAIL_CACHE_DIR"); p.CacheDir == "" {
		if p.CacheDir, err = xdg("XDG_CACHE_HOME", ".cache"); err != nil {
			return Paths{}, err
		}
		p.CacheDir = filepath.Join(p.CacheDir, "frostmail")
	}
	if p.Socket = getenv("FROSTMAIL_SOCKET"); p.Socket == "" {
		run := getenv("XDG_RUNTIME_DIR")
		if !filepath.IsAbs(run) {
			return Paths{}, errors.New("XDG_RUNTIME_DIR is not set; set FROSTMAIL_SOCKET")
		}
		p.Socket = filepath.Join(run, "frostmail", "maild.sock")
	}
	p.DB = filepath.Join(p.DataDir, "frostmail.db")
	p.Blobs = filepath.Join(p.DataDir, "blobs")
	return p, nil
}
