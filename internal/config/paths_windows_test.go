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
		DataDir: `C:\Users\u\AppData\Local\Frostmail`, CacheDir: `C:\Users\u\AppData\Local\Frostmail\Cache`,
		Socket: `\\.\pipe\frostmail-` + sid,
		DB:     `C:\Users\u\AppData\Local\Frostmail\frostmail.db`, Blobs: `C:\Users\u\AppData\Local\Frostmail\blobs`,
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
