// Package blob stores raw messages by content: each blob is a file named by
// the hex SHA-256 of its bytes, under <dir>/<first two hex digits>/<sha>
// (docs/design/storage.md).
package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrInvalidID rejects anything but 64 lowercase hex digits, so an ID can
// never name a path outside the store.
var ErrInvalidID = errors.New("blob: invalid id")

// Store is a directory of immutable blobs. It is safe for concurrent use.
type Store struct {
	dir string
}

// New returns a Store rooted at dir, which is created (0700) on first Put.
func New(dir string) *Store { return &Store{dir: dir} }

// path returns the file a valid id names, or ErrInvalidID if id is not
// exactly 64 lowercase hex digits.
func (s *Store) path(id string) (string, error) {
	if len(id) != 64 {
		return "", ErrInvalidID
	}
	for i := range len(id) {
		if c := id[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", ErrInvalidID
		}
	}
	return filepath.Join(s.dir, id[:2], id), nil
}

// Put copies r into the store and returns the blob's ID. Content already
// stored is not written twice. The blob appears atomically: r is streamed to
// a temporary file in the store directory while hashing, synced, and renamed
// into place. Directories are 0700 and blobs 0600. If ctx is done before the
// rename, Put returns ctx.Err() and leaves nothing behind.
func (s *Store) Put(ctx context.Context, r io.Reader) (string, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("blob: create store dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, ".put-*")
	if err != nil {
		return "", fmt.Errorf("blob: create temp: %w", err)
	}
	name := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(name)
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), r); err != nil {
		return "", fmt.Errorf("blob: read content: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := tmp.Chmod(0o600); err != nil {
		return "", fmt.Errorf("blob: chmod temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return "", fmt.Errorf("blob: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("blob: close temp: %w", err)
	}

	id := hex.EncodeToString(h.Sum(nil))
	shard := filepath.Join(s.dir, id[:2])
	if err := os.MkdirAll(shard, 0o700); err != nil {
		return "", fmt.Errorf("blob: create shard dir: %w", err)
	}
	final := filepath.Join(shard, id)
	if _, err := os.Stat(final); err == nil {
		return id, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("blob: stat blob: %w", err)
	}
	if err := os.Rename(name, final); err != nil {
		return "", fmt.Errorf("blob: rename into place: %w", err)
	}
	committed = true
	return id, nil
}

// Open returns the blob's content. A missing blob returns an error matching
// fs.ErrNotExist; a malformed id returns ErrInvalidID.
func (s *Store) Open(id string) (io.ReadCloser, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("blob: open %s: %w", id, err)
	}
	return f, nil
}

// Has reports whether the blob exists; a malformed id returns ErrInvalidID.
func (s *Store) Has(id string) (bool, error) {
	p, err := s.path(id)
	if err != nil {
		return false, err
	}
	switch _, err := os.Stat(p); {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("blob: stat %s: %w", id, err)
	}
}

// Remove deletes the blob. A missing blob is not an error; a malformed id
// returns ErrInvalidID.
func (s *Store) Remove(id string) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("blob: remove %s: %w", id, err)
	}
	return nil
}
