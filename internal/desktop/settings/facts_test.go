package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/settings"
)

func TestFactsFrom(t *testing.T) {
	home, data := t.TempDir(), t.TempDir()
	touch := func(path string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{"HOME": home, "FROSTMAIL_CACHE_DIR": data, "FROSTMAIL_DATA_DIR": data}
	get := func(key string) string { return env[key] }
	if f := settings.FactsFrom(get); f != (settings.ServerFacts{}) {
		t.Errorf("no facts = %+v", f)
	}
	touch(filepath.Join(data, "frostmail.db"))
	if f := settings.FactsFrom(get); !f.LocalData || f.Flatpak {
		t.Errorf("data = %+v", f)
	}
	touch(filepath.Join(home, ".local", "share", "frostmail", "frostmail.db"))
	if f := settings.FactsFrom(get); f.HostMail {
		t.Errorf("outside the Flatpak the app does not ask: %+v", f)
	}
	env["FLATPAK_ID"] = "org.frostyard.Frostmail"
	if f := settings.FactsFrom(get); !f.Flatpak || !f.HostMail {
		t.Errorf("in the Flatpak = %+v", f)
	}
	env["FROSTMAIL_SOCKET"] = "/dev.sock"
	if f := settings.FactsFrom(get); f.HostMail {
		t.Errorf("the development bundle's maild is the host's: %+v", f)
	}
	delete(env, "FROSTMAIL_SOCKET")
	env["FLATPAK_ID"] = "org.other.App"
	if f := settings.FactsFrom(get); f.Flatpak || f.HostMail {
		t.Errorf("another app's Flatpak = %+v", f)
	}
}
