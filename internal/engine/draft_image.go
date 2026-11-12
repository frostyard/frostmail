package engine

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"golang.org/x/net/html"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/render"
)

// Image names an image already cached for a saved draft; it never loads a URL.
func (d drafts) Image(ctx context.Context, p *api.DraftImageParams) (*api.PartFile, error) {
	draft, err := d.Get(ctx, &api.DraftGetParams{ID: p.ID})
	if err != nil {
		return nil, err
	}
	if !fs.ValidPath(p.Path) || strings.ContainsAny(p.Path, "\\\x00") || p.Path == "." {
		return nil, api.InvalidParams("invalid cached image path")
	}
	if !draftHasImage(draft.Content.HTML, "mailpart://localhost/"+p.Path) {
		return nil, api.NotFound("draft has no such image")
	}
	cache := d.parts()
	if cache == nil {
		return nil, api.Unavailable("parts cache is not running")
	}
	root, err := os.OpenRoot(cache.Root)
	if err != nil {
		return nil, api.NotFound("cached image is unavailable")
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(p.Path)
	if err != nil {
		return nil, api.NotFound("cached image is unavailable")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > render.MaxImageBytes {
		return nil, api.NotFound("cached image is unavailable")
	}
	var header [512]byte
	n, err := io.ReadFull(file, header[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	kind := http.DetectContentType(header[:n])
	if !strings.HasPrefix(kind, "image/") {
		return nil, api.NotFound("cached file is not an image")
	}
	out := &api.PartFile{Path: p.Path, ContentType: kind, Filename: path.Base(p.Path), Size: info.Size()}
	if d.Content != nil {
		contentPath := d.Content.PartPath(p.Path)
		out.ContentPath = &contentPath
	}
	return out, nil
}

func draftHasImage(body, src string) bool {
	z := html.NewTokenizer(strings.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken, html.SelfClosingTagToken:
			token := z.Token()
			if token.Data != "img" {
				continue
			}
			for _, attr := range token.Attr {
				if attr.Key == "src" {
					if attr.Val == src {
						return true
					}
					break
				}
			}
		}
	}
}
