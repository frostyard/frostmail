package icons_test

import (
	"bytes"
	"image/png"
	"os"
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/icons"
)

// The tray icon is the app's own icon, so the panel shows the app: a PNG,
// square, large enough for a tray (32 pixels or more), and the same bytes
// as the icon the app ships.
func TestTrayIconIsTheAppsIcon(t *testing.T) {
	icon := icons.Tray()
	img, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		t.Fatalf("the tray icon is not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != b.Dy() || b.Dx() < 32 {
		t.Errorf("the tray icon is %d by %d, want a square of 32 pixels or more", b.Dx(), b.Dy())
	}
	shipped, err := os.ReadFile("../../../packaging/icons/128x128.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(icon, shipped) {
		t.Error("the tray icon differs from packaging/icons/128x128.png; copy it again")
	}
}
