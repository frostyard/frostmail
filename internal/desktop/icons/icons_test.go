package icons_test

import (
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
