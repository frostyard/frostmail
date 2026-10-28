package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadMode(t *testing.T) {
	for _, tc := range []struct {
		name, file string // file "" means no file
		want       Mode
		wantErr    bool
	}{
		{"no file", "", ModeLocal, false},
		{"empty object", `{}`, ModeLocal, false},
		{"no lodge key", `{"other": 1}`, ModeLocal, false},
		{"empty lodge", `{"lodge": ""}`, ModeLocal, false},
		{"null lodge", `{"lodge": null}`, ModeLocal, false},
		{"lodge", `{"lodge": "https://lodge.example.ts.net"}`, ModeLodge, false},
		{"lodge and unknown keys", `{"x": [1], "lodge": "http://127.0.0.1:8080", "y": {}}`, ModeLodge, false},
		{"not JSON", `lodge`, "", true},
		{"empty file", ``, "", true},
		{"lodge is not a string", `{"lodge": 7}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "app.json")
			if tc.file != "" || tc.name == "empty file" {
				if err := os.WriteFile(path, []byte(tc.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ReadMode(path)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("ReadMode = %q, %v; want %q (error %v)", got, err, tc.want, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), path) {
				t.Fatalf("error %q does not name the file", err)
			}
		})
	}
}

func TestReadModeNeverWrites(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadMode(filepath.Join(dir, "app.json")); err != nil {
		t.Fatal(err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("ReadMode made %v", ents)
	}
}
