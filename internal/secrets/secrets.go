// Package secrets keeps account credentials out of the database
// (docs/design/storage.md#credentials). M1 uses File; Secret Service over
// D-Bus replaces it in M4 behind the same interface.
package secrets

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
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

// RefreshToken is the key of an oauth2 account's refresh token.
func RefreshToken(accountID int64) string { return fmt.Sprintf("account/%d/refresh-token", accountID) }

// OAuthClientSecret is the key of a provider's OAuth client secret.
func OAuthClientSecret(provider string) string { return "oauth/" + provider + "/client-secret" }

// Credential is the key of the secret an account signs in with.
func Credential(accountID int64, oauth bool) string {
	if oauth {
		return RefreshToken(accountID)
	}
	return AccountPassword(accountID)
}

// File is a Store kept in one JSON object ({"key": "value", ...}) in a file
// with mode 0600. It is the M1 development store: plain text on disk, guarded
// only by file permissions.
type File struct {
	mu   sync.Mutex
	path string
}

var _ Store = (*File)(nil)

// NewFile returns a File store at path. The file is created on first Set;
// its directory must exist.
func NewFile(path string) *File { return &File{path: path} }

// Get returns the secret stored under key, or ErrNotFound. A missing file
// holds no secrets. A file whose mode allows group or other access is refused
// with an error naming the file and its mode.
func (f *File) Get(ctx context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	m, err := f.read()
	if err != nil {
		return "", err
	}
	value, ok := m[key]
	if !ok {
		return "", fmt.Errorf("secrets: get %s: %w", key, ErrNotFound)
	}
	return value, nil
}

// Set stores value under key. It rewrites the whole file atomically: write a
// temporary file in the same directory with mode 0600, then rename it over
// the old file.
func (f *File) Set(ctx context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	m, err := f.read()
	if err != nil {
		return err
	}
	m[key] = value
	return f.write(m)
}

// Delete removes key, rewriting the file atomically like Set. A missing key
// or a missing file is not an error.
func (f *File) Delete(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	m, err := f.read()
	if err != nil {
		return err
	}
	if _, ok := m[key]; !ok {
		return nil
	}
	delete(m, key)
	return f.write(m)
}

// read loads the whole map. A missing file is an empty map; a file readable
// by group or other, or one that is not a JSON object of strings, is an
// error.
func (f *File) read() (map[string]string, error) {
	fi, err := os.Stat(f.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("secrets: stat %s: %w", f.path, err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("secrets: %s has mode 0%o, want 0600", f.path, fi.Mode().Perm())
	}
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("secrets: read %s: %w", f.path, err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("secrets: decode %s: %w", f.path, err)
	}
	if m == nil {
		m = map[string]string{}
	}
	return m, nil
}

// write replaces the file with data atomically: a temporary file in the
// target's directory, mode 0600, synced, then renamed over the target. The
// temporary file is removed on any error.
func (f *File) write(m map[string]string) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("secrets: encode %s: %w", f.path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".secrets-*")
	if err != nil {
		return fmt.Errorf("secrets: create temp in %s: %w", filepath.Dir(f.path), err)
	}
	if err := syncTemp(tmp, data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("secrets: close %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("secrets: rename %s over %s: %w", tmp.Name(), f.path, err)
	}
	return nil
}

// syncTemp writes data to tmp and flushes it with mode 0600.
func syncTemp(tmp *os.File, data []byte) error {
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("secrets: write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("secrets: chmod %s: %w", tmp.Name(), err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("secrets: sync %s: %w", tmp.Name(), err)
	}
	return nil
}
