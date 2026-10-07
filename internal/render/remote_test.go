package render

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// loopback allows the httptest servers these tests use.
func loopback(a netip.Addr) bool { return a.IsLoopback() }

func TestPublicAddr(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8": true, "1.1.1.1": true, "2606:4700:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false, "224.0.0.1": false,
		"198.18.0.1": false, "::1": false, "fe80::1": false, "fc00::1": false, "::": false,
		"::ffff:10.0.0.1": false, "::ffff:8.8.8.8": true, "64:ff9b::a00:1": false,
	}
	for s, want := range cases {
		if got := PublicAddr(netip.MustParseAddr(s)); got != want {
			t.Errorf("PublicAddr(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestFetchStoresAndCaches(t *testing.T) {
	img := pngBytes(t)
	var hits atomic.Int32
	var referer atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		referer.Store(r.Header.Get("Referer"))
		if r.Header.Get("Cookie") != "" {
			t.Errorf("cookie sent: %q", r.Header.Get("Cookie"))
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(img)
	}))
	defer srv.Close()
	cache := &PartsCache{Root: t.TempDir()}
	f := NewFetcher(cache, loopback)
	rel, err := f.Fetch(t.Context(), srv.URL+"/a.png")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rel, "r/") || !strings.HasSuffix(rel, ".png") || !cache.Has(rel) {
		t.Fatalf("Fetch = %q", rel)
	}
	again, err := f.Fetch(t.Context(), srv.URL+"/a.png")
	if err != nil || again != rel || hits.Load() != 1 {
		t.Fatalf("second fetch = %q, %v after %d requests; want the cached file", again, err, hits.Load())
	}
	if r, _ := referer.Load().(string); r != "" {
		t.Errorf("Referer = %q", r)
	}
}

func TestFetchRefusesLocalAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a refused address was contacted")
	}))
	defer srv.Close()
	f := NewFetcher(&PartsCache{Root: t.TempDir()}, nil)
	if _, err := f.Fetch(t.Context(), srv.URL+"/a.png"); !errors.Is(err, ErrRefusedAddress) {
		t.Fatalf("fetch from loopback = %v, want ErrRefusedAddress", err)
	}
	if _, err := f.Fetch(t.Context(), "http://localhost:1/a.png"); !errors.Is(err, ErrRefusedAddress) {
		t.Fatalf("fetch from localhost = %v, want ErrRefusedAddress", err)
	}
}

func TestFetchRejectsWhatIsNotAnImage(t *testing.T) {
	img := pngBytes(t)
	big := bytes.Repeat([]byte{0}, MaxImageBytes+10)
	copy(big, img)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/html":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<p>hi</p>"))
		case "/lie":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("<html><script>x</script></html>"))
		case "/svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="x()"/>`))
		case "/big":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(big)
		case "/missing":
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := NewFetcher(&PartsCache{Root: t.TempDir()}, loopback)
	for _, p := range []string{"/html", "/lie", "/svg", "/big", "/missing"} {
		if rel, err := f.Fetch(t.Context(), srv.URL+p); err == nil {
			t.Errorf("fetch %s = %q, want an error", p, rel)
		}
	}
	for _, u := range []string{"ftp://e.test/a.png", "file:///etc/passwd", "javascript:alert(1)", "/relative.png"} {
		if _, err := f.Fetch(t.Context(), u); err == nil {
			t.Errorf("fetch %s succeeded", u)
		}
	}
}

func TestFetchLimitsRedirects(t *testing.T) {
	img := pngBytes(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n int
		if _, err := fmt.Sscanf(r.URL.Path, "/hop/%d", &n); err == nil && n > 0 {
			http.Redirect(w, r, fmt.Sprintf("%s/hop/%d", srv.URL, n-1), http.StatusFound)
			return
		}
		if r.Header.Get("Referer") != "" {
			t.Errorf("redirect sent Referer %q", r.Header.Get("Referer"))
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(img)
	}))
	defer srv.Close()
	f := NewFetcher(&PartsCache{Root: t.TempDir()}, loopback)
	if _, err := f.Fetch(t.Context(), srv.URL+"/hop/3"); err != nil {
		t.Errorf("three redirects: %v", err)
	}
	if _, err := f.Fetch(t.Context(), srv.URL+"/hop/4"); err == nil {
		t.Error("four redirects succeeded")
	}
}
