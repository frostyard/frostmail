package render

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Limits on fetching remote images (docs/design/rendering.md).
const (
	MaxImageBytes    = 5 << 20
	MaxImagesPerMail = 32
	fetchTimeout     = 10 * time.Second
	fetchWorkers     = 4
	maxRedirects     = 3
)

// ErrRefusedAddress means a remote image resolved to an address that is not
// on the public internet.
var ErrRefusedAddress = errors.New("render: refused non-public address")

// imageTypes are the image types the fetcher keeps, by sniffed type.
var imageTypes = map[string]string{
	"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp",
	"image/bmp": "bmp", "image/x-icon": "ico", "image/avif": "avif",
}

// extraBlocked are non-public ranges netip's predicates do not cover.
var extraBlocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// PublicAddr reports whether a is a public unicast address, the only kind the
// fetcher connects to.
func PublicAddr(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsMulticast() || a.IsUnspecified() || a.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range extraBlocked {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// Fetcher downloads remote images into the parts cache for a message whose
// remote content the user allowed. It sends no cookies or Referer, and its
// dialer refuses every address allow rejects, so a message cannot reach the
// local network, also through DNS or redirects.
type Fetcher struct {
	client *http.Client
	cache  *PartsCache
}

// NewFetcher returns a Fetcher; allow nil means PublicAddr.
func NewFetcher(cache *PartsCache, allow func(netip.Addr) bool) *Fetcher {
	if allow == nil {
		allow = PublicAddr
	}
	dialer := &net.Dialer{
		Timeout: fetchTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || !allow(ip) {
				return fmt.Errorf("%w: %s", ErrRefusedAddress, host)
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   fetchTimeout,
		ResponseHeaderTimeout: fetchTimeout,
		MaxIdleConnsPerHost:   2,
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return errors.New("render: too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("render: redirect to a non-HTTP URL")
			}
			req.Header.Del("Referer")
			return nil
		},
	}
	return &Fetcher{client: client, cache: cache}
}

// Fetch downloads an image URL into the cache, or finds it there, and returns
// its relative path.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("render: not an http(s) URL: %q", rawURL)
	}
	sum := sha256.Sum256([]byte(u.String()))
	base := "r/" + hex.EncodeToString(sum[:16])
	for _, ext := range imageTypes {
		if f.cache.Has(base + "." + ext) {
			return base + "." + ext, nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "image/*")
	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("render: fetch %s: %s", u.Host, resp.Status)
	}
	declared, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(declared, "image/") {
		return "", fmt.Errorf("render: fetch %s: not an image (%q)", u.Host, declared)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxImageBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > MaxImageBytes {
		return "", fmt.Errorf("render: fetch %s: image larger than %d bytes", u.Host, MaxImageBytes)
	}
	sniffed, _, _ := mime.ParseMediaType(http.DetectContentType(data))
	ext, ok := imageTypes[sniffed]
	if !ok {
		return "", fmt.Errorf("render: fetch %s: content is %q", u.Host, sniffed)
	}
	rel := base + "." + ext
	if err := f.cache.Write(rel, data); err != nil {
		return "", err
	}
	return rel, nil
}
