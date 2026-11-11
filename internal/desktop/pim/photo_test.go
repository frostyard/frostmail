package pim

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// pngHeader is a PNG of nothing but a header declaring its size.
func pngHeader(width, height uint32) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := binary.BigEndian.AppendUint32(nil, width)
	ihdr = binary.BigEndian.AppendUint32(ihdr, height)
	ihdr = append(ihdr, 8, 6, 0, 0, 0) // 8 bits, RGBA
	typed := append([]byte("IHDR"), ihdr...)
	b.Write(binary.BigEndian.AppendUint32(nil, uint32(len(ihdr))))
	b.Write(typed)
	b.Write(binary.BigEndian.AppendUint32(nil, crc32.ChecksumIEEE(typed)))
	return b.Bytes()
}

func TestScalePhotoRefusesAHugeHeader(t *testing.T) {
	data := pngHeader(50_000, 50_000)
	if cfg, err := png.DecodeConfig(bytes.NewReader(data)); err != nil || cfg.Width != 50_000 {
		t.Fatalf("the fixture is not a valid header: %+v, %v", cfg, err)
	}
	if _, err := scalePhoto(data); !errors.Is(err, errPhotoTooBig) {
		t.Errorf("err = %v", err)
	}
}

func TestScalePhotoRefusesOnePixelOverTheLimit(t *testing.T) {
	if _, err := scalePhoto(pngHeader(photoPixelLimit+1, 1)); !errors.Is(err, errPhotoTooBig) {
		t.Errorf("err = %v", err)
	}
}

func TestScalePhotoRefusesTooManyBytes(t *testing.T) {
	if _, err := scalePhoto(make([]byte, photoFileLimit+1)); !errors.Is(err, errPhotoFileTooBig) {
		t.Errorf("err = %v", err)
	}
}

func TestReadPhotoRefusesALargeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, photoFileLimit+1); err != nil { // sparse: nothing is written
		t.Fatal(err)
	}
	if _, err := readPhoto(path); !errors.Is(err, errPhotoFileTooBig) {
		t.Errorf("err = %v", err)
	}
	if err := os.Truncate(path, photoFileLimit); err != nil {
		t.Fatal(err)
	}
	if data, err := readPhoto(path); err != nil || len(data) != photoFileLimit {
		t.Errorf("a file at the limit: %d bytes, %v", len(data), err)
	}
}
