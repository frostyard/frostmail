package render

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PartsCache is the directory the app reads as mailpart://localhost/
// (<cache>/parts). Paths are relative and use forward slashes.
type PartsCache struct {
	Root string
}

// errBadPath means a relative path tried to leave the cache.
var errBadPath = errors.New("render: path outside the parts cache")

func (c *PartsCache) abs(rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if rel == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", errBadPath, rel)
	}
	return filepath.Join(c.Root, clean), nil
}

// Has reports whether rel exists in the cache.
func (c *PartsCache) Has(rel string) bool {
	p, err := c.abs(rel)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Open opens the cached file at rel.
func (c *PartsCache) Open(rel string) (io.ReadCloser, error) {
	p, err := c.abs(rel)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("parts cache: %w", err)
	}
	return f, nil
}

// Write stores data at rel atomically: a temp file in the same directory,
// then a rename.
func (c *PartsCache) Write(rel string, data []byte) error {
	p, err := c.abs(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("parts cache: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".part-*")
	if err != nil {
		return fmt.Errorf("parts cache: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return fmt.Errorf("parts cache: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return fmt.Errorf("parts cache: %w", err)
	}
	if err := os.Rename(f.Name(), p); err != nil {
		_ = os.Remove(f.Name())
		return fmt.Errorf("parts cache: %w", err)
	}
	return nil
}
