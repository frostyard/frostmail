package mainwin_test

import (
	"image/png"
	"os"

	"github.com/egoist/mygo/ui"
)

func writePNG(path string, tt *ui.Tester) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, tt.Image())
}
