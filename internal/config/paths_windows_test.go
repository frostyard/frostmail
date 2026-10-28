package config

import (
	"strings"
	"testing"
)

func TestResolveDefaults(t *testing.T) {
	p, err := Resolve(env(map[string]string{"LOCALAPPDATA": `C:\Users\u\AppData\Local`}))
	if err != nil {
		t.Fatal(err)
	}
	sid, err := UserSID()
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		DataDir: `C:\Users\u\AppData\Local\Frostmail\Data`, CacheDir: `C:\Users\u\AppData\Local\Frostmail\Cache`,
		Socket: `\\.\pipe\frostmail-` + sid,
		DB:     `C:\Users\u\AppData\Local\Frostmail\Data\frostmail.db`, Blobs: `C:\Users\u\AppData\Local\Frostmail\Data\blobs`,
	}
	if p != want {
		t.Fatalf("Resolve = %+v\nwant %+v", p, want)
	}
	if !strings.HasPrefix(sid, "S-1-") {
		t.Errorf("SID = %q", sid)
	}
}

func TestResolveOverrides(t *testing.T) {
	p, err := Resolve(env(map[string]string{"LOCALAPPDATA": `C:\L`, "FROSTMAIL_DATA_DIR": `D:\d`, "FROSTMAIL_SOCKET": `\\.\pipe\x`}))
	if err != nil {
		t.Fatal(err)
	}
	if p.DataDir != `D:\d` || p.DB != `D:\d\frostmail.db` || p.CacheDir != `C:\L\Frostmail\Cache` || p.Socket != `\\.\pipe\x` {
		t.Fatalf("Resolve = %+v", p)
	}
}

func TestResolveNeedsLocalAppData(t *testing.T) {
	if _, err := Resolve(env(map[string]string{})); err == nil {
		t.Fatal("want an error without LOCALAPPDATA or the overrides")
	}
}

func TestAppConfigFile(t *testing.T) {
	got, err := AppConfigFile(env(map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}))
	if err != nil || got != `C:\Users\u\AppData\Roaming\Frostmail\app.json` {
		t.Fatalf("AppConfigFile = %q, %v", got, err)
	}
	if _, err := AppConfigFile(env(nil)); err == nil {
		t.Fatal("want an error without APPDATA")
	}
}
