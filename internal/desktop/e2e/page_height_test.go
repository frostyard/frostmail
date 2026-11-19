//go:build e2e

package e2e

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A message that sizes its root to the viewport and clips a box of that
// height, as some notices do around a header that is wider than their
// column, shows all of its content: the reader's page is as tall as the
// same message without the root's height. Without the page's height rule,
// the page measures as its viewport, the reader's first 40 pixels, and
// shows only the top of the header. Neither an important rule with a higher
// specificity nor one in the message's own layer undoes the page's rule.
func TestViewportRootMessageShowsAllOfItsContent(t *testing.T) {
	const notice = `<div style="display:none;max-height:0;overflow:hidden">Preheader text</div>` +
		`<div class="wrap" style="height:100%;overflow:hidden">` +
		`<table width="100%" cellpadding="0" cellspacing="0" bgcolor="#368727"><tr><td style="height:40px">` +
		`<div style="width:900px;height:24px;margin:16px auto 0;background:#fff;border-radius:24px 24px 0 0"></div></td></tr></table>` +
		`<table width="600" align="center" cellpadding="0" cellspacing="0" bgcolor="#ffffff"><tr><td style="padding:24px 32px;font:16px/24px Arial,sans-serif">` +
		`<p>A direct debit of 10.00 USD left the account that ends in 0000.</p>` +
		`<p>When you did not expect this payment, call the number on the back of your card.</p>` +
		`<p>This notice is automatic. Replies to it are not read.</p></td></tr></table></div>`
	// Each page has its app in a subtest of its own, which stops it before
	// the next starts: on Windows, a second app of the same user does not
	// start while one runs.
	var control float64
	if !t.Run("control", func(t *testing.T) {
		control = settledPageHeight(t, `<style>body{margin:0!important;padding:0!important}</style>`+notice)
		if control <= 100 {
			t.Fatalf("the control page is %v high: its content did not lay out", control)
		}
	}) {
		return
	}
	for _, tc := range []struct{ name, style string }{
		{"percent root", `html,body{height:100%!important;margin:0!important;padding:0!important}`},
		{"specific important", `html{height:100%!important}html body{height:100%!important;margin:0!important;padding:0!important}`},
		{"own layer", `@layer root{html,body{height:100%!important}}body{margin:0!important;padding:0!important}`},
		{"viewport units", `html{height:100vh!important}body{height:100%!important;margin:0!important;padding:0!important}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := settledPageHeight(t, `<style>`+tc.style+`</style>`+notice); math.Abs(got-control) > 0.5 {
				t.Errorf("the page is %v high, want %v, the height of the same message without its root's height", got, control)
			}
		})
	}
}

// settledPageHeight opens page as the only message, and returns the reader
// page's height once it has stayed the same for longer than the frame's
// polls take to agree (400 milliseconds × 3).
func settledPageHeight(t *testing.T, page string) float64 {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "01-notice.html"), []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	s := start(t, options{fixture: []string{"-n", "0", "-hostile", dir}})
	openHostile(s)
	var height, last float64
	var unchangedSince time.Time
	defer func() {
		if t.Failed() {
			t.Logf("the last page height seen was %v", last)
		}
	}()
	eventually(t, "the reader page's settled height", func() bool {
		height = 0
		if n := find(s.tree(1), "", "Message body"); n != nil {
			height = n.Bounds.Height
		}
		if height != last || height <= 0 {
			last, unchangedSince = height, time.Now()
			return false
		}
		return time.Since(unchangedSince) >= 1300*time.Millisecond
	})
	return height
}
