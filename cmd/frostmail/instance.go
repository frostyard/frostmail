package main

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"github.com/frostyard/frostmail/internal/config"
)

// sharedRuntimeDir is the directory for the single-instance socket, when
// $XDG_RUNTIME_DIR will not do; "" when it will. In the Flatpak each sandbox
// has its own $XDG_RUNTIME_DIR, and a notification's click starts the app in
// a new sandbox, so only the app's directory under it reaches the running
// app.
func sharedRuntimeDir(getenv func(string) string) string {
	if !config.InFlatpak(getenv) {
		return ""
	}
	return filepath.Join(getenv("XDG_RUNTIME_DIR"), "app", config.AppID)
}

// lockSingleInstance takes MyGo's lock, which puts its socket in
// $XDG_RUNTIME_DIR; the variable changes for that one call.
func lockSingleInstance() bool {
	if dir := sharedRuntimeDir(os.Getenv); dir != "" {
		was := os.Getenv("XDG_RUNTIME_DIR")
		if os.Setenv("XDG_RUNTIME_DIR", dir) == nil {
			defer func() { _ = os.Setenv("XDG_RUNTIME_DIR", was) }()
		}
	}
	return mygo.App.RequestSingleInstanceLock()
}
