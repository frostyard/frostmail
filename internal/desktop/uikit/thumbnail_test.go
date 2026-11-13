package uikit

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// quadrants is a w × h picture whose corners differ: red top left, green
// top right, blue bottom left, white bottom right.
func quadrants(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{255, 0, 0, 255}
			switch {
			case x >= w/2 && y < h/2:
				c = color.RGBA{0, 255, 0, 255}
			case x < w/2 && y >= h/2:
				c = color.RGBA{0, 0, 255, 255}
			case x >= w/2 && y >= h/2:
				c = color.RGBA{255, 255, 255, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func pngOf(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// jpegWithOrientation is img as a JPEG whose EXIF block names orientation.
func jpegWithOrientation(t *testing.T, img image.Image, orientation uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08\x00\x01")
	tiff = append(tiff, 0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01)
	tiff = binary.BigEndian.AppendUint16(tiff, orientation)
	tiff = append(tiff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
	exif := append([]byte("Exif\x00\x00"), tiff...)
	seg := binary.BigEndian.AppendUint16([]byte{0xFF, 0xE1}, uint16(len(exif)+2))
	seg = append(seg, exif...)
	data := buf.Bytes()
	return append(append(append([]byte{}, data[:2]...), seg...), data[2:]...)
}

// name is the corner color nearest to c: JPEG moves the values a little.
func name(c color.RGBA) string {
	switch {
	case c.R > 200 && c.G > 200 && c.B > 200:
		return "white"
	case c.R > 200 && c.G < 60 && c.B < 60:
		return "red"
	case c.G > 200 && c.R < 60 && c.B < 60:
		return "green"
	case c.B > 200 && c.R < 60 && c.G < 60:
		return "blue"
	}
	return "other"
}

// corners names the picture's corners: top left, top right, bottom left,
// bottom right.
func corners(img *image.RGBA) [4]string {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	return [4]string{name(img.RGBAAt(1, 1)), name(img.RGBAAt(w-2, 1)), name(img.RGBAAt(1, h-2)), name(img.RGBAAt(w-2, h-2))}
}

func TestThumbnailScalesDownToFitAndNeverUp(t *testing.T) {
	for _, tc := range []struct {
		w, h, maxW, maxH, wantW, wantH int
	}{
		{800, 400, 200, 200, 200, 100}, // wide: the width decides
		{400, 800, 200, 200, 100, 200}, // tall: the height decides
		{800, 600, 440, 240, 320, 240},
		{100, 50, 440, 240, 100, 50}, // fits: its own size
		{3000, 10, 100, 100, 100, 1}, // never thinner than a pixel
	} {
		got, err := Thumbnail(pngOf(t, quadrants(tc.w, tc.h)), tc.maxW, tc.maxH)
		if err != nil {
			t.Fatalf("%d × %d: %v", tc.w, tc.h, err)
		}
		if b := got.Bounds(); b.Dx() != tc.wantW || b.Dy() != tc.wantH {
			t.Errorf("%d × %d in %d × %d = %d × %d, want %d × %d", tc.w, tc.h, tc.maxW, tc.maxH, b.Dx(), b.Dy(), tc.wantW, tc.wantH)
		}
	}
	got, err := Thumbnail(pngOf(t, quadrants(800, 400)), 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	if c := corners(got); c != [4]string{"red", "green", "blue", "white"} {
		t.Errorf("the scaled picture's corners = %v", c)
	}
}

// Each EXIF orientation shows upright: the corners land where a viewer
// that honors the tag shows them, and a quarter turn swaps the sides.
func TestThumbnailIsUpright(t *testing.T) {
	want := map[uint16][4]string{
		1: {"red", "green", "blue", "white"},
		2: {"green", "red", "white", "blue"},
		3: {"white", "blue", "green", "red"},
		4: {"blue", "white", "red", "green"},
		5: {"red", "blue", "green", "white"},
		6: {"blue", "red", "white", "green"},
		7: {"white", "green", "blue", "red"},
		8: {"green", "white", "red", "blue"},
	}
	for o, corner := range want {
		got, err := Thumbnail(jpegWithOrientation(t, quadrants(400, 200), o), 100, 100)
		if err != nil {
			t.Fatalf("orientation %d: %v", o, err)
		}
		w, h := 100, 50
		if o >= 5 {
			w, h = 50, 100
		}
		if b := got.Bounds(); b.Dx() != w || b.Dy() != h {
			t.Errorf("orientation %d: %d × %d, want %d × %d", o, b.Dx(), b.Dy(), w, h)
		}
		if c := corners(got); c != corner {
			t.Errorf("orientation %d: corners %v, want %v", o, c, corner)
		}
	}
}

func TestThumbnailRefuses(t *testing.T) {
	// A header that declares more pixels than the bound, in a few bytes.
	var huge bytes.Buffer
	if err := png.Encode(&huge, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := huge.Bytes()
	binary.BigEndian.PutUint32(b[16:], 50_000)
	binary.BigEndian.PutUint32(b[20:], 50_000)
	for name, data := range map[string][]byte{
		"text":            []byte("not a picture"),
		"nothing":         nil,
		"a cut picture":   pngOf(t, quadrants(64, 64))[:40],
		"too many pixels": b,
	} {
		if img, err := Thumbnail(data, 100, 100); err == nil {
			t.Errorf("%s gave a %v thumbnail", name, img.Bounds())
		}
	}
	if _, err := Thumbnail(pngOf(t, quadrants(8, 8)), 0, 10); err == nil {
		t.Error("a box of no width was accepted")
	}
}

// A JPEG's segments are read no further than the data reaches.
func TestExifOrientationOfBrokenData(t *testing.T) {
	good := jpegWithOrientation(t, quadrants(16, 16), 6)
	if got := exifOrientation(good); got != 6 {
		t.Fatalf("orientation = %d", got)
	}
	for n := range len(good) {
		_ = exifOrientation(good[:n]) // must not panic
	}
	bad := bytes.Clone(good)
	binary.BigEndian.PutUint32(bad[2+4+6+4:], 0xFFFFFFF0) // the directory's offset, past the block
	if got := exifOrientation(bad); got != 0 {
		t.Errorf("a directory past the block gave %d", got)
	}
	if got := exifOrientation(pngOf(t, quadrants(8, 8))); got != 0 {
		t.Errorf("a PNG gave %d", got)
	}
}
