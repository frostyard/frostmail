package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFlatpakBuildReadsAppCommitOfInstance(t *testing.T) {
	const info = `[Application]
name=org.frostyard.Frostmail
app-commit=not-this-one

[Instance]
instance-id=123
app-path=/x
app-commit = 0a1b2c3d
runtime-commit=ffff

[Other]
app-commit=nor-this
`
	path := filepath.Join(t.TempDir(), "info")
	writeFile(t, path, info)
	got, err := flatpakBuild(func(k string) string {
		if k == "FROSTMAIL_FLATPAK_INFO" {
			return path
		}
		return ""
	})
	if err != nil || got != "0a1b2c3d" {
		t.Fatalf("flatpakBuild = %q, %v; want 0a1b2c3d", got, err)
	}
}

func TestFlatpakBuildErrors(t *testing.T) {
	dir := t.TempDir()
	noKey := filepath.Join(dir, "nokey")
	writeFile(t, noKey, "[Instance]\ninstance-id=1\n[Application]\napp-commit=x\n")
	empty := filepath.Join(dir, "empty")
	writeFile(t, empty, "[Instance]\napp-commit=\n")
	for name, path := range map[string]string{"missing file": filepath.Join(dir, "none"), "no key": noKey, "empty value": empty} {
		t.Run(name, func(t *testing.T) {
			got, err := flatpakBuild(func(string) string { return path })
			if err == nil || got != "" {
				t.Fatalf("flatpakBuild = %q, %v; want an error", got, err)
			}
		})
	}
}

func TestPrintBuildWritesNothingOnError(t *testing.T) {
	t.Setenv("FROSTMAIL_FLATPAK_INFO", filepath.Join(t.TempDir(), "none"))
	var out bytes.Buffer
	if err := printBuild(&out); err == nil || out.Len() != 0 {
		t.Fatalf("printBuild = %v, %q; want an error and no output", err, out.String())
	}
	path := filepath.Join(t.TempDir(), "info")
	writeFile(t, path, "[Instance]\napp-commit=abc\n")
	t.Setenv("FROSTMAIL_FLATPAK_INFO", path)
	if err := printBuild(&out); err != nil || out.String() != "abc\n" {
		t.Fatalf("printBuild = %v, %q; want abc", err, out.String())
	}
}

// -mode answers local or lodge, with or without FLATPAK_ID, and fails on a
// malformed app.json naming the file.
func TestPrintMode(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("APPDATA", cfg) // Windows' place for it
	app := filepath.Join(cfg, "frostmail", "app.json")
	if err := os.MkdirAll(filepath.Dir(app), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		app = filepath.Join(cfg, "Frostmail", "app.json")
		if err := os.MkdirAll(filepath.Dir(app), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, flatpakID := range []string{"", "org.frostyard.Frostmail"} {
		t.Setenv("FLATPAK_ID", flatpakID)
		for _, tc := range []struct{ file, want string }{
			{`{}`, "local\n"},
			{`{"lodge":"https://l.example"}`, "lodge\n"},
		} {
			writeFile(t, app, tc.file)
			var out bytes.Buffer
			if err := printMode(&out); err != nil || out.String() != tc.want {
				t.Fatalf("printMode(%s, FLATPAK_ID=%q) = %q, %v; want %q", tc.file, flatpakID, out.String(), err, tc.want)
			}
		}
		writeFile(t, app, `{"lodge":`)
		var out bytes.Buffer
		if err := printMode(&out); err == nil || !strings.Contains(err.Error(), "app.json") || out.Len() != 0 {
			t.Fatalf("printMode(bad file) = %q, %v; want an error that names app.json", out.String(), err)
		}
	}
}
