package uikit_test

import (
	"image"
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
