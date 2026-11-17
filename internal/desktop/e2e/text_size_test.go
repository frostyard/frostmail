//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// P-957: the text size the user chose sets every row of the list and the
// text of the real reader page, at once, and the Default size is the size it
// was before. The page's height is its text's: a long paragraph in the real
// page, zoomed, whose measure reports the zoomed height.
func TestTextSizeScalesTheListAndThePage(t *testing.T) {
	dir := t.TempDir()
	page := `<p>` + strings.Repeat("a long paragraph of the page whose height is its text ", 80) + `</p>`
	if err := os.WriteFile(filepath.Join(dir, "01-long.html"), []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	s := start(t, options{fixture: []string{"-n", "0", "-hostile", dir}, server: &rpctest.Options{}})
	openHostile(s)
	heights := func() (row, page float64) {
		tree := s.tree(1)
		if n := find(tree, "listItem", "Hostile"); n != nil {
			row = n.Bounds.Height
		}
		if n := find(tree, "", "Message body"); n != nil {
			page = n.Bounds.Height
		}
		return row, page
	}
	set := func(size api.TextSize) {
		t.Helper()
		if _, err := s.client.Settings().Set(t.Context(), &api.SettingsSetParams{TextSize: &size}); err != nil {
			t.Fatal(err)
		}
	}
	// The page lays out after the window shows it: wait for its height to
	// settle, which is the text's.
	var row0, page0, last float64
	eventually(t, "the page's height at the Default size", func() bool {
		row0, page0 = heights()
		same := page0 > 100 && page0 == last
		last = page0
		return row0 > 0 && same
	})
	t.Logf("Default: row %v, page %v", row0, page0)

	set(api.TextSizeLargest)
	var row1, page1 float64
	eventually(t, fmt.Sprintf("the Largest size: a row 1.2 times %v and a page 1.15 times %v", row0, page0), func() bool {
		row1, page1 = heights()
		return row1 >= 1.2*row0 && page1 >= 1.15*page0
	})
	t.Logf("Largest: row %v, page %v", row1, page1)

	set(api.TextSizeDefault)
	eventually(t, fmt.Sprintf("the Default size again: row %v, page %v", row0, page0), func() bool {
		row, page := heights()
		return row == row0 && page == page0
	})
}
