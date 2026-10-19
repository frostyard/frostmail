package lodge

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/render"
)

func newContent(t *testing.T) *Content {
	t.Helper()
	c, _ := newContentBlobs(t)
	return c
}

func newContentBlobs(t *testing.T) (*Content, *blob.Store) {
	t.Helper()
	parts := &render.PartsCache{Root: filepath.Join(t.TempDir(), "parts")}
	blobs := blob.New(filepath.Join(t.TempDir(), "blobs"))
	c, err := NewContent(parts, blobs)
	if err != nil {
		t.Fatal(err)
	}
	return c, blobs
}

func get(t *testing.T, c *Content, method, path string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	c.ServeHTTP(rec, req)
	return rec
}

func TestPagesAreBounded(t *testing.T) {
	c := newContent(t)
	first := c.KeepPage([]byte("first"))
	if !strings.HasPrefix(first, "r/") || len(first) != len("r/")+32+len(".html") {
		t.Fatalf("page path = %q, want r/<32 hex>.html", first)
	}
	for range MaxPages - 1 {
		c.KeepPage([]byte("x"))
	}
	if rec := get(t, c, "GET", "/content/"+first); rec.Code != http.StatusOK {
		t.Fatalf("page %d of %d = %d, want it kept", MaxPages, MaxPages, rec.Code)
	}
	c.KeepPage([]byte("one more"))
	if rec := get(t, c, "GET", "/content/"+first); rec.Code != http.StatusNotFound {
		t.Fatalf("the least recently kept page past %d = %d, want 404", MaxPages, rec.Code)
	}

	c = newContent(t)
	big := bytes.Repeat([]byte("a"), MaxPageBytes/2)
	a, b := c.KeepPage(big), c.KeepPage(big)
	if rec := get(t, c, "HEAD", "/content/"+a); rec.Code != http.StatusOK {
		t.Fatalf("two pages of half the budget: the first = %d, want kept", rec.Code)
	}
	c.KeepPage([]byte("past the budget"))
	if rec := get(t, c, "HEAD", "/content/"+a); rec.Code != http.StatusNotFound {
		t.Fatalf("the oldest page past %d bytes = %d, want 404", MaxPageBytes, rec.Code)
	}
	if rec := get(t, c, "HEAD", "/content/"+b); rec.Code != http.StatusOK {
		t.Fatalf("the newer page = %d, want kept", rec.Code)
	}
	if p := c.KeepPage(make([]byte, MaxPageBytes+1)); p != "" {
		t.Fatalf("a page over the budget = %q, want none", p)
	}
}

func TestPageResponse(t *testing.T) {
	c := newContent(t)
	path := c.KeepPage(render.Page(`<p>hi <a href="https://example.com/">x</a></p>`))
	rec := get(t, c, "GET", "/content/"+path)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	for k, want := range map[string]string{
		"Content-Type":            "text/html; charset=utf-8",
		"Content-Security-Policy": PagePolicy,
		"X-Content-Type-Options":  "nosniff",
		"Cache-Control":           "private, no-store",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	body := rec.Body.String()
	for _, want := range []string{`<!doctype html>`, `<base target="_blank">`, `<p>hi <a href="https://example.com/">x</a></p>`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s:\n%s", want, body)
		}
	}
	if !strings.Contains(PagePolicy, "sandbox allow-popups allow-popups-to-escape-sandbox") || strings.Contains(PagePolicy, "allow-scripts") ||
		strings.Contains(PagePolicy, "allow-same-origin") {
		t.Errorf("page policy = %q, want a sandbox with popups only", PagePolicy)
	}
	if rec := get(t, c, "HEAD", "/content/"+path); rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("HEAD = %d with %d bytes, want 200 and no body", rec.Code, rec.Body.Len())
	}
	if rec := get(t, c, "POST", "/content/"+path); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", rec.Code)
	}
	for _, bad := range []string{"r/00000000000000000000000000000000.html", "r/" + strings.ToUpper(path[2:34]) + ".html", "r/x.html", "nothing"} {
		if rec := get(t, c, "GET", "/content/"+bad); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", bad, rec.Code)
		}
	}
}

