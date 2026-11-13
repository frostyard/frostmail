package uikit

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// Thumbnail decodes a picture from mail and returns it scaled down to fit
// maxW × maxH pixels, upright as a JPEG's EXIF orientation says. A picture
// that fits is returned at its own size. The picture's header must first
// say it is small enough to decode (MaxBitmapPixels): the full picture is
// in memory only while this runs.
func Thumbnail(data []byte, maxW, maxH int) (*image.RGBA, error) {
	if maxW < 1 || maxH < 1 {
		return nil, fmt.Errorf("a thumbnail of %d × %d", maxW, maxH)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read the picture's header: %w", err)
	}
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > MaxBitmapPixels {
		return nil, fmt.Errorf("the picture is too large or empty: %d × %d", cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode the picture: %w", err)
	}
	orientation := exifOrientation(data)
	if orientation >= 5 { // turned a quarter: the box is for the upright picture
		maxW, maxH = maxH, maxW
	}
	return upright(scaleDown(src, maxW, maxH), orientation), nil
}

// samples is how many points a side scaleDown reads of the source for one
// pixel of the thumbnail: enough to smooth a photo, and far fewer reads
// than every pixel of a large one.
const samples = 4

// scaleDown is src scaled to fit maxW × maxH, never up, each pixel the
// mean of a grid of points in its part of src.
func scaleDown(src image.Image, maxW, maxH int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > maxW || h > maxH {
		if int64(w)*int64(maxH) > int64(h)*int64(maxW) {
			w, h = maxW, max(1, int(int64(h)*int64(maxW)/int64(w)))
		} else {
			w, h = max(1, int(int64(w)*int64(maxH)/int64(h))), maxH
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0, y1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+(y+1)*b.Dy()/h
		for x := range w {
			x0, x1 := b.Min.X+x*b.Dx()/w, b.Min.X+(x+1)*b.Dx()/w
			var r, g, bl, a, n uint32
			for sy := range samples {
				py := y0 + sy*(y1-y0)/samples
				for sx := range samples {
					px := x0 + sx*(x1-x0)/samples
					cr, cg, cb, ca := src.At(px, py).RGBA()
					r, g, bl, a, n = r+cr, g+cg, bl+cb, a+ca, n+1
					if x1-x0 <= 1 {
						break
					}
				}
				if y1-y0 <= 1 {
					break
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), uint8(a / n >> 8)})
		}
	}
	return dst
}

// upright turns and mirrors img as an EXIF orientation (1 to 8) says the
// camera held it; any other value leaves it.
func upright(img *image.RGBA, orientation int) *image.RGBA {
	if orientation < 2 || orientation > 8 {
		return img
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	dw, dh := w, h
	if orientation >= 5 {
		dw, dh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch orientation {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			out.SetRGBA(dx, dy, img.RGBAAt(x, y))
		}
	}
	return out
}

// exifOrientation is the EXIF orientation of a JPEG, 1 to 8, or 0 when
// data is not a JPEG or names none. It reads the segments before the
// picture's data and trusts no length further than data reaches.
func exifOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return 0
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // the picture's data, or its end
			return 0
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return 0
		}
		seg := data[i+4 : i+2+size]
		if marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 0
}

// tiffOrientation reads tag 0x0112 of a TIFF block's first directory.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 0
	}
	var order binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0
	}
	if order.Uint16(t[2:]) != 42 {
		return 0
	}
	dir := int64(order.Uint32(t[4:]))
	if dir+2 > int64(len(t)) {
		return 0
	}
	count := int64(order.Uint16(t[dir:]))
	for i := range count {
		entry := dir + 2 + i*12
		if entry+12 > int64(len(t)) {
			return 0
		}
		if order.Uint16(t[entry:]) == 0x0112 {
			if v := int(order.Uint16(t[entry+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 0
		}
	}
	return 0
}
