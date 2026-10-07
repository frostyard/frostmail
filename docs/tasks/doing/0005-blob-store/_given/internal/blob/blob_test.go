package blob

// CONTRACT TEST for task card T-0005 (docs/tasks). Do not edit.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func readAll(t *testing.T, s *Store, id string) []byte {
	t.Helper()
	rc, err := s.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// files lists every regular file under dir, relative to it.
func files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, rel)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return out
}

func TestPutOpenRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "blobs")
	s := New(dir)
	msg := []byte("From: a@b.test\r\nSubject: hi\r\n\r\nbody\r\n")
	id, err := s.Put(t.Context(), bytes.NewReader(msg))
	if err != nil {
		t.Fatal(err)
	}
	if id != sha(msg) {
		t.Fatalf("id = %s, want the SHA-256 %s", id, sha(msg))
	}
	if got := readAll(t, s, id); !bytes.Equal(got, msg) {
		t.Fatalf("content = %q", got)
	}
	want := filepath.Join(id[:2], id)
	if got := files(t, dir); len(got) != 1 || got[0] != want {
		t.Fatalf("files = %v, want [%s]", got, want)
	}
	fi, err := os.Stat(filepath.Join(dir, want))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("blob mode = %v, %v; want 0600", fi.Mode().Perm(), err)
	}
	di, err := os.Stat(filepath.Join(dir, id[:2]))
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("shard dir mode = %v, %v; want 0700", di.Mode().Perm(), err)
	}
}

func TestPutIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	big := bytes.Repeat([]byte("0123456789abcdef"), 1<<16) // 1 MiB
	a, err := s.Put(t.Context(), bytes.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Put(t.Context(), bytes.NewReader(big))
	if err != nil || a != b {
		t.Fatalf("second Put = %s, %v; want %s", b, err, a)
	}
	if got := files(t, dir); len(got) != 1 {
		t.Fatalf("files after two identical Puts = %v", got)
	}
	if got := readAll(t, s, a); !bytes.Equal(got, big) {
		t.Fatal("1 MiB blob changed")
	}
}

func TestHasAndRemove(t *testing.T) {
	s := New(t.TempDir())
	id, err := s.Put(t.Context(), strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Has(id); !ok || err != nil {
		t.Fatalf("Has = %v, %v", ok, err)
	}
	if err := s.Remove(id); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Has(id); ok || err != nil {
		t.Fatalf("Has after Remove = %v, %v", ok, err)
	}
	if err := s.Remove(id); err != nil {
		t.Fatalf("second Remove = %v, want nil", err)
	}
	if _, err := s.Open(id); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open after Remove = %v, want fs.ErrNotExist", err)
	}
}

func TestInvalidIDs(t *testing.T) {
	s := New(t.TempDir())
	good := sha([]byte("x"))
	for _, id := range []string{
		"", "../../etc/passwd", strings.ToUpper(good), good[:63], good + "0",
		strings.Repeat("g", 64), good[:2] + "/" + good[3:],
	} {
		if _, err := s.Open(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Open(%q) = %v, want ErrInvalidID", id, err)
		}
		if _, err := s.Has(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Has(%q) = %v, want ErrInvalidID", id, err)
		}
		if err := s.Remove(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Remove(%q) = %v, want ErrInvalidID", id, err)
		}
	}
}

// failingReader returns data, then an error.
type failingReader struct{ n int }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n > 0 {
		f.n--
		return copy(p, "partial"), nil
	}
	return 0, errors.New("network went away")
}

func TestPutFailuresLeaveNothing(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if _, err := s.Put(t.Context(), &failingReader{n: 3}); err == nil || !strings.Contains(err.Error(), "network went away") {
		t.Fatalf("Put with a failing reader = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Put(ctx, strings.NewReader("never stored")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put with a canceled context = %v, want context.Canceled", err)
	}
	if got := files(t, dir); len(got) != 0 {
		t.Fatalf("failed Puts left %v", got)
	}
}
