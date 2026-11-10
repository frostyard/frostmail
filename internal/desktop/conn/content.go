package conn

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MaxDownload is the most an attachment or a message downloaded from
// lodge's content node may be.
const MaxDownload = 128 << 20

// downloadClient follows no redirect, and gives up after two minutes.
var downloadClient = &http.Client{
	Timeout:       120 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// ContentOrigin asks lodge for its content node, and derives its usual
// name when lodge cannot say.
func (l Lodge) ContentOrigin(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.origin+"/lodge/apps", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Origin", l.origin)
	asked, err := func() (string, error) {
		resp, err := downloadClient.Do(req)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("lodge answered %s", resp.Status)
		}
		var apps struct {
			ContentOrigin string `json:"contentOrigin"`
		}
		if err := json.UnmarshalRead(resp.Body, &apps); err != nil {
			return "", err
		}
		if apps.ContentOrigin == "" {
			return "", errNoContentOrigin
		}
		parsed, err := ParseLodge(apps.ContentOrigin)
		if err != nil {
			return "", err
		}
		return parsed.Origin(), nil
	}()
	switch {
	case err == nil:
		return asked, nil
	case errors.Is(err, errNoContentOrigin):
		return "", err
	}
	slog.Warn("lodge's /lodge/apps; deriving the content origin", "err", err)
	return l.DerivedContentOrigin()
}

var errNoContentOrigin = errors.New("lodge serves no content origin for mail")

// fetch reads <contentOrigin>/mail/<contentPath>, at most MaxDownload
// bytes of it.
func (l Lodge) fetch(ctx context.Context, contentPath, what string) ([]byte, error) {
	if strings.HasPrefix(contentPath, "/") || strings.Contains(contentPath, "..") || strings.ContainsAny(contentPath, "?#") {
		return nil, fmt.Errorf("not a content path: %s", contentPath)
	}
	origin, err := l.ContentOrigin(ctx)
	if err != nil {
		return nil, err
	}
	url := origin + "/mail/" + contentPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxDownload+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if len(data) > MaxDownload {
		return nil, fmt.Errorf("%s is over %d MiB", what, MaxDownload>>20)
	}
	return data, nil
}

// Download fetches a part from lodge's content node into dir, under a
// name that stays in it, and returns the file's path.
func (l Lodge) Download(ctx context.Context, dir, contentPath, filename string) (string, error) {
	data, err := l.fetch(ctx, contentPath, filename)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, SafeName(filename))
	return path, writeNew(path, data)
}

// SaveTo fetches a message maild named on the content node into path, the
// file the user chose, replacing it.
func (l Lodge) SaveTo(ctx context.Context, contentPath, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("not an absolute path: %s", path)
	}
	data, err := l.fetch(ctx, contentPath, filepath.Base(path))
	if err != nil {
		return err
	}
	return writeNew(path, data)
}

// SafeName is a file name that stays in its directory: no separators, no
// leading dot, no control characters.
func SafeName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f || r == '/' || r == '\\' || r == ':' {
			return '_'
		}
		return r
	}, name)
	trimmed := strings.TrimLeft(strings.TrimSpace(cleaned), ".")
	if trimmed == "" {
		return "attachment"
	}
	return trimmed
}

func writeNew(path string, data []byte) error {
	tmp := path + ".download"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
