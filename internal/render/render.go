// Package render prepares stored messages for the reader: sanitized HTML whose
// resources are mailpart://localhost/ URLs into the parts cache, inline
// parts decoded into that cache, and remote images fetched only on request
// (docs/design/rendering.md, ADR-0005).
package render

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/frostyard/frostmail/internal/mimex"
)

// PartURL is the prefix of every URL the app resolves through the parts cache.
const PartURL = "mailpart://localhost/"

// ErrNoPart means a message has no part with the requested path.
var ErrNoPart = errors.New("render: no such part")

// Rendering is a message prepared for the reader (api.Rendering).
type Rendering struct {
	HTML     string
	Text     string
	Remote   int // remote resources left out
	Trackers int // tracking images removed
}

// PartFile is a part decoded into the parts cache (api.PartFile).
type PartFile struct {
	Path        string
	ContentType string
	Filename    string
	Size        int64
}

// Renderer renders messages into its parts cache. Fetcher may be nil, which
// leaves every remote image out.
type Renderer struct {
	Parts   *PartsCache
	Fetcher *Fetcher
}

var dataImage = regexp.MustCompile(`^data:image/(png|gif|jpeg|webp);base64,[A-Za-z0-9+/=\s]+$`)

// Render prepares a raw message. With remote, remote images (not trackers)
// are fetched into the cache; otherwise they are left out and counted.
func (r *Renderer) Render(ctx context.Context, messageID int64, raw []byte, remote bool) (Rendering, error) {
	text, _, err := mimex.BodyText(raw)
	if err != nil {
		return Rendering{}, err
	}
	src, err := mimex.BodyHTML(raw)
	if err != nil {
		return Rendering{}, err
	}
	out := Rendering{Text: text}
	if strings.TrimSpace(src) == "" {
		return out, nil
	}
	inline, err := inlineParts(raw)
	if err != nil {
		return Rendering{}, err
	}
	nonce, err := randomHex(16)
	if err != nil {
		return Rendering{}, err
	}
	var pending []string
	index := map[string]int{}
	resolve := func(raw string, tracker bool) string {
		raw = strings.TrimSpace(raw)
		lower := strings.ToLower(raw)
		switch {
		case strings.HasPrefix(lower, "cid:"):
			return r.inlineURL(messageID, inline, raw[4:])
		case strings.HasPrefix(lower, "data:"):
			if dataImage.MatchString(raw) {
				return raw
			}
			return ""
		case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
			u, err := url.Parse(raw)
			if err != nil {
				return ""
			}
			if tracker || trackerURL(u) {
				out.Trackers++
				return ""
			}
			if !remote || r.Fetcher == nil {
				out.Remote++
				return ""
			}
			i, ok := index[u.String()]
			if !ok {
				i = len(pending)
				index[u.String()] = i
				pending = append(pending, u.String())
			}
			return pendingToken(nonce, i)
		}
		return ""
	}
	html, err := sanitizeHTML(src, resolve)
	if err != nil {
		return Rendering{}, err
	}
	if len(pending) > 0 {
		paths := r.fetchAll(ctx, pending)
		for i, p := range paths {
			target := ""
			if p != "" {
				target = PartURL + p
			} else {
				out.Remote += strings.Count(html, pendingToken(nonce, i))
			}
			html = strings.ReplaceAll(html, pendingToken(nonce, i), target)
		}
	}
	out.HTML = html
	return out, nil
}

// pendingToken stands in for a remote image until it is fetched. The nonce
// is random per render, so message content cannot forge one.
func pendingToken(nonce string, i int) string {
	return "frostmail-pending:" + nonce + "/" + strconv.Itoa(i) + "/"
}

// fetchAll fetches urls with fetchWorkers workers; paths[i] is "" on failure
// and for URLs past MaxImagesPerMail.
func (r *Renderer) fetchAll(ctx context.Context, urls []string) []string {
	paths := make([]string, len(urls))
	work := make(chan int)
	var wg sync.WaitGroup
	for range fetchWorkers {
		wg.Go(func() {
			for i := range work {
				if p, err := r.Fetcher.Fetch(ctx, urls[i]); err == nil {
					paths[i] = p
				}
			}
		})
	}
	for i := range min(len(urls), MaxImagesPerMail) {
		work <- i
	}
	close(work)
	wg.Wait()
	return paths
}

// inlinePart is a part a cid: URL can name.
type inlinePart struct {
	path, contentType string
	data              []byte
}

func inlineParts(raw []byte) (map[string]inlinePart, error) {
	parts := map[string]inlinePart{}
	err := mimex.WalkParts(raw, func(p mimex.PartInfo, body io.Reader) error {
		if p.ContentID == "" || !strings.HasPrefix(p.ContentType, "image/") {
			return nil
		}
		data, err := io.ReadAll(body)
		if err != nil {
			return nil // a truncated image is skipped, not the message
		}
		parts[p.ContentID] = inlinePart{path: p.Path, contentType: p.ContentType, data: data}
		return nil
	})
	return parts, err
}

// inlineURL decodes the part a cid: URL names into the cache and returns its
// mailpart URL, or "" when the message has no such image part.
func (r *Renderer) inlineURL(messageID int64, parts map[string]inlinePart, cid string) string {
	if unescaped, err := url.PathUnescape(cid); err == nil {
		cid = unescaped
	}
	p, ok := parts[strings.Trim(cid, "<>")]
	if !ok {
		return ""
	}
	ext, ok := imageTypes[p.contentType]
	if !ok {
		return ""
	}
	rel := fmt.Sprintf("m/%d/%s.%s", messageID, p.path, ext)
	if !r.Parts.Has(rel) {
		if err := r.Parts.Write(rel, p.data); err != nil {
			return ""
		}
	}
	return PartURL + rel
}

// Part decodes the part at path (an IMAP part specifier) into
// m/<messageID>/<path>/<safe file name>.
func (r *Renderer) Part(messageID int64, raw []byte, path string) (PartFile, error) {
	var out PartFile
	found := false
	err := mimex.WalkParts(raw, func(p mimex.PartInfo, body io.Reader) error {
		if p.Path != path {
			return nil
		}
		data, err := io.ReadAll(body)
		if err != nil {
			return fmt.Errorf("render: read part %s: %w", path, err)
		}
		name := SafeFilename(p.Filename, p.Path, p.ContentType)
		rel := fmt.Sprintf("m/%d/%s/%s", messageID, p.Path, name)
		if err := r.Parts.Write(rel, data); err != nil {
			return err
		}
		out = PartFile{Path: rel, ContentType: p.ContentType, Filename: name, Size: int64(len(data))}
		found = true
		return errStop
	})
	if err != nil && !errors.Is(err, errStop) {
		return PartFile{}, err
	}
	if !found {
		return PartFile{}, fmt.Errorf("%w: %q", ErrNoPart, path)
	}
	return out, nil
}

var errStop = errors.New("stop")

// SafeFilename makes a part's file name safe to create: no path separators,
// control characters or leading dots, at most 200 bytes. An empty result
// becomes part-<path>.<ext>.
func SafeFilename(name, partPath, contentType string) string {
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7f || r == utf8.RuneError {
			return -1
		}
		return r
	}, name)
	name = strings.TrimLeft(strings.TrimSpace(name), ".")
	for len(name) > 200 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	if name == "" {
		ext := imageTypes[contentType]
		if ext == "" {
			ext = "bin"
		}
		name = "part-" + partPath + "." + ext
	}
	return name
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
