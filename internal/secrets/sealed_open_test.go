//go:build !windows

package secrets

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sealedEnv makes a fake credentials directory with a key (nil: none), and
// points a sealed Open at it. It returns the data directory and the plain
// secrets file's path.
func sealedEnv(t *testing.T, key []byte) (dataDir, file string) {
	t.Helper()
	cred := t.TempDir()
	if key != nil {
		if err := os.WriteFile(filepath.Join(cred, CredentialName), key, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CREDENTIALS_DIRECTORY", cred)
	t.Setenv("FROSTMAIL_SECRETS", "sealed")
	dataDir = t.TempDir()
	return dataDir, filepath.Join(dataDir, "secrets.json")
}

// logBuf is a logger that keeps what it is told.
func logBuf() (*slog.Logger, *bytes.Buffer) {
	var b bytes.Buffer
	return slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{Level: slog.LevelDebug})), &b
}

func TestOpenSealedMovesAPlainFileIn(t *testing.T) {
	ctx := t.Context()
	dir, file := sealedEnv(t, testKey(3))
	plain := NewFile(file)
	for k, v := range map[string]string{secretName: secretValue, AccountPassword(1): "pw1"} {
		if err := plain.Set(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	log, logged := logBuf()
	st, err := Open(ctx, file, log, HostOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.(*File); !ok {
		t.Fatalf("store = %T, want *File", st)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the plain file is still there: %v", err)
	}
	sealedFile := filepath.Join(dir, SealedFileName)
	data, err := os.ReadFile(sealedFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("FMSEAL01")) || bytes.Contains(data, []byte(secretValue)) {
		t.Errorf("secrets.sealed is not sealed")
	}
	for k, want := range map[string]string{secretName: secretValue, AccountPassword(1): "pw1"} {
		if got, err := st.Get(ctx, k); err != nil || got != want {
			t.Errorf("Get(%s) = %q, %v; want %q", k, got, err, want)
		}
	}
	if !strings.Contains(logged.String(), "level=INFO") || !strings.Contains(logged.String(), sealedFile) ||
		strings.Contains(logged.String(), secretValue) {
		t.Errorf("the log = %q", logged)
	}

	// A new Set goes to the sealed file; a second start reads it back.
	if err := st.Set(ctx, AccountPassword(2), "pw2"); err != nil {
		t.Fatal(err)
	}
	again, err := Open(ctx, file, log, HostOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := again.Get(ctx, AccountPassword(2)); err != nil || got != "pw2" {
		t.Errorf("after a restart: %q, %v", got, err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a plain file came back: %v", err)
	}
}

func TestOpenSealedFirstStartIsEmpty(t *testing.T) {
	dir, file := sealedEnv(t, testKey(3))
	st, err := Open(t.Context(), file, slog.New(slog.DiscardHandler), HostOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(t.Context(), "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(dir, SealedFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("secrets.sealed exists before a first Set: %v", err)
	}
}

// noFallback checks that Open fails with an error that names the credential
// and the sealed file, and leaves a plain secrets.json byte for byte.
func noFallback(t *testing.T, file string, plainBytes []byte, secrets ...string) {
	t.Helper()
	st, err := Open(t.Context(), file, slog.New(slog.DiscardHandler), HostOptions())
	if err == nil {
		t.Fatalf("Open gave %T and no error", st)
	}
	for _, want := range []string{"frostmail-secrets-key", filepath.Join(filepath.Dir(file), "secrets.sealed")} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q lacks %q", err, want)
		}
	}
	for _, s := range secrets {
		if strings.Contains(err.Error(), s) {
			t.Errorf("the error %q holds a secret", err)
		}
	}
	got, rerr := os.ReadFile(file)
	if rerr != nil || !bytes.Equal(got, plainBytes) {
		t.Errorf("the plain file changed: %q, %v", got, rerr)
	}
}

func TestOpenSealedNeverFallsBack(t *testing.T) {
	ctx := t.Context()
	t.Run("no credentials directory", func(t *testing.T) {
		_, file := sealedEnv(t, testKey(3))
		t.Setenv("CREDENTIALS_DIRECTORY", "")
		if err := NewFile(file).Set(ctx, secretName, secretValue); err != nil {
			t.Fatal(err)
		}
		plain, _ := os.ReadFile(file)
		noFallback(t, file, plain, secretValue, string(testKey(3)))
	})
	t.Run("no key in it", func(t *testing.T) {
		_, file := sealedEnv(t, nil)
		if err := NewFile(file).Set(ctx, secretName, secretValue); err != nil {
			t.Fatal(err)
		}
		plain, _ := os.ReadFile(file)
		noFallback(t, file, plain, secretValue)
	})
	t.Run("a key of the wrong length", func(t *testing.T) {
		_, file := sealedEnv(t, testKey(3)[:31])
		if err := NewFile(file).Set(ctx, secretName, secretValue); err != nil {
			t.Fatal(err)
		}
		plain, _ := os.ReadFile(file)
		noFallback(t, file, plain, secretValue, string(testKey(3)[:31]))
	})
	t.Run("a key that does not open secrets.sealed", func(t *testing.T) {
		dir, file := sealedEnv(t, testKey(4))
		other, err := NewSealedFile(filepath.Join(dir, SealedFileName), testKey(3))
		if err != nil {
			t.Fatal(err)
		}
		if err := other.Set(ctx, "old", "old-secret"); err != nil {
			t.Fatal(err)
		}
		// A plain leftover too: it must not move into a file nothing opens.
		if err := NewFile(file).Set(ctx, secretName, secretValue); err != nil {
			t.Fatal(err)
		}
		plain, _ := os.ReadFile(file)
		sealedBefore, _ := os.ReadFile(filepath.Join(dir, SealedFileName))
		noFallback(t, file, plain, secretValue, "old-secret", string(testKey(3)), string(testKey(4)))
		if after, _ := os.ReadFile(filepath.Join(dir, SealedFileName)); !bytes.Equal(after, sealedBefore) {
			t.Error("secrets.sealed changed")
		}
	})
}

func TestOpenRefusesAnUnknownValue(t *testing.T) {
	t.Setenv("FROSTMAIL_SECRETS", "keyring")
	file := filepath.Join(t.TempDir(), "secrets.json")
	_, err := Open(t.Context(), file, slog.New(slog.DiscardHandler), HostOptions())
	if err == nil || !strings.Contains(err.Error(), `"keyring"`) || !strings.Contains(err.Error(), "file, sealed") {
		t.Errorf("Open = %v", err)
	}
	if _, err := OpenReader(t.Context(), file, HostOptions()); err == nil {
		t.Error("OpenReader took the unknown value")
	}
}

func TestOpenFileWarnsOnce(t *testing.T) {
	t.Setenv("FROSTMAIL_SECRETS", "file")
	file := filepath.Join(t.TempDir(), "secrets.json")
	log, logged := logBuf()
	st, err := Open(t.Context(), file, log, HostOptions())
	if _, ok := st.(*File); err != nil || !ok {
		t.Fatalf("Open = %T, %v", st, err)
	}
	if n := strings.Count(logged.String(), "level=WARN"); n != 1 || !strings.Contains(logged.String(), file) ||
		!strings.Contains(logged.String(), "FROSTMAIL_SECRETS=sealed") {
		t.Errorf("the log = %q", logged)
	}
}

func TestOpenReaderSealed(t *testing.T) {
	ctx := t.Context()
	dir, file := sealedEnv(t, testKey(3))
	sealed, err := NewSealedFile(filepath.Join(dir, SealedFileName), testKey(3))
	if err != nil {
		t.Fatal(err)
	}
	if err := sealed.Set(ctx, "a", "from-sealed"); err != nil {
		t.Fatal(err)
	}
	if err := sealed.Set(ctx, "b", "sealed-b"); err != nil {
		t.Fatal(err)
	}
	// A plain leftover: it answers only for a key the sealed file lacks.
	leftover := NewFile(file)
	if err := leftover.Set(ctx, "b", "plain-b"); err != nil {
		t.Fatal(err)
	}
	if err := leftover.Set(ctx, "c", "from-plain"); err != nil {
		t.Fatal(err)
	}
	plainBefore, _ := os.ReadFile(file)
	sealedBefore, _ := os.ReadFile(filepath.Join(dir, SealedFileName))

	r, err := OpenReader(ctx, file, HostOptions())
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"a": "from-sealed", "b": "sealed-b", "c": "from-plain"} {
		if got, err := r.Get(ctx, k); err != nil || got != want {
			t.Errorf("Get(%s) = %q, %v; want %q", k, got, err, want)
		}
	}
	if _, err := r.Get(ctx, "d"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get of a missing key = %v", err)
	}
	if err := r.Set(ctx, "x", "y"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Set = %v", err)
	}
	if err := r.Delete(ctx, "a"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Delete = %v", err)
	}
	if after, _ := os.ReadFile(file); !bytes.Equal(after, plainBefore) {
		t.Error("the plain file changed")
	}
	if after, _ := os.ReadFile(filepath.Join(dir, SealedFileName)); !bytes.Equal(after, sealedBefore) {
		t.Error("the sealed file changed")
	}

	// Without a key, or with one that does not open the file: an error.
	t.Setenv("CREDENTIALS_DIRECTORY", "")
	if _, err := OpenReader(ctx, file, HostOptions()); err == nil || !strings.Contains(err.Error(), "frostmail-secrets-key") {
		t.Errorf("OpenReader without a key = %v", err)
	}
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, CredentialName), testKey(8), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CREDENTIALS_DIRECTORY", other)
	if _, err := OpenReader(ctx, file, HostOptions()); err == nil || !strings.Contains(err.Error(), SealedFileName) {
		t.Errorf("OpenReader with a wrong key = %v", err)
	}
}

func TestOpenReaderSealedFallsBackToTheLeftoverWithNoSealedFile(t *testing.T) {
	ctx := t.Context()
	_, file := sealedEnv(t, testKey(3))
	if err := NewFile(file).Set(ctx, "c", "from-plain"); err != nil {
		t.Fatal(err)
	}
	r, err := OpenReader(ctx, file, HostOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := r.Get(ctx, "c"); err != nil || got != "from-plain" {
		t.Errorf("Get = %q, %v", got, err)
	}
	if _, err := os.Stat(strings.TrimSuffix(file, ".json") + ".sealed"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the reader made a sealed file: %v", err)
	}
}

func TestOpenSealedRemovesAnEmptyPlainFile(t *testing.T) {
	dir, file := sealedEnv(t, testKey(3))
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), file, slog.New(slog.DiscardHandler), HostOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an empty plain file is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, SealedFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an empty plain store made a sealed file: %v", err)
	}
}
