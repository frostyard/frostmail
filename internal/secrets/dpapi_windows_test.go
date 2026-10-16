package secrets

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// The DPAPI store keeps the File's JSON sealed for the user: the bytes on
// disk hold no secret, and a second store at the path reads them back.
func TestDPAPIFileSealsItsSecrets(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "secrets.dpapi")
	s := NewDPAPIFile(path)
	if err := s.Set(ctx, "account/1/refresh-token", "a-refresh-token"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("a-refresh-token")) || bytes.Contains(raw, []byte("account/1")) {
		t.Fatalf("the sealed file holds plain text: %q", raw)
	}
	if got, err := NewDPAPIFile(path).Get(ctx, "account/1/refresh-token"); err != nil || got != "a-refresh-token" {
		t.Fatalf("read back = %q, %v", got, err)
	}
	if _, err := NewFile(path).Get(ctx, "account/1/refresh-token"); err == nil {
		t.Fatal("a plain File read the sealed file")
	}
}

// Open seals on Windows, and moves secrets left in the plain file over.
func TestOpenMovesThePlainFileIntoDPAPI(t *testing.T) {
	ctx := t.Context()
	t.Setenv("FROSTMAIL_SECRETS", "")
	dir := t.TempDir()
	plain := filepath.Join(dir, "secrets.json")
	if err := NewFile(plain).Set(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, plain, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(ctx, "k"); err != nil || got != "v" {
		t.Fatalf("moved secret = %q, %v", got, err)
	}
	if _, err := os.Stat(plain); !os.IsNotExist(err) {
		t.Fatalf("the plain file is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets.dpapi")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FROSTMAIL_SECRETS", "file")
	if s, err := Open(ctx, plain, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	} else if _, ok := s.(*File); !ok || s.(*File).seal != nil {
		t.Fatalf("FROSTMAIL_SECRETS=file gave %T", s)
	}
}
