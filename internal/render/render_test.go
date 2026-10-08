package render

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newsletter builds a multipart/related message: HTML with an inline cid
// image, a remote image, a tracking pixel and a hidden image, plus a PDF
// attachment.
func newsletter(t *testing.T, remote string) []byte {
	t.Helper()
	img := base64.StdEncoding.EncodeToString(pngBytes(t))
	html := `<html><head><style>.hero{background:url(cid:logo@x)}</style></head><body bgcolor="#f0f0f0">` +
		`<p class="hero"><img src="cid:logo@x" alt="Logo"></p>` +
		`<img src="` + remote + `/photo.png" alt="Photo">` +
		`<img src="https://track.example.test/p.gif" width="1" height="1">` +
		`<img src="https://example.test/x.png" style="display:none">` +
		`<p><a href="https://example.test/read">Read</a> <a href="javascript:alert(1)">x</a></p></body></html>`
	return []byte(strings.Join([]string{
		"From: News <news@example.test>",
		"To: test1@mailtest.test",
		"Subject: October",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="outer"`,
		"",
		"--outer",
		`Content-Type: multipart/related; boundary="rel"`,
		"",
		"--rel",
		"Content-Type: text/html; charset=utf-8",
		"",
		html,
		"--rel",
		"Content-Type: image/png",
		"Content-Transfer-Encoding: base64",
		"Content-ID: <logo@x>",
		"",
		img,
		"--rel--",
		"--outer",
		`Content-Type: application/pdf; name="report.pdf"`,
		`Content-Disposition: attachment; filename="../../report.pdf"`,
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 fake")),
		"--outer--",
		"",
	}, "\r\n"))
}

func imageServer(t *testing.T) *httptest.Server {
	t.Helper()
	img := pngBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(img)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRenderWithoutRemoteContent(t *testing.T) {
	srv := imageServer(t)
	cache := &PartsCache{Root: t.TempDir()}
	r := &Renderer{Parts: cache, Fetcher: NewFetcher(cache, loopback)}
	got, err := r.Render(t.Context(), 7, newsletter(t, srv.URL), false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Remote != 1 || got.Trackers != 2 {
		t.Errorf("remote %d trackers %d, want 1 and 2", got.Remote, got.Trackers)
	}
	// Images are embedded: WebKitGTK does not load mailpart:// inside the
	// reader's frame. The decoded part is still cached for opening.
	inline := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes(t))
	if strings.Count(got.HTML, inline) != 2 || !cache.Has("m/7/1.2.png") {
		t.Errorf("inline image not embedded twice or not cached:\n%s", got.HTML)
	}
	if strings.Contains(got.HTML, PartURL) {
		t.Errorf("a mailpart URL is left, which the frame cannot load:\n%s", got.HTML)
	}
	for _, bad := range []string{srv.URL, "track.example.test", "javascript", "x.png"} {
		if strings.Contains(got.HTML, bad) {
			t.Errorf("HTML contains %q:\n%s", bad, got.HTML)
		}
	}
	if !strings.HasPrefix(got.HTML, `<div bgcolor="#f0f0f0">`) || !strings.Contains(got.HTML, `<a href="https://example.test/read">Read</a>`) {
		t.Errorf("layout lost:\n%s", got.HTML)
	}
	if !strings.Contains(got.Text, "Read") {
		t.Errorf("text = %q", got.Text)
	}
	checkSafe(t, got.HTML)
}

func TestRenderWithRemoteContent(t *testing.T) {
	srv := imageServer(t)
	cache := &PartsCache{Root: t.TempDir()}
	r := &Renderer{Parts: cache, Fetcher: NewFetcher(cache, loopback)}
	got, err := r.Render(t.Context(), 7, newsletter(t, srv.URL), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Remote != 0 || got.Trackers != 2 {
		t.Errorf("remote %d trackers %d, want 0 and 2", got.Remote, got.Trackers)
	}
	if strings.Count(got.HTML, `src="data:image/png;base64,`) != 2 || strings.Contains(got.HTML, "frostmail-pending") ||
		strings.Contains(got.HTML, PartURL) {
		t.Errorf("remote image not embedded:\n%s", got.HTML)
	}
	checkSafe(t, got.HTML)
}

func TestRenderCountsFailedRemoteImages(t *testing.T) {
	cache := &PartsCache{Root: t.TempDir()}
	r := &Renderer{Parts: cache, Fetcher: NewFetcher(cache, nil)} // refuses loopback
	srv := imageServer(t)
	got, err := r.Render(t.Context(), 7, newsletter(t, srv.URL), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Remote != 1 || strings.Contains(got.HTML, "frostmail-pending") || strings.Contains(got.HTML, srv.URL) {
		t.Errorf("remote %d:\n%s", got.Remote, got.HTML)
	}
}

func TestRenderPlainMessage(t *testing.T) {
	raw := []byte("From: a@example.test\r\nSubject: hi\r\nContent-Type: text/plain\r\n\r\nHello <b>there</b>\r\n")
	r := &Renderer{Parts: &PartsCache{Root: t.TempDir()}}
	got, err := r.Render(t.Context(), 1, raw, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.HTML != "" || got.Text != "Hello <b>there</b>" {
		t.Errorf("render = %+v", got)
	}
}

func TestPart(t *testing.T) {
	cache := &PartsCache{Root: t.TempDir()}
	r := &Renderer{Parts: cache}
	raw := newsletter(t, "https://example.test")
	pf, err := r.Part(7, raw, "2")
	if err != nil {
		t.Fatal(err)
	}
	if pf.Path != "m/7/2/report.pdf" || pf.Filename != "report.pdf" || pf.ContentType != "application/pdf" || pf.Size != 13 {
		t.Errorf("part = %+v", pf)
	}
	data, err := os.ReadFile(filepath.Join(cache.Root, "m", "7", "2", "report.pdf"))
	if err != nil || string(data) != "%PDF-1.4 fake" {
		t.Errorf("file = %q, %v", data, err)
	}
	if _, err := r.Part(7, raw, "9"); !errors.Is(err, ErrNoPart) {
		t.Errorf("missing part error = %v", err)
	}
}

func TestSafeFilename(t *testing.T) {
	cases := []struct{ name, path, ctype, want string }{
		{"report.pdf", "2", "application/pdf", "report.pdf"},
		{"../../etc/passwd", "2", "", "etcpasswd"},
		{"..hidden", "2", "", "hidden"},
		{"a\x00b\nc", "2", "", "abc"},
		{"Ünal Rechnung.pdf", "2", "", "Ünal Rechnung.pdf"},
		{"", "1.2", "image/png", "part-1.2.png"},
		{"", "3", "application/zip", "part-3.bin"},
		{strings.Repeat("é", 150), "2", "", strings.Repeat("é", 100)},
	}
	for _, tc := range cases {
		if got := SafeFilename(tc.name, tc.path, tc.ctype); got != tc.want {
			t.Errorf("SafeFilename(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPartsCacheStaysInside(t *testing.T) {
	c := &PartsCache{Root: t.TempDir()}
	for _, rel := range []string{"../x", "/etc/x", "", "a/../../x"} {
		if err := c.Write(rel, []byte("x")); err == nil {
			t.Errorf("Write(%q) succeeded", rel)
		}
	}
}

// Past MaxEmbeddedBytes an image keeps its mailpart URL rather than making
// the rendering unbounded.
func TestEmbedderBudget(t *testing.T) {
	cache := &PartsCache{Root: t.TempDir()}
	small := pngBytes(t)
	if err := cache.Write("r/a.png", small); err != nil {
		t.Fatal(err)
	}
	if err := cache.Write("r/b.avif", small); err != nil {
		t.Fatal(err)
	}
	e := &embedder{parts: cache, used: MaxEmbeddedBytes - len(small)}
	if got := e.url("r/a.png"); !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Errorf("an image that fits = %q", got)
	}
	if got := e.url("r/a.png"); got != PartURL+"r/a.png" {
		t.Errorf("an image past the budget = %q", got)
	}
	e.used = 0
	if got := e.url("r/b.avif"); got != PartURL+"r/b.avif" {
		t.Errorf("a type the sanitizer does not admit as data = %q", got)
	}
	if got := e.url("r/missing.png"); got != PartURL+"r/missing.png" {
		t.Errorf("a missing file = %q", got)
	}
}