func TestPartResponse(t *testing.T) {
	c := newContent(t)
	rel := "m/7/2/Résumé (final).pdf"
	data := []byte("%PDF-1.4 0123456789")
	if err := c.parts.Write(rel, data); err != nil {
		t.Fatal(err)
	}
	path := c.PartPath(rel)
	if want := "/m/7/2/R%C3%A9sum%C3%A9%20%28final%29.pdf"; !strings.HasSuffix(path, want) || !strings.HasPrefix(path, "p/") || len(path) != len("p/")+22+len(want) {
		t.Fatalf("part path = %q, want p/<22 characters>%s", path, want)
	}
	rec := get(t, c, "GET", "/content/"+path)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("GET = %d %q", rec.Code, rec.Body)
	}
	for k, want := range map[string]string{
		"Content-Type":            "application/pdf",
		"Content-Security-Policy": PartPolicy,
		"X-Content-Type-Options":  "nosniff",
		"Cache-Control":           "private, no-store",
		"Content-Disposition":     "",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	rec = get(t, c, "GET", "/content/"+path, "Range", "bytes=0-3")
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "%PDF" {
		t.Errorf("one range = %d %q, want 206 %%PDF", rec.Code, rec.Body)
	}
	rec = get(t, c, "GET", "/content/"+path, "Range", "bytes=0-1,4-5")
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), data) {
		t.Errorf("two ranges = %d, want the whole part", rec.Code)
	}
	rec = get(t, c, "GET", "/content/"+path+"?download=1")
	if got, want := rec.Header().Get("Content-Disposition"), "attachment; filename*=UTF-8''R%C3%A9sum%C3%A9%20%28final%29.pdf"; got != want {
		t.Errorf("download disposition = %q, want %q", got, want)
	}

	mac := path[2:24]
	other := c.PartPath("m/8/2/x.pdf")
	for name, bad := range map[string]string{
		"another part's mac":       "p/" + other[2:24] + path[24:],
		"a mac for another key":    "p/AAAAAAAAAAAAAAAAAAAAAA" + path[24:],
		"a separator in a segment": "p/" + mac + "/m%2F7/2/x.pdf",
		"a NUL":                    "p/" + mac + "/m/7/2/x%00.pdf",
		"an empty segment":         "p/" + mac + "/m//2/x.pdf",
	} {
		if rec := get(t, c, "GET", "/content/"+bad); rec.Code != http.StatusNotFound {
			t.Errorf("%s: GET %s = %d, want 404", name, bad, rec.Code)
		}
	}
	// A valid mac never leaves the cache: the parts cache refuses the path.
	out := c.PartPath("../secret")
	if rec := get(t, c, "GET", "/content/"+out); rec.Code != http.StatusNotFound {
		t.Errorf("a signed path out of the cache = %d, want 404", rec.Code)
	}
	// Paths from another start, whose key differs, are gone.
	if rec := get(t, newContent(t), "GET", "/content/"+path); rec.Code != http.StatusNotFound {
		t.Errorf("a part path from before a restart = %d, want 404", rec.Code)
	}
}

func TestSourceResponse(t *testing.T) {
	c, blobs := newContentBlobs(t)
	raw := []byte("From: a@mailtest.test\r\nSubject: Hi\r\n\r\n<script>alert(1)</script>\r\n")
	id, err := blobs.Put(t.Context(), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	path := c.SourcePath(id, "Hi there é.eml")
	if want := "/" + id + "/Hi%20there%20%C3%A9.eml"; !strings.HasPrefix(path, "m/") || !strings.HasSuffix(path, want) || len(path) != len("m/")+22+len(want) {
		t.Fatalf("source path = %q, want m/<22 characters>%s", path, want)
	}

	// To read: text, which the content node's policy keeps inert.
	rec := get(t, c, "GET", "/content/"+path)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), raw) {
		t.Fatalf("GET = %d %q", rec.Code, rec.Body)
	}
	for k, want := range map[string]string{
		"Content-Type":            "text/plain; charset=utf-8",
		"Content-Security-Policy": PartPolicy,
		"X-Content-Type-Options":  "nosniff",
		"Cache-Control":           "private, no-store",
		"Content-Disposition":     "",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	// To save: an .eml download under the name the path carries.
	rec = get(t, c, "GET", "/content/"+path+"?download=1")
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), raw) {
		t.Fatalf("download = %d %q", rec.Code, rec.Body)
	}
	if got, want := rec.Header().Get("Content-Type"), "message/rfc822"; got != want {
		t.Errorf("download Content-Type = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Content-Disposition"), "attachment; filename*=UTF-8''Hi%20there%20%C3%A9.eml"; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
	if rec := get(t, c, "GET", "/content/"+path, "Range", "bytes=0-3"); rec.Code != http.StatusPartialContent || rec.Body.String() != "From" {
		t.Errorf("one range = %d %q, want 206 From", rec.Code, rec.Body)
	}

	mac := path[len("m/") : len("m/")+22]
	other := strings.Repeat("0", 64)
	for name, bad := range map[string]string{
		"another name":          "m/" + mac + "/" + id + "/Other.eml",
		"another blob":          "m/" + mac + "/" + other + "/Hi%20there%20%C3%A9.eml",
		"a bad mac":             "m/" + strings.Repeat("A", 22) + "/" + id + "/Hi%20there%20%C3%A9.eml",
		"a missing blob":        c.SourcePath(other, "x.eml"),
		"an encoded separator":  "m/" + c.mac(sourceKey(id, "a/b.eml")) + "/" + id + "/a%2Fb.eml",
		"the part of that name": "p/" + mac + "/" + id + "/Hi%20there%20%C3%A9.eml",
		"an upper-case blob":    "m/" + mac + "/" + strings.ToUpper(id) + "/Hi%20there%20%C3%A9.eml",
	} {
		if rec := get(t, c, "GET", "/content/"+bad); rec.Code != http.StatusNotFound {
			t.Errorf("%s: GET %s = %d, want 404", name, bad, rec.Code)
		}
	}
}

func TestContentTypes(t *testing.T) {
	for name, want := range map[string]string{
		"a.PNG": "image/png", "a.jpg": "image/jpeg", "a.jpeg": "image/jpeg", "a.gif": "image/gif", "a.webp": "image/webp",
		"a.svg": "image/svg+xml", "a.pdf": "application/pdf", "a.txt": "text/plain; charset=utf-8",
		"a.html": "application/octet-stream", "a.exe": "application/octet-stream", "noext": "application/octet-stream",
	} {
		if got := contentType(name); got != want {
			t.Errorf("contentType(%q) = %q, want %q", name, got, want)
		}
	}
}
