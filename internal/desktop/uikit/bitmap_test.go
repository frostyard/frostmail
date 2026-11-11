package uikit

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"strings"
	"testing"
)

// A few bytes that declare a huge image are refused from the header.
func TestDecodeBitmapRefusesAHugeHeader(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 0, 17)
	ihdr = append(ihdr, "IHDR"...)
	ihdr = binary.BigEndian.AppendUint32(ihdr, 50000)
	ihdr = binary.BigEndian.AppendUint32(ihdr, 50000)
	ihdr = append(ihdr, 8, 6, 0, 0, 0)
	_ = binary.Write(&b, binary.BigEndian, uint32(13))
	b.Write(ihdr)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(ihdr))
	if _, err := DecodeBitmap(b.Bytes()); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("err = %v, want too large", err)
	}
}

func TestDecodeBitmapDecodesASmallPicture(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if bmp, err := DecodeBitmap(b.Bytes()); err != nil || bmp == nil {
		t.Errorf("bitmap = %v, err = %v", bmp, err)
	}
}
