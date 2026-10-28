// Package config resolves where maild keeps its files (docs/design/overview.md#configuration).
package config

import "path/filepath"

// AppID is the reverse-DNS application ID (frostyard/core ADR-0016).
const AppID = "org.frostyard.Frostmail"

// InFlatpak reports whether the process runs in this application's Flatpak
// (FLATPAK_ID is AppID). Another application's Flatpak, such as an editor
// that runs mailctl, is not.
func InFlatpak(getenv func(string) string) bool { return getenv("FLATPAK_ID") == AppID }

// Paths are maild's file locations.
type Paths struct {
	DataDir  string // database and blob store; must be on a local filesystem
	CacheDir string // decoded message parts served to the UI
	Socket   string // the RPC socket (a named pipe on Windows)
	DB       string // DataDir/frostmail.db
	Blobs    string // DataDir/blobs
}

// Resolve computes Paths from the environment: the XDG base directories on
// Linux, the user's local app data and a per-user named pipe on Windows
// (ADR-0038). FROSTMAIL_DATA_DIR, FROSTMAIL_CACHE_DIR and FROSTMAIL_SOCKET
// override the defaults, for tests and for running a second maild.
func Resolve(getenv func(string) string) (Paths, error) {
	var p Paths
	var err error
	if p.DataDir = getenv("FROSTMAIL_DATA_DIR"); p.DataDir == "" {
		if p.DataDir, err = defaultDataDir(getenv); err != nil {
			return Paths{}, err
		}
	}
	if p.CacheDir = getenv("FROSTMAIL_CACHE_DIR"); p.CacheDir == "" {
		if p.CacheDir, err = defaultCacheDir(getenv); err != nil {
			return Paths{}, err
		}
	}
	if p.Socket = getenv("FROSTMAIL_SOCKET"); p.Socket == "" {
		if p.Socket, err = DefaultSocket(getenv); err != nil {
			return Paths{}, err
		}
	}
	p.DB = filepath.Join(p.DataDir, "frostmail.db")
	p.Blobs = filepath.Join(p.DataDir, "blobs")
	return p, nil
}
