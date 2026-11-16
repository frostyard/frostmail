package icons_test

import (
	"bytes"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/internal/desktop/icons"
)

// iconName finds the names the app draws: frostui's Icon and IconButton,
// a row's Icon field, and a literal SummaryIcon.
var iconName = regexp.MustCompile(`(?:frostui\.Icon\(c[^,]*, |frostui\.IconButton\(c[^,]*, |frostui\.LabeledButton\(c[^,]*, |frostui\.Empty\(c[^,]*, |Icon: |Name: )"([a-z][a-z0-9-]*)"`)

// The tray icon is the app's own icon, so the panel shows the app: a PNG,
// square, large enough for a tray (frostui.TrayOptions.Icon asks 32 pixels
// or more), and the same bytes as the icon the app ships.
func TestTrayIconIsTheAppsIcon(t *testing.T) {
	icon := icons.Tray()
	img, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		t.Fatalf("the tray icon is not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != b.Dy() || b.Dx() < 32 {
		t.Errorf("the tray icon is %d by %d, want a square of 32 pixels or more", b.Dx(), b.Dy())
	}
	shipped, err := os.ReadFile("../../../packaging/icons/128x128.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(icon, shipped) {
		t.Error("the tray icon differs from packaging/icons/128x128.png; copy it again")
	}
}

// Off Linux an icon comes only from the app's set or frostui's: with no
// icon theme to search, every name the app draws must still resolve.
func TestEveryIconTheAppDrawsIsEmbedded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	set := frostui.NewIcons(icons.Embedded())
	seen := 0
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" || filepath.Base(path) == "icons_test.go" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range iconName.FindAllSubmatch(data, -1) {
			seen++
			if set.Get(string(m[1])) == nil {
				t.Errorf("%s draws %q, which no embedded set has", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 10 {
		t.Fatalf("found only %d icons; the pattern has gone stale", seen)
	}
}
