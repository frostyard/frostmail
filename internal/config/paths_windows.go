package config

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// localAppData is %LOCALAPPDATA%\Frostmail, where the app is installed
// (Tauri's per-user default), with maild's data in its Data folder and
// parts in its Cache folder. Local app data does not roam, so SQLite stays
// on a local disk (ADR-0038).
func localAppData(getenv func(string) string) (string, error) {
	dir := getenv("LOCALAPPDATA")
	if !filepath.IsAbs(dir) {
		return "", errors.New("LOCALAPPDATA is not set")
	}
	return filepath.Join(dir, "Frostmail"), nil
}

func defaultDataDir(getenv func(string) string) (string, error) {
	dir, err := localAppData(getenv)
	return filepath.Join(dir, "Data"), err
}

func defaultCacheDir(getenv func(string) string) (string, error) {
	dir, err := localAppData(getenv)
	return filepath.Join(dir, "Cache"), err
}

// DefaultSocket is maild's named pipe when FROSTMAIL_SOCKET is not set:
// \\.\pipe\frostmail-<the user's SID>, one per Windows user.
func DefaultSocket(func(string) string) (string, error) {
	sid, err := UserSID()
	if err != nil {
		return "", err
	}
	return `\\.\pipe\frostmail-` + sid, nil
}

// AppConfigFile is the app's own configuration, app.json:
// %APPDATA%\Frostmail\app.json (app/src-tauri/src/paths.rs, config_dir).
func AppConfigFile(getenv func(string) string) (string, error) {
	dir := getenv("APPDATA")
	if !filepath.IsAbs(dir) {
		return "", errors.New("APPDATA is not set")
	}
	return filepath.Join(dir, "Frostmail", "app.json"), nil
}

// UserSID is the SID of the user the process runs as, in string form.
func UserSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("the process's user: %w", err)
	}
	return user.User.Sid.String(), nil
}
