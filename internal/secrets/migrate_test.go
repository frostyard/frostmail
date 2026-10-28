package secrets

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// failSecondSet is a store whose second Set fails, as a keyring that stops
// answering in the middle of a move.
type failSecondSet struct {
	Store
	mu   sync.Mutex
	sets int
}

var errSecondSet = errors.New("the keyring stopped answering")

func (s *failSecondSet) Set(ctx context.Context, key, value string) error {
	s.mu.Lock()
	s.sets++
	fail := s.sets == 2
	s.mu.Unlock()
	if fail {
		return errSecondSet
	}
	return s.Store.Set(ctx, key, value)
}

// A move that fails on its second key leaves the file as it is and the first
// key in the destination, and a second move into a working store moves every
// key and removes the file.
func TestMigrateFailureKeepsTheFileAndTheNextMoveFinishes(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	file := filepath.Join(dir, "secrets.json")
	from := NewFile(file)
	want := map[string]string{AccountPassword(1): "pw1", RefreshToken(2): "rt2", OAuthClientSecret("google"): "cs3"}
	for k, v := range want {
		if err := from.Set(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	dest := NewFile(filepath.Join(dir, "dest.json"))
	n, err := migrate(ctx, from, &failSecondSet{Store: dest})
	if !errors.Is(err, errSecondSet) || n != 0 {
		t.Fatalf("migrate = %d, %v; want 0 and the Set error", n, err)
	}
	for _, v := range want {
		if strings.Contains(err.Error(), v) {
			t.Errorf("the error holds a secret value: %v", err)
		}
	}
	if after, err := os.ReadFile(file); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("the file changed after a failed move: %v", err)
	}
	if got, err := dest.All(); err != nil || len(got) != 1 {
		t.Fatalf("the destination holds %v, %v; want the 1 key copied before the failure", got, err)
	}

	n, err = migrate(ctx, from, dest)
	if err != nil || n != len(want) {
		t.Fatalf("second migrate = %d, %v; want %d", n, err, len(want))
	}
	got, err := dest.All()
	if err != nil || len(got) != len(want) {
		t.Fatalf("the destination holds %v, %v", got, err)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the file is still there: %v", err)
	}
}

// readBack is a store that fails or changes what it reads back.
type readBack struct {
	Store
	err   error
	other bool
}

func (r readBack) Get(ctx context.Context, key string) (string, error) {
	v, err := r.Store.Get(ctx, key)
	switch {
	case r.err != nil:
		return "", r.err
	case r.other:
		return v + "x", err
	}
	return v, err
}

// A read-back failure wraps its cause with %w; a mismatch is its own error;
// neither holds a value.
func TestMigrateReadBackErrors(t *testing.T) {
	ctx := t.Context()
	errGet := errors.New("get failed")
	for name, tc := range map[string]struct {
		store readBack
		is    error
		text  string
	}{
		"a Get error":   {readBack{err: errGet}, errGet, "did not read back"},
		"another value": {readBack{other: true}, nil, "read back another value"},
	} {
		dir := t.TempDir()
		file := filepath.Join(dir, "secrets.json")
		from := NewFile(file)
		if err := from.Set(ctx, AccountPassword(1), "very-secret-value"); err != nil {
			t.Fatal(err)
		}
		tc.store.Store = NewFile(filepath.Join(dir, "dest.json"))
		_, err := migrate(ctx, from, tc.store)
		if err == nil || !strings.Contains(err.Error(), tc.text) || strings.Contains(err.Error(), "very-secret-value") {
			t.Errorf("%s: migrate = %v", name, err)
		}
		if tc.is != nil && !errors.Is(err, tc.is) {
			t.Errorf("%s: the error does not wrap its cause: %v", name, err)
		}
		if _, err := os.Stat(file); err != nil {
			t.Errorf("%s: the file is gone: %v", name, err)
		}
	}
}

// Probes that run at once, on one store, all succeed and leave nothing.
func TestConcurrentProbesOnAFile(t *testing.T) {
	st := NewFile(filepath.Join(t.TempDir(), "secrets.json"))
	runProbes(t, st, func() map[string]string {
		all, err := st.All()
		if err != nil {
			t.Fatal(err)
		}
		return all
	})
}

// runProbes runs 8 probes at once on st and checks that each succeeds and
// that left() is empty afterwards.
func runProbes(t *testing.T, st Store, left func() map[string]string) {
	t.Helper()
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() { errs[i] = Probe(t.Context(), st) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("probe %d = %v", i, err)
		}
	}
	if got := left(); len(got) != 0 {
		t.Errorf("the store holds %d keys after the probes; want none", len(got))
	}
}
