package main

import (
	"path/filepath"
	"testing"

	"github.com/frostyard/frostmail/internal/config"
)

func TestSharedRuntimeDirIsTheAppsInTheFlatpak(t *testing.T) {
	env := map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000", "FLATPAK_ID": config.AppID}
	want := filepath.Join("/run/user/1000", "app", config.AppID)
	if got := sharedRuntimeDir(func(k string) string { return env[k] }); got != want {
		t.Errorf("in the Flatpak: %q, want %q", got, want)
	}
	delete(env, "FLATPAK_ID")
	if got := sharedRuntimeDir(func(k string) string { return env[k] }); got != "" {
		t.Errorf("outside it: %q, want none", got)
	}
}
