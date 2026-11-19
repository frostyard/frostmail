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
// page's height once it has settled (heightSettle).
func settledPageHeight(t *testing.T, page string) float64 {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "01-notice.html"), []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	s := start(t, options{fixture: []string{"-n", "0", "-hostile", dir}})
	openHostile(s)
	var height float64
	var settle heightSettle
	defer func() {
		if t.Failed() {
			t.Logf("the last page height seen was %v", settle.last)
		}
	}()
	eventually(t, "the reader page's settled height", func() bool {
		height = 0
		if n := find(s.tree(1), "", "Message body"); n != nil {
			height = n.Bounds.Height
		}
		return settle.settled(height, time.Now())
	})
	return height
}

// readerPlaceholder is the height the reader gives a page that has not
// measured yet (internal/desktop/mainwin/reader.go, max(Height(), 40)).
const readerPlaceholder = 40

// settleQuiet is longer than the frame's polls take to agree
// (measureInterval × measureAgree, 400 milliseconds × 3).
const settleQuiet = 1300 * time.Millisecond

// heightSettle follows the reader page's height. The page shows at the
// reader's placeholder height until its document has loaded and measured,
// which on a slow runner can take longer than settleQuiet: a height at or
// under the placeholder never settles, so a page that measures as its
// 40-pixel viewport fails as a timeout, not as a height.
type heightSettle struct {
	last  float64
	since time.Time
}

// settled takes the height read at now, and reports whether a height above
// the reader's placeholder has stayed the same for settleQuiet.
func (h *heightSettle) settled(height float64, now time.Time) bool {
	if height != h.last || height <= readerPlaceholder {
		h.last, h.since = height, now
		return false
	}
	return now.Sub(h.since) >= settleQuiet
}

// The reader's placeholder height never settles, however long it stays:
// the page has not measured yet. A measured height settles after
// settleQuiet, and a change starts the wait again.
func TestHeightSettleWaitsForTheMeasure(t *testing.T) {
	t0 := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	var h heightSettle
	for _, ms := range []int{0, 1300, 5000, 30000} {
		if h.settled(readerPlaceholder, t0.Add(time.Duration(ms)*time.Millisecond)) {
			t.Fatalf("the placeholder height settled at %d ms", ms)
		}
	}
	if h.settled(0, t0.Add(31*time.Second)) {
		t.Fatal("no height settled")
	}
	base := t0.Add(32 * time.Second)
	for _, step := range []struct {
		height float64
		after  time.Duration
		want   bool
	}{
		{300, 0, false},
		{300, settleQuiet - time.Millisecond, false},
		{300, settleQuiet, true},
		{320, settleQuiet + time.Millisecond, false},
		{320, 2 * settleQuiet, false},
		{320, 2*settleQuiet + time.Millisecond, true},
		{41, 3 * settleQuiet, false},
		{41, 4 * settleQuiet, true},
	} {
		if got := h.settled(step.height, base.Add(step.after)); got != step.want {
			t.Errorf("height %v at +%v: settled %v, want %v", step.height, step.after, got, step.want)
		}
	}
}
