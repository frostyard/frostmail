package uikit_test

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// blankFrame stands in for a page the native view has not painted: it
// draws nothing of its own.
type blankFrame struct{}

func (blankFrame) Load(string)                        {}
func (blankFrame) View(c *ui.Context) ui.Element      { return ui.Column(c).Focusable() }
func (blankFrame) Height() float32                    { return 0 }
func (blankFrame) Selection() string                  { return "" }
func (blankFrame) Find(string, bool, bool) (int, int) { return 0, 0 }
func (blankFrame) StopFind()                          {}
func (blankFrame) Print()                             {}
func (blankFrame) Close()                             {}

// The page keeps its white ground in both themes and sits on the panel
// inside a hairline: where the view has not painted, the window shows the
// page's white, never its own dark ground (frostyard/frostmail#269).
func TestPageFrameSetsTheWhitePageOnThePanel(t *testing.T) {
	const width = 400
	var page *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) {
		page = frostui.Adapt(c)
		ui.Column(c).Fill().AlignItems(ui.Stretch).Background(page.Background).Children(func() {
			uikit.PageFrame(c, blankFrame{}, 120).Label("Message body")
		})
	}, width, 200)
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Frame()
		body, ok := tt.Access("Message body")
		if !ok {
			t.Fatalf("no page in %q", strings.Join(tt.Texts(), ", "))
		}
		b := body.Bounds
		if b.H != 120 || b.X < frostui.SpaceXL || b.X+b.W > width-frostui.SpaceXL {
			t.Errorf("dark=%v: the page is at %+v, without room around it", dark, b)
		}
		if p := pixel(tt, width, b.X+b.W/2, b.Y+b.H/2); !near(p, uikit.PageColor()) {
			t.Errorf("dark=%v: an unpainted page shows %v, not the page's white", dark, p)
		}
		if p := pixel(tt, width, b.X-frostui.SpaceM/2, b.Y+b.H/2); !near(p, frostui.Panel(page)) {
			t.Errorf("dark=%v: the page's surround is %v, not the panel %v", dark, p, frostui.Panel(page))
		}
	}
}
