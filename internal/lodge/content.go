// Package lodge serves maild as lodge's mail app on its app socket
// (docs/specs/lodge-app.md, ADR-0041): /health, the /api WebSocket as caller
// session, and /content/ for reader pages, parts and whole messages, which
// lodge-content serves to the reader's frame and to downloads.
package lodge

import (
	"bytes"
	"container/list"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/frostyard/frostmail/internal/render"
)

// Limits on the reader pages kept for the content route; past either, the
// least recently kept page goes.
const (
	MaxPages     = 64
	MaxPageBytes = 64 << 20
)

// The policies maild sends beside lodge's on content responses. A reader
// page may open the tabs its links ask for (the manifest's contentLinks);
// neither runs scripts or loads anything but data: URLs.
const (
	PagePolicy = "default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:; " +
		"base-uri 'none'; form-action 'none'; sandbox allow-popups allow-popups-to-escape-sandbox"
	PartPolicy = "default-src 'none'; img-src data:; style-src 'unsafe-inline'; sandbox"
)

// Blobs opens a stored message by its blob ID (blob.Store).
type Blobs interface {
	Open(id string) (io.ReadCloser, error)
}

// Content is what maild serves under /content/: reader pages kept in
// memory, and files of the parts cache and stored messages named by a MAC
// under a key drawn at start, so content paths are unguessable and end
// with the process.
type Content struct {
	parts *render.PartsCache
	blobs Blobs
	key   []byte

	mu    sync.Mutex
	pages map[string]*list.Element // token → *page in order
	order *list.List               // front: the most recently kept
	size  int
}

type page struct {
	token string
	body  []byte
	kept  time.Time
}

// NewContent serves parts from parts and messages' sources from blobs.
func NewContent(parts *render.PartsCache, blobs Blobs) (*Content, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("content key: %w", err)
	}
	return &Content{parts: parts, blobs: blobs, key: key, pages: map[string]*list.Element{}, order: list.New()}, nil
}

// KeepPage keeps a reader page and returns its content path,
// r/<token>.html, or "" for a page larger than MaxPageBytes.
func (c *Content) KeepPage(body []byte) string {
	if len(body) > MaxPageBytes {
		return ""
	}
	raw := make([]byte, 16)
	_, _ = rand.Read(raw) // crypto/rand.Read never fails
	p := &page{token: hex.EncodeToString(raw), body: body, kept: time.Now()}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pages[p.token] = c.order.PushFront(p)
	c.size += len(body)
	for c.order.Len() > MaxPages || c.size > MaxPageBytes {
		old := c.order.Remove(c.order.Back()).(*page)
		delete(c.pages, old.token)
		c.size -= len(old.body)
	}
	return "r/" + p.token + ".html"
}

func (c *Content) page(token string) (*page, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.pages[token]
	if !ok {
		return nil, false
	}
	return e.Value.(*page), true
}

// PartPath is the content path of rel, a path in the parts cache:
// p/<mac>/<rel with each segment percent-encoded>.
func (c *Content) PartPath(rel string) string {
	segs := strings.Split(rel, "/")
	for i, s := range segs {
		segs[i] = escape(s)
	}
	return "p/" + c.mac(rel) + "/" + strings.Join(segs, "/")
}

// SourcePath is the content path of a stored message, the blob blobID,
// saved as name: m/<mac>/<blobID>/<name percent-encoded>. The MAC covers
// the name too, so a path names one file name.
func (c *Content) SourcePath(blobID, name string) string {
	return "m/" + c.mac(sourceKey(blobID, name)) + "/" + blobID + "/" + escape(name)
}

// sourceKey is what a source path's MAC covers; the NUL keeps it apart
// from every parts cache path, which never holds one.
func sourceKey(blobID, name string) string { return "m\x00" + blobID + "/" + name }

// mac is 22 base64url characters: the first 16 bytes of HMAC-SHA256(rel).
func (c *Content) mac(rel string) string {
	h := hmac.New(sha256.New, c.key)
	h.Write([]byte(rel))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:16])
}

// escape percent-encodes every byte outside RFC 3986's unreserved set.
func escape(s string) string {
	var b strings.Builder
	for i := range len(s) {
		ch := s[i]
		if 'A' <= ch && ch <= 'Z' || 'a' <= ch && ch <= 'z' || '0' <= ch && ch <= '9' || strings.IndexByte("-._~", ch) >= 0 {
			b.WriteByte(ch)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", ch)
	}
	return b.String()
}

// unescape reverses escape for one segment, refusing a malformed escape or
// one that makes a separator.
func unescape(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", false
		}
		v, err := hex.DecodeString(s[i+1 : i+3])
		if err != nil || v[0] == '/' || v[0] == 0 {
			return "", false
		}
		b.WriteByte(v[0])
		i += 2
	}
	return b.String(), true
}

