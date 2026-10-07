package config

import "testing"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

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
