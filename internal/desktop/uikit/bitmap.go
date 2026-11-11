package uikit

import (
	"bytes"
	"fmt"
	"image"

	"github.com/egoist/mygo/ui"
)

// MaxBitmapPixels bounds a picture decoded from bytes the app did not make: a
// few bytes can declare an image that takes gigabytes to decode.
const MaxBitmapPixels = 40_000_000

// DecodeBitmap decodes a picture from mail or a contact, after its header
// says it is small enough.
func DecodeBitmap(data []byte) (*ui.Bitmap, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read the picture's header: %w", err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxBitmapPixels {
		return nil, fmt.Errorf("the picture is too large: %d × %d", cfg.Width, cfg.Height)
	}
	return ui.DecodeBitmap(data)
}
