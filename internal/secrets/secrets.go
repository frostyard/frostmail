// Package secrets keeps account credentials out of the database
// (docs/design/storage.md#credentials). M1 uses File; Secret Service over
// D-Bus replaces it in M4 behind the same interface.
package secrets

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotFound means no secret is stored under the key.
var ErrNotFound = errors.New("secrets: not found")

// Store keeps string secrets by key.
type Store interface {
	// Get returns the secret, or ErrNotFound.
	Get(ctx context.Context, key string) (string, error)
	// Set stores the secret, replacing any previous value.
	Set(ctx context.Context, key, value string) error
	// Delete removes the secret; a missing key is not an error.
	Delete(ctx context.Context, key string) error
}

// AccountPassword is the key of an account's password.
func AccountPassword(accountID int64) string { return fmt.Sprintf("account/%d/password", accountID) }

// File is a Store kept in one JSON object ({"key": "value", ...}) in a file
// with mode 0600. It is the M1 development store: plain text on disk, guarded
// only by file permissions.
type File struct {
	path string
}

var _ Store = (*File)(nil)

// NewFile returns a File store at path. The file is created on first Set;
// its directory must exist.
func NewFile(path string) *File { return &File{path: path} }

// errNotImplemented marks the stubs task T-0004 replaces.
var errNotImplemented = errors.New("secrets: not implemented")

// Get returns the secret stored under key, or ErrNotFound. A missing file
// holds no secrets. A file whose mode allows group or other access is refused
// with an error naming the file and its mode.
func (f *File) Get(ctx context.Context, key string) (string, error) {
	return "", errNotImplemented
}

// Set stores value under key. It rewrites the whole file atomically: write a
// temporary file in the same directory with mode 0600, then rename it over
// the old file.
func (f *File) Set(ctx context.Context, key, value string) error {
	return errNotImplemented
}

// Delete removes key, rewriting the file atomically like Set. A missing key
// or a missing file is not an error.
func (f *File) Delete(ctx context.Context, key string) error {
	return errNotImplemented
}
