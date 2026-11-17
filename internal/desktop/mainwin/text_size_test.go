package mainwin_test

import (
	"math"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

func (f *fixture) setTextSize(t *testing.T, size api.TextSize) {
	t.Helper()
	if _, err := f.srv.Dial(t).Settings().Set(t.Context(), &api.SettingsSetParams{TextSize: &size}); err != nil {
		t.Fatal(err)
	}
}

// sized draws the window as the app does: through uikit.Adapt, at the text
// size the window holds (cmd/frostmail's windowSet.sized).
func (f *fixture) sized() {
	f.tt = ui.NewTester(func(c *ui.Context) {
		f.loop.Flush()
		uikit.Adapt(c, mail.TextScale(f.win.MailState().Settings))
		f.win.View(c)
	}, 1200, 780)
}

// P-957: the list's rows and text follow the text size, at the usual size
// they are what they always were, and the window says once that the size
// changed, for the app's other windows.
func TestTextSizeScalesTheListAndTellsTheApp(t *testing.T) {
	var changed atomic.Int32
	f := newFixtureWith(t, 3, func(o *mainwin.Options) { o.TextSizeChanged = func() { changed.Add(1) } })
	f.sized()
	f.until(t, "the rows", func() bool { return f.tt.HasText("Message 3") && f.tt.HasText("Message 2") })
	measure := func() (gap, text float32) {
		a, ok1 := f.tt.Find("Message 3")
		b, ok2 := f.tt.Find("Message 2")
		if !ok1 || !ok2 {
			return 0, 0
		}
		return max(a.Y-b.Y, b.Y-a.Y), a.H
	}
	usualGap, usualText := measure()
	if usualGap != uikit.SummaryHeightFor(2) || usualText <= 0 {
		t.Fatalf("at the usual size the rows are %v apart (want %v) and the text %v high", usualGap, uikit.SummaryHeightFor(2), usualText)
	}
	near := func(got, want float32) bool { return got-want < 0.5 && want-got < 0.5 }
	for _, c := range []struct {
		size  api.TextSize
		scale float32
	}{{api.TextSizeLargest, 1.3}, {api.TextSizeSmaller, 0.9}, {api.TextSizeLarger, 1.15}} {
		f.setTextSize(t, c.size)
		f.until(t, string(c.size)+" rows", func() bool {
			gap, text := measure()
			// A line of text is as high as its font makes it, in whole pixels.
			return near(gap, usualGap*c.scale) && math.Abs(float64(text-usualText*c.scale)) <= 1.5 && (text > usualText) == (c.scale > 1)
		})
	}
	f.setTextSize(t, api.TextSizeDefault)
	f.until(t, "the usual rows again", func() bool {
		gap, text := measure()
		return gap == usualGap && text == usualText
	})
	// Frames after the last change ask nothing more of the app: 4 changes, 4 calls.
	for range 20 {
		f.tt.Frame()
	}
	if n := changed.Load(); n != 4 {
		t.Errorf("the window said the text size changed %d times, want 4", n)
	}
}

// The page of a message zooms with the text size, and not at the usual
// size; a change loads the open page again, once.
func TestReaderPageFollowsTheTextSize(t *testing.T) {
	f := oneMail(t, "Content-Type: text/html\n", "<p>Hello from the page</p>")
	f.sized()
	f.open(t, "Alpha report")
	var frame *fakeFrame
	f.until(t, "the page", func() bool {
		if p := f.live(); len(p) == 1 {
			frame = p[0]
		}
		return frame != nil && strings.Contains(frame.page(), "Hello from the page")
	})
	const zoom = "@media screen{:root{zoom:"
	if strings.Contains(frame.page(), zoom) {
		t.Fatalf("the page zooms at the usual size: %s", frame.page())
	}
	usual := frame.page()
	for _, c := range []struct {
		size api.TextSize
		want string
	}{{api.TextSizeLargest, zoom + "1.3}}"}, {api.TextSizeSmaller, zoom + "0.9}}"}} {
		before := frame.count()
		f.setTextSize(t, c.size)
		f.until(t, string(c.size)+" page", func() bool { return strings.Contains(frame.page(), c.want) })
		for range 20 {
			f.tt.Frame()
		}
		if got := frame.count() - before; got != 1 {
			t.Errorf("%s: the page loaded %d times, want 1", c.size, got)
		}
		// The zoom comes after the print guard, so the guard's own rules
		// (which set zoom 1 on paper) stay first in the page.
		if strings.Index(frame.page(), mail.PrintGuard) > strings.Index(frame.page(), c.want) {
			t.Errorf("%s: the zoom comes before the print guard", c.size)
		}
	}
	f.setTextSize(t, api.TextSizeDefault)
	f.until(t, "the usual page", func() bool { return frame.page() == usual })
}

func (f *fakeFrame) page() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.doc
}

func (f *fakeFrame) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loads
}
