//go:build e2e

package e2e

import (
	"fmt"
	"math"
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
// page, zoomed, whose measure reports the zoomed height. A message's own
// style cannot undo the zoom: the second page asks for zoom 1 with
// !important on the root.
func TestTextSizeScalesTheListAndThePage(t *testing.T) {
	text := `<p>` + strings.Repeat("a long paragraph of the page whose height is its text ", 80) + `</p>`
	for _, tc := range []struct{ name, page string }{
		{"plain", text},
		{"own zoom", `<style>:root{zoom:1 !important}html{zoom:1 !important}</style>` + text},
	} {
		t.Run(tc.name, func(t *testing.T) { textSizeScales(t, tc.page) })
	}
}

func textSizeScales(t *testing.T, page string) {
	dir := t.TempDir()
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
	// waitHeights polls until ok accepts the list row's and the page's
	// heights, and on a timeout says the last pair it saw.
	waitHeights := func(what string, ok func(row, page float64) bool) (row, page float64) {
		t.Helper()
		defer func() {
			if t.Failed() {
				t.Logf("waiting for %s: the last heights seen were row %v, page %v", what, row, page)
			}
		}()
		eventually(t, what, func() bool {
			row, page = heights()
			return ok(row, page)
		})
		return row, page
	}
	// A page loaded again need not come back to its first height: on
	// WebView2 the first page measured 1875 and the same page, loaded again
	// at Default after Largest, 1692 (CI, frostyard/frostmail#342), as the
	// first measure can come before the window's final layout. So the
	// return to Default holds the row to its height and the page to a
	// shrink from its Largest height by at least the factor Largest added
	// to it, which a page still zoomed cannot meet.
	near := func(got, want, tolerance float64) bool { return math.Abs(got-want) <= tolerance }

	// The page lays out after the window shows it: wait for its height to
	// settle, which is the text's.
	var last float64
	row0, page0 := waitHeights("the page's height at the Default size", func(row, page float64) bool {
		same := page > 100 && page == last
		last = page
		return row > 0 && same
	})
	t.Logf("Default: row %v, page %v", row0, page0)

	set(api.TextSizeLargest)
	row1, page1 := waitHeights(fmt.Sprintf("the Largest size: a row 1.2 times %v and a page 1.15 times %v", row0, page0), func(row, page float64) bool {
		return row >= 1.2*row0 && page >= 1.15*page0
	})
	t.Logf("Largest: row %v, page %v", row1, page1)

	set(api.TextSizeDefault)
	row2, page2 := waitHeights(fmt.Sprintf("the Default size again: a row within 0.5 of %v and a page at most %v / 1.15", row0, page1), func(row, page float64) bool {
		return near(row, row0, 0.5) && page > 100 && page*1.15 <= page1
	})
	t.Logf("Default again: row %v, page %v", row2, page2)
}
