package secrets

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// testKey is a 32-byte key, filled with b.
func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

// A value and a key name the files below must never show in plain text.
const (
	secretValue = "hunter2-very-secret"
	secretName  = "account/7/refresh-token"
)

func newSealed(t *testing.T, key []byte) (*File, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secrets.sealed")
	f, err := NewSealedFile(path, key)
	if err != nil {
		t.Fatal(err)
	}
	return f, path
}

func TestSealedRoundTrip(t *testing.T) {
	ctx := t.Context()
	f, path := newSealed(t, testKey(1))
	if err := f.Set(ctx, AccountPassword(1), "one"); err != nil {
		t.Fatal(err)
	}
	if err := f.Set(ctx, AccountPassword(2), "two"); err != nil {
		t.Fatal(err)
	}
	if err := f.Delete(ctx, AccountPassword(1)); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewSealedFile(path, testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.Get(ctx, AccountPassword(2)); err != nil || got != "two" {
		t.Fatalf("Get = %q, %v; want two", got, err)
	}
	if _, err := reopened.Get(ctx, AccountPassword(1)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get of a deleted key = %v, want ErrNotFound", err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, %v; want 0600", fi, err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("the directory holds %v, %v; want only the sealed file", entries, err)
	}
}

func TestSealedMissingFileIsEmpty(t *testing.T) {
	f, _ := newSealed(t, testKey(1))
	if _, err := f.Get(t.Context(), "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get = %v, want ErrNotFound", err)
	}
}

func TestSealedFileHoldsNoPlaintext(t *testing.T) {
	f, path := newSealed(t, testKey(1))
	if err := f.Set(t.Context(), secretName, secretValue); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("FMSEAL01")) {
		t.Errorf("the file starts with %q, not the magic", data[:8])
	}
	for _, s := range []string{secretValue, secretName, "refresh-token", "account"} {
		if bytes.Contains(data, []byte(s)) {
			t.Errorf("the sealed file holds %q in plain text", s)
		}
	}
	if bytes.Contains(data, testKey(1)) {
		t.Error("the sealed file holds the key")
	}
}

func TestSealedUsesAFreshNonce(t *testing.T) {
	f, path := newSealed(t, testKey(1))
	if err := f.Set(t.Context(), "k", "v"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The same map again: Set rewrites the file.
	if err := f.Set(t.Context(), "k", "v"); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("2 writes of the same map gave the same bytes: the nonce is not fresh")
	}
	if bytes.Equal(first[8:20], second[8:20]) {
		t.Fatal("the nonce repeated")
	}
}

func TestSealedRefusals(t *testing.T) {
	ctx := t.Context()
	write := func(t *testing.T) (string, []byte) {
		t.Helper()
		f, path := newSealed(t, testKey(1))
		if err := f.Set(ctx, secretName, secretValue); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return path, data
	}
	for name, tc := range map[string]struct {
		key    byte
		mutate func(data []byte) []byte
	}{
		"a wrong key":    {2, func(d []byte) []byte { return d }},
		"a tampered tag": {1, func(d []byte) []byte { d[len(d)-1] ^= 1; return d }},
		"a tampered body": {1, func(d []byte) []byte {
			d[len(d)/2+5] ^= 1
			return d
		}},
		"a tampered nonce": {1, func(d []byte) []byte { d[9] ^= 1; return d }},
		"a truncated file": {1, func(d []byte) []byte { return d[:20] }},
		"half a file":      {1, func(d []byte) []byte { return d[:len(d)-3] }},
		"an empty file":    {1, func([]byte) []byte { return nil }},
		"a wrong magic":    {1, func(d []byte) []byte { copy(d, "FMSEAL02"); return d }},
	} {
		t.Run(name, func(t *testing.T) {
			path, data := write(t)
			if err := os.WriteFile(path, tc.mutate(data), 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := NewSealedFile(path, testKey(tc.key))
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.Get(ctx, secretName)
			if err == nil || errors.Is(err, ErrNotFound) {
				t.Fatalf("Get = %v, want a refusal", err)
			}
			if _, err := f.All(); err == nil {
				t.Fatal("All opened the file")
			}
			if err := f.Set(ctx, "other", "x"); err == nil {
				t.Fatal("Set rewrote a file it cannot open")
			}
			for _, s := range []string{secretValue, secretName, string(testKey(tc.key)), string(testKey(1))} {
				if strings.Contains(err.Error(), s) {
					t.Errorf("the error %q holds a secret", err)
				}
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("the error %q does not name the file", err)
			}
		})
	}
}

func TestNewSealedFileWantsA32ByteKey(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33, 64} {
		if _, err := NewSealedFile("x", make([]byte, n)); err == nil {
			t.Errorf("a key of %d bytes was taken", n)
		}
	}
}

func TestLoadKey(t *testing.T) {
	_, err := LoadKey("")
	if err == nil || !strings.Contains(err.Error(), "$CREDENTIALS_DIRECTORY is not set") ||
		!strings.Contains(err.Error(), "LoadCredentialEncrypted=frostmail-secrets-key") {
		t.Errorf("an empty directory: %v", err)
	}

	dir := t.TempDir()
	_, err = LoadKey(dir)
	if err == nil || !strings.Contains(err.Error(), "frostmail-secrets-key") || !strings.Contains(err.Error(), filepath.Join(dir, "frostmail-secrets-key")) {
		t.Errorf("a missing file: %v", err)
	}

	for _, n := range []int{31, 33, 0} {
		bad := bytes.Repeat([]byte("K"), n)
		if err := os.WriteFile(filepath.Join(dir, "frostmail-secrets-key"), bad, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadKey(dir)
		if err == nil || !strings.Contains(err.Error(), "is "+strconv.Itoa(n)+" bytes") || (n > 0 && strings.Contains(err.Error(), string(bad))) {
			t.Errorf("a key of %d bytes: %v", n, err)
		}
	}

	want := testKey(9)
	if err := os.WriteFile(filepath.Join(dir, "frostmail-secrets-key"), want, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadKey(dir); err != nil || !bytes.Equal(got, want) {
		t.Errorf("LoadKey = %x, %v", got, err)
	}
}
