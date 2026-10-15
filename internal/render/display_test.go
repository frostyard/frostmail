package render

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/mimex"
)

// displayCopy is newsletter as a display copy (ADR-0037) that left out its
// inline image and its PDF.
func displayCopy(t *testing.T) []byte {
	t.Helper()
	raw := string(newsletter(t, "https://example.test"))
	img := base64.StdEncoding.EncodeToString(pngBytes(t))
	pdf := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 fake"))
	for _, cut := range []struct{ header, body string }{{"Content-ID: <logo@x>\r\n", img}, {"Content-Transfer-Encoding: base64\r\n", pdf}} {
		was := cut.header + "\r\n" + cut.body
		if !strings.Contains(raw, was) {
			t.Fatalf("no %q in the fixture", cut.header)
		}
		raw = strings.Replace(raw, was, cut.header+mimex.OmittedHeader+": 900\r\n\r\n", 1)
	}
	return []byte(raw)
}

func TestRenderDisplayCopy(t *testing.T) {
	cache := &PartsCache{Root: t.TempDir()}
	r := &Renderer{Parts: cache}
	got, err := r.Render(t.Context(), 7, displayCopy(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.HTML, "data:image/png") || cache.Has("m/7/1.2.png") {
		t.Errorf("an omitted image was embedded or cached:\n%s", got.HTML)
	}
	if !strings.Contains(got.HTML, `<a href="https://example.test/read">Read</a>`) || !strings.Contains(got.Text, "Read") {
		t.Errorf("the text is lost: %+v", got)
	}
}

func TestPartOfDisplayCopy(t *testing.T) {
	r := &Renderer{Parts: &PartsCache{Root: t.TempDir()}}
	if _, err := r.Part(7, displayCopy(t), "2"); !errors.Is(err, ErrOmitted) {
		t.Errorf("Part(omitted) = %v, want ErrOmitted", err)
	}
	if pf, err := r.Part(7, displayCopy(t), "1.1"); err != nil || pf.ContentType != "text/html" {
		t.Errorf("Part(kept) = %+v, %v", pf, err)
	}
}
