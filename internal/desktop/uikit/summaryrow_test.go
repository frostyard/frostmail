package uikit_test

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

const testIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path d="M4 4h16v16H4z"/></svg>`

func setIcons() {
	frostui.SetIcons(frostui.NewIcons(map[string][]byte{"mail-mark-important": []byte(testIcon), "mail-attachment": []byte(testIcon)}))
}

// row draws a summary row with every field, whose action counts its clicks.
func row(clicks *int) *ui.Tester {
	photo := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 8, 8)))
	return ui.NewTester(func(c *ui.Context) {
		ui.Column(c).AlignItems(ui.Stretch).Children(func() {
			uikit.SummaryRow(c, uikit.Summary{Title: "Ann Lee", Date: "9:41", Subtitle: "Plans", Preview: "See you there",
				Unread: true, UnreadLabel: "Not read yet", Mark: ui.Color{R: 1, A: 1}, MarkLabel: "Flagged: Red",
				Icons: []uikit.SummaryIcon{{Name: "mail-attachment", Label: "Has attachments"}},
				Count: "3", Photo: photo,
				Actions: func() {
					if ui.Button(c, "Archive row").Clicked() {
						*clicks++
					}
				}})
		})
	}, 600, 200)
}

func TestSummaryRowNamesItsFields(t *testing.T) {
	setIcons()
	clicks := 0
	tt := row(&clicks)
	for _, name := range []string{"Not read yet", "Flagged: Red", "Has attachments", "9:41", "Plans", "See you there", "3"} {
		if _, ok := tt.Access(name); !ok {
			t.Errorf("assistive technology does not find %q in %q", name, tt.Texts())
		}
	}
	if _, ok := tt.AccessByRole(ui.RoleImage, "Ann Lee"); !ok {
		t.Error("the photo is not an image named for the person")
	}
	if _, ok := tt.Access("Unread"); ok {
		t.Error("the dot has a name of its own, not the one given")
	}
}

func TestSummaryRowDrawsOnlyWhatItIsGiven(t *testing.T) {
	setIcons()
	tt := ui.NewTester(func(c *ui.Context) {
		uikit.SummaryRow(c, uikit.Summary{Title: "Ann Lee", UnreadLabel: "Not read yet", MarkLabel: ""})
	}, 600, 200)
	if _, ok := tt.Access("Not read yet"); ok {
		t.Error("a read row has the dot")
	}
	if _, ok := tt.AccessByRole(ui.RoleImage, "Ann Lee"); ok {
		t.Error("a row with no photo has one")
	}
}

// The row's buttons show on hover, work, and repeat its commands, so
// assistive technology does not see them.
func TestSummaryRowActionsShowOnHoverOnly(t *testing.T) {
	setIcons()
	clicks := 0
	tt := row(&clicks)
	if tt.HasText("Archive row") {
		t.Fatal("the actions show with the pointer away")
	}
	title, ok := tt.Find("Plans")
	if !ok {
		t.Fatalf("no subtitle in %q", tt.Texts())
	}
	tt.Move(title.X+title.W/2, title.Y+title.H/2)
	tt.Frame()
	if !tt.HasText("Archive row") {
		t.Fatalf("no actions on hover in %q", tt.Texts())
	}
	if info, ok := tt.Access("Archive row"); ok {
		t.Errorf("assistive technology sees the action: %+v", info)
	}
	if err := tt.Click("Archive row"); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Errorf("the action ran %d times", clicks)
	}
}

// pixel is the color the last frame drew at (x, y), in DIPs.
func pixel(tt *ui.Tester, width int, x, y float32) color.RGBA {
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / float32(width)
	return color.RGBAModel.Convert(img.At(int(x*scale), int(y*scale))).(color.RGBA)
}

// near reports whether a drawn pixel is c, give or take rounding.
func near(p color.RGBA, c ui.Color) bool {
	d := func(a, b uint8) bool { return max(a, b)-min(a, b) <= 2 }
	return d(p.R, c.R) && d(p.G, c.G) && d(p.B, c.B)
}

// frostyard/frostmail#267: a preview of one token far wider than the list
// wraps or ends inside the row, and nothing paints past the row.
func TestSummaryRowKeepsALongTokenInside(t *testing.T) {
	setIcons()
	const width, listWidth = 600, 220
	token := strings.Repeat("spacer", 50)
	var bg ui.Color
	tt := ui.NewTester(func(c *ui.Context) {
		frostui.Adapt(c)
		bg = c.Theme().Background
		ui.Row(c).Fill().AlignItems(ui.Start).Background(bg).Children(func() {
			ui.Column(c).Width(listWidth).Shrink(0).AlignItems(ui.Stretch).Children(func() {
				uikit.SummaryRow(c, uikit.Summary{Title: "Spacer Weekly " + token, Date: "9:41", Subtitle: token, Preview: token, Count: "12"})
			})
		})
	}, width, 120)
	tt.Frame()
	if r, ok := tt.Find("12"); !ok || r.X+r.W > listWidth {
		t.Errorf("the count is at %+v (%v), outside the %d-wide list", r, ok, listWidth)
	}
	for y := float32(0); y < uikit.SummaryHeight; y++ {
		for x := float32(listWidth + 1); x < width; x += 3 {
			if p := pixel(tt, width, x, y); !near(p, bg) {
				t.Fatalf("the row paints %v at (%v, %v), past its %d DIPs", p, x, y, listWidth)
			}
		}
	}
}

// A chosen row in a SoftList takes the soft selection's tint and the
// accent rule on its leading edge, and its unread dot keeps the accent.
func TestSoftListSoftensTheChoice(t *testing.T) {
	setIcons()
	const width = 400
	var st ui.ListState
	chosen := 0
	st.Selected = &chosen
	var page *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) {
		page = frostui.Adapt(c)
		ui.Column(c).Fill().AlignItems(ui.Stretch).Background(page.Background).Children(func() {
			uikit.SoftList(c, &st, 2, func(i int) {
				uikit.SummaryRow(c, uikit.Summary{Title: fmt.Sprintf("Row %d", i), Subtitle: "Plans", Preview: "See you there",
					Unread: true, UnreadLabel: fmt.Sprintf("Unread %d", i)})
			}).Label("Messages")
		})
	}, width, 300)
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Frame()
		tt.Frame()
		tint := frostui.SoftSelection(page).Accent
		mid := float32(uikit.SummaryHeight / 2)
		if p := pixel(tt, width, width-4, mid); !near(p, tint) {
			t.Errorf("dark=%v: the chosen row's face is %v, want the tint %v", dark, p, tint)
		}
		if p := pixel(tt, width, 0, mid); !near(p, page.Accent) {
			t.Errorf("dark=%v: the chosen row's leading edge is %v, want the accent %v", dark, p, page.Accent)
		}
		if p := pixel(tt, width, width-4, uikit.SummaryHeight+mid); !near(p, page.Background) {
			t.Errorf("dark=%v: the other row's face is %v, want the background %v", dark, p, page.Background)
		}
		dot, ok := tt.Access("Unread 0")
		if !ok {
			t.Fatalf("no dot on the chosen row in %q", tt.Texts())
		}
		if p := pixel(tt, width, dot.Bounds.X+dot.Bounds.W/2, dot.Bounds.Y+dot.Bounds.H/2); !near(p, page.Accent) {
			t.Errorf("dark=%v: the chosen row's dot is %v, want the accent %v", dark, p, page.Accent)
		}
	}
}

// P-211: a row shows as many lines of preview as it is told, none for 0,
// and its height follows, whatever its text. A preview line is as high as
// its font makes it, which differs by platform (16 on Linux; Windows
// measured 16.63, frostyard/frostmail#313), so the test measures a line
// and holds every count of lines to what the usual 2 show: a row never
// cuts more of its preview than the 2-line row does in the same font.
func TestSummaryRowLinesSetTheHeightAndThePreview(t *testing.T) {
	setIcons()
	const slack = 0.01
	long := strings.Repeat("several words of a preview ", 40)
	for lines, height := range map[int]float32{0: 52, 1: 68, 2: 84, 3: 101, 4: 118, 5: 135} {
		if got := uikit.SummaryHeightFor(lines); got != height {
			t.Errorf("SummaryHeightFor(%d) = %v, want %v", lines, got, height)
		}
	}
	if uikit.SummaryHeightFor(2) != uikit.SummaryHeight || uikit.SummaryHeightFor(-3) != 52 {
		t.Error("2 lines is the usual height, and fewer than none is none")
	}
	draw := func(lines int, preview string) (row, box ui.Rect, shown bool) {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Width(320).AlignItems(ui.Stretch).Children(func() {
				uikit.SummaryRowLines(c, uikit.Summary{Title: "Ann Lee", Date: "9:41", Subtitle: "Plans", Preview: preview}, lines).Label("the row")
			})
		}, 320, 240)
		tt.Frame()
		row, ok := tt.Find("the row")
		if !ok {
			t.Fatalf("%d lines: no row", lines)
		}
		box, shown = tt.Find(preview)
		return row, box, shown && preview != ""
	}
	// The font's line: one line of preview in a row with room for 5.
	_, one, ok := draw(5, "short")
	if !ok || one.H < 12 || one.H > 24 {
		t.Fatalf("one line of preview is %v high", one.H)
	}
	line := one.H
	// What the usual row cuts of its 2 lines in this font: nothing where a
	// line is 16 high.
	_, two, _ := draw(2, long)
	cut := max(2*line-two.H, 0)
	if cut > line/4 {
		t.Fatalf("the usual row cuts %v of its 2 lines of %v", cut, line)
	}
	for _, lines := range []int{0, 1, 2, 3, 4, 5} {
		for _, preview := range []string{long, "short", ""} {
			what := fmt.Sprintf("%d lines, preview %q…", lines, firstWord(preview))
			row, box, shown := draw(lines, preview)
			if row.H != uikit.SummaryHeightFor(lines) {
				t.Errorf("%s: the row is %v high, want %v", what, row.H, uikit.SummaryHeightFor(lines))
			}
			if preview == "" {
				continue
			}
			if (lines > 0) != shown {
				t.Errorf("%s: the preview shows = %v", what, shown)
			}
			if !shown {
				continue
			}
			// A long preview fills the lines asked for, a short one takes
			// 1; neither is cut more than the usual row cuts, nor leaves
			// the row.
			full := line
			if preview == long {
				full = float32(lines) * line
			}
			if box.H > full+slack || box.H < full-cut-slack {
				t.Errorf("%s: the preview is %v high, want %v, less at most %v", what, box.H, full, cut)
			}
			if box.Y+box.H > row.Y+row.H+slack {
				t.Errorf("%s: its box %+v leaves the row %+v", what, box, row)
			}
		}
	}
}

func firstWord(s string) string {
	word, _, _ := strings.Cut(s, " ")
	return word
}
