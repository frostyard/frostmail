package render

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPartKeepsTheCachedFile: message.part called again for a part already in
// the cache returns the same file and reports the cached file's size; it
// neither reads the part again nor writes the file again.
func TestPartKeepsTheCachedFile(t *testing.T) {
	cache := &PartsCache{Root: t.TempDir()}
	r := &Renderer{Parts: cache}
	raw := newsletter(t, "https://example.test")
	first, err := r.Part(7, raw, "2")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(cache.Root, filepath.FromSlash(first.Path))
	// A marker shorter than the part tells a rewrite from a kept file.
	marker := []byte("%PDF-1.4")
	if err := os.WriteFile(file, marker, 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := r.Part(7, raw, "2")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(data, marker) {
		t.Fatalf("the cached file holds %q, %v; want it kept: %q", data, err, marker)
	}
	if again.Size != int64(len(marker)) {
		t.Errorf("Size = %d, want the cached file's %d bytes", again.Size, len(marker))
	}
	if again.Path != first.Path || again.Filename != first.Filename || again.ContentType != first.ContentType {
		t.Errorf("part = %+v, want the same file as %+v", again, first)
	}
}

// TestRenderFindsACIDURLInAnyCase: the cid: check that skips the image parts
// is case-insensitive, so an uppercase CID: URL still embeds its image.
func TestRenderFindsACIDURLInAnyCase(t *testing.T) {
	cache := &PartsCache{Root: t.TempDir()}
	r := &Renderer{Parts: cache}
	img := base64.StdEncoding.EncodeToString(pngBytes(t))
	raw := []byte(strings.Join([]string{
		"From: News <news@example.test>",
		"Subject: Upper",
		"MIME-Version: 1.0",
		`Content-Type: multipart/related; boundary="rel"`,
		"",
		"--rel",
		"Content-Type: text/html; charset=utf-8",
		"",
		`<p><img src="CID:logo@x" alt="Logo"></p>`,
		"--rel",
		"Content-Type: image/png",
		"Content-Transfer-Encoding: base64",
		"Content-ID: <logo@x>",
		"",
		img,
		"--rel--",
		"",
	}, "\r\n"))
	got, err := r.Render(t.Context(), 7, raw, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes(t)); !strings.Contains(got.HTML, want) {
		t.Errorf("the CID: image is not embedded:\n%s", got.HTML)
	}
}
