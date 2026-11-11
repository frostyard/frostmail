package pim

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // decodes PNG photos
	"io"
	"os"

	"github.com/frostyard/frostmail/api"
)

// photoSize is the widest and tallest a contact photo is; photoLimit the
// most bytes maild takes; photoFileLimit the largest file the editor reads,
// and photoPixelLimit the most pixels it decodes: a small PNG can declare
// more than memory holds.
const (
	photoSize       = 512
	photoLimit      = 256 * 1024
	photoFileLimit  = 20_000_000
	photoPixelLimit = 40_000_000
)

var (
	errPhotoFileTooBig = errors.New("the photo's file is over 20 MB")
	errPhotoTooBig     = errors.New("the photo is over 40 million pixels")
)

// readPhoto reads the chosen file, refusing one over photoFileLimit before
// it holds the bytes.
func readPhoto(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the file the user chose
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > photoFileLimit {
		return nil, errPhotoFileTooBig
	}
	// The size can be wrong, as a pipe's or a growing file's.
	data, err := io.ReadAll(io.LimitReader(f, photoFileLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > photoFileLimit {
		return nil, errPhotoFileTooBig
	}
	return data, nil
}

// scalePhoto crops an image to its centered square and scales it to at
// most 512 by 512, as a JPEG (docs/specs/pim-ui.md, Photo well). It reads
// the image's header first, and refuses what would take too much memory
// to decode.
func scalePhoto(data []byte) (*api.Photo, error) {
	if len(data) > photoFileLimit {
		return nil, errPhotoFileTooBig
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read the photo: %w", err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > photoPixelLimit {
		return nil, errPhotoTooBig
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read the photo: %w", err)
	}
	dst := cropScale(src, photoSize)
	for quality := 90; ; quality -= 15 {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("encode the photo: %w", err)
		}
		if out.Len() <= photoLimit || quality <= 30 {
			return &api.Photo{ContentType: "image/jpeg", Data: base64.StdEncoding.EncodeToString(out.Bytes())}, nil
		}
	}
}

// cropScale averages the source's centered square down to at most size.
func cropScale(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	n := min(side, size)
	dst := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		sy0, sy1 := y*side/n, max((y+1)*side/n, y*side/n+1)
		for x := range n {
			sx0, sx1 := x*side/n, max((x+1)*side/n, x*side/n+1)
			var r, g, bl, a, count uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					cr, cg, cb, ca := src.At(x0+sx, y0+sy).RGBA()
					r, g, bl, a, count = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), count+1
				}
			}
			dst.Set(x, y, color.RGBA64{uint16(r / count), uint16(g / count), uint16(bl / count), uint16(a / count)})
		}
	}
	return dst
}
