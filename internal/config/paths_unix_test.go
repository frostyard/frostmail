//go:build !windows

package config

import "testing"

func TestResolveDefaults(t *testing.T) {
	p, err := Resolve(env(map[string]string{"HOME": "/home/u", "XDG_RUNTIME_DIR": "/run/user/1000"}))
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		DataDir: "/home/u/.local/share/frostmail", CacheDir: "/home/u/.cache/frostmail",
		Socket: "/run/user/1000/frostmail/maild.sock",
		DB:     "/home/u/.local/share/frostmail/frostmail.db", Blobs: "/home/u/.local/share/frostmail/blobs",
	}
	if p != want {
		t.Fatalf("Resolve = %+v\nwant %+v", p, want)
	}
}

func TestResolveOverridesAndRelativeXDG(t *testing.T) {
	p, err := Resolve(env(map[string]string{
		"HOME": "/home/u", "XDG_DATA_HOME": "relative/ignored", "XDG_CACHE_HOME": "/c",
		"FROSTMAIL_DATA_DIR": "/d", "FROSTMAIL_SOCKET": "/s/maild.sock",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if p.DataDir != "/d" || p.DB != "/d/frostmail.db" || p.CacheDir != "/c/frostmail" || p.Socket != "/s/maild.sock" {
		t.Fatalf("Resolve = %+v", p)
	}
}

func TestResolveNeedsRuntimeDir(t *testing.T) {
	if _, err := Resolve(env(map[string]string{"HOME": "/home/u"})); err == nil {
		t.Fatal("want an error without XDG_RUNTIME_DIR or FROSTMAIL_SOCKET")
	}
}

func TestFlatpakSocketIsInTheAppsSharedRuntimeDir(t *testing.T) {
	run := map[string]string{"HOME": "/home/u", "XDG_RUNTIME_DIR": "/run/user/1000"}
	for _, tc := range []struct{ name, id, want string }{
		{"this app's Flatpak", AppID, "/run/user/1000/app/org.frostyard.Frostmail/frostmail/maild.sock"},
		{"another app's Flatpak", "org.example.Editor", "/run/user/1000/frostmail/maild.sock"},
		{"no Flatpak", "", "/run/user/1000/frostmail/maild.sock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := map[string]string{"FLATPAK_ID": tc.id}
			for k, v := range run {
				e[k] = v
			}
			p, err := Resolve(env(e))
			if err != nil || p.Socket != tc.want {
				t.Fatalf("Resolve socket = %q, %v; want %q", p.Socket, err, tc.want)
			}
		})
	}
	// FROSTMAIL_SOCKET still wins in the Flatpak.
	p, err := Resolve(env(map[string]string{"HOME": "/home/u", "FLATPAK_ID": AppID, "XDG_RUNTIME_DIR": "/run/user/1000", "FROSTMAIL_SOCKET": "/s/m.sock"}))
	if err != nil || p.Socket != "/s/m.sock" {
		t.Fatalf("Resolve socket = %q, %v; want FROSTMAIL_SOCKET", p.Socket, err)
	}
}

func TestAppConfigFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"XDG_CONFIG_HOME", map[string]string{"HOME": "/home/u", "XDG_CONFIG_HOME": "/cfg"}, "/cfg/frostmail/app.json"},
		{"HOME", map[string]string{"HOME": "/home/u"}, "/home/u/.config/frostmail/app.json"},
		{"a relative XDG_CONFIG_HOME", map[string]string{"HOME": "/home/u", "XDG_CONFIG_HOME": "rel"}, "/home/u/.config/frostmail/app.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AppConfigFile(env(tc.env))
			if err != nil || got != tc.want {
				t.Fatalf("AppConfigFile = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	if _, err := AppConfigFile(env(nil)); err == nil {
		t.Fatal("want an error without HOME")
	}
}
