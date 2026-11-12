package render

import (
	"bytes"
	"os"
	"path/filepath"
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
