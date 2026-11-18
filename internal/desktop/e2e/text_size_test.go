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
		{"classic scrollbar", `<style>::-webkit-scrollbar{width:17px}</style>` + text},
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
	// The page must return to its Default height, not only shrink from
	// Largest. A classic inner scrollbar must not narrow the first page
	// while its viewport still has the reader's initial 40-pixel height.
	// The native reader scrolls the page; its viewport has no vertical
	// scrollbar, so the first load and a reload wrap at the same width.
	near := func(got, want, tolerance float64) bool { return math.Abs(got-want) <= tolerance }

	// Wait for a measured page height to reach the native tree. Two reads
	// do not establish that the frame's slower height poll has settled;
	// the page must wrap at its full width even before that poll finishes.
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
	row2, page2 := waitHeights(fmt.Sprintf("the Default size again: a row within 0.5 of %v and a page within 0.5 of %v", row0, page0), func(row, page float64) bool {
		return near(row, row0, 0.5) && near(page, page0, 0.5)
	})
	t.Logf("Default again: row %v, page %v", row2, page2)
}
