// Package blob stores raw messages by content: each blob is a file named by
// the hex SHA-256 of its bytes, under <dir>/<first two hex digits>/<sha>
// (docs/design/storage.md).
package blob

import (
	"context"
	"errors"
	"io"
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

// errNotImplemented marks the stubs task T-0005 replaces.
var errNotImplemented = errors.New("blob: not implemented")

// Put copies r into the store and returns the blob's ID. Content already
// stored is not written twice. The blob appears atomically: r is streamed to
// a temporary file in the store directory while hashing, synced, and renamed
// into place. Directories are 0700 and blobs 0600. If ctx is done before the
// rename, Put returns ctx.Err() and leaves nothing behind.
func (s *Store) Put(ctx context.Context, r io.Reader) (string, error) {
	return "", errNotImplemented
}

// Open returns the blob's content. A missing blob returns an error matching
// fs.ErrNotExist; a malformed id returns ErrInvalidID.
func (s *Store) Open(id string) (io.ReadCloser, error) {
	return nil, errNotImplemented
}

// Has reports whether the blob exists; a malformed id returns ErrInvalidID.
func (s *Store) Has(id string) (bool, error) {
	return false, errNotImplemented
}

// Remove deletes the blob. A missing blob is not an error; a malformed id
// returns ErrInvalidID.
func (s *Store) Remove(id string) error {
	return errNotImplemented
}