var (
	pagePath   = regexp.MustCompile(`^r/([0-9a-f]{32})\.html$`)
	partPath   = regexp.MustCompile(`^p/([A-Za-z0-9_-]{22})/(.+)$`)
	sourcePath = regexp.MustCompile(`^m/([A-Za-z0-9_-]{22})/([0-9a-f]{64})/([^/]+)$`)
)

// ServeHTTP serves GET and HEAD /content/<content path>. lodge has checked
// the caller; the socket's mux sends only caller content here.
func (c *Content) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		httpError(w, "content is read with GET or HEAD", http.StatusMethodNotAllowed)
		return
	}
	path, ok := strings.CutPrefix(r.URL.EscapedPath(), "/content/")
	if !ok {
		httpError(w, "not found", http.StatusNotFound)
		return
	}
	if m := pagePath.FindStringSubmatch(path); m != nil {
		c.servePage(w, r, m[1])
		return
	}
	if m := partPath.FindStringSubmatch(path); m != nil {
		c.servePart(w, r, m[1], m[2])
		return
	}
	if m := sourcePath.FindStringSubmatch(path); m != nil {
		c.serveSource(w, r, m[1], m[2], m[3])
		return
	}
	httpError(w, "not found", http.StatusNotFound)
}

func (c *Content) servePage(w http.ResponseWriter, r *http.Request, token string) {
	p, ok := c.page(token)
	if !ok {
		httpError(w, "no such page: render the message again", http.StatusNotFound)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", PagePolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-store")
	oneRange(r)
	http.ServeContent(w, r, "", p.kept, bytes.NewReader(p.body))
}

func (c *Content) servePart(w http.ResponseWriter, r *http.Request, mac, escaped string) {
	segs := strings.Split(escaped, "/")
	for i, s := range segs {
		var ok bool
		if segs[i], ok = unescape(s); !ok || segs[i] == "" {
			httpError(w, "not found", http.StatusNotFound)
			return
		}
	}
	rel := strings.Join(segs, "/")
	if !hmac.Equal([]byte(mac), []byte(c.mac(rel))) {
		httpError(w, "not found", http.StatusNotFound)
		return
	}
	f, err := c.parts.File(rel)
	if err != nil {
		httpError(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		httpError(w, "not found", http.StatusNotFound)
		return
	}
	name := segs[len(segs)-1]
	h := w.Header()
	h.Set("Content-Type", contentType(name))
	h.Set("Content-Security-Policy", PartPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-store")
	if r.URL.Query().Get("download") == "1" {
		h.Set("Content-Disposition", "attachment; filename*=UTF-8''"+escape(name))
	}
	oneRange(r)
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

// serveSource serves a stored message as the server holds it: as text to
// read, or with ?download=1 as an .eml file to save (Save As).
func (c *Content) serveSource(w http.ResponseWriter, r *http.Request, mac, blobID, escaped string) {
	name, ok := unescape(escaped)
	if !ok || name == "" || c.blobs == nil || !hmac.Equal([]byte(mac), []byte(c.mac(sourceKey(blobID, name)))) {
		httpError(w, "not found", http.StatusNotFound)
		return
	}
	rc, err := c.blobs.Open(blobID)
	if err != nil {
		httpError(w, "not found", http.StatusNotFound)
		return
	}
	defer rc.Close()
	var modified time.Time
	if st, ok := rc.(interface{ Stat() (fs.FileInfo, error) }); ok {
		if fi, err := st.Stat(); err == nil {
			modified = fi.ModTime()
		}
	}
	body, ok := rc.(io.ReadSeeker)
	if !ok {
		raw, err := io.ReadAll(rc)
		if err != nil {
			httpError(w, "not found", http.StatusNotFound)
			return
		}
		body = bytes.NewReader(raw)
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	if r.URL.Query().Get("download") == "1" {
		h.Set("Content-Type", "message/rfc822")
		h.Set("Content-Disposition", "attachment; filename*=UTF-8''"+escape(name))
	}
	h.Set("Content-Security-Policy", PartPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-store")
	oneRange(r)
	http.ServeContent(w, r, "", modified, body)
}

// oneRange drops a Range header naming several ranges: content answers one
// range per request, or the whole.
func oneRange(r *http.Request) {
	if strings.Contains(r.Header.Get("Range"), ",") {
		r.Header.Del("Range")
	}
}

// contentType is a part's type by its extension, as the app's mailpart://
// scheme has it; anything else is a download.
func contentType(name string) string {
	ext := ""
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		ext = strings.ToLower(name[i+1:])
	}
	switch ext {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "pdf":
		return "application/pdf"
	case "txt":
		return "text/plain; charset=utf-8"
	}
	return "application/octet-stream"
}

// httpError answers one line of text/plain, as the app contract asks.
func httpError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, msg+"\n")
}
