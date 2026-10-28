//go:build !windows

package main

// This test needs a real data lock, which internal/datalock makes a no-op on
// Windows.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/datalock"
)

// A maild that holds the lock without answering on the socket keeps the
// import out: it fails within its wait, and changes nothing. Once the lock
// goes, the same import goes through.
func TestImportRefusesWhileTheLockIsHeld(t *testing.T) {
	f := newImportFixture(t)
	if err := os.MkdirAll(f.data, 0o700); err != nil {
		t.Fatal(err)
	}
	release, err := datalock.Acquire(f.data) // stands for a maild that is starting
	if err != nil {
		t.Fatal(err)
	}
	held := true
	defer func() {
		if held {
			release()
		}
	}()

	var out bytes.Buffer
	err = runImport(t.Context(), &out, &rootOptions{}, f.export.dir, 300*time.Millisecond, nil)
	if err == nil || !strings.Contains(err.Error(), "is still locked") {
		t.Fatalf("import beside a held lock = %v\n%s", err, out.String())
	}
	refusedCleanly(t, f.data, "")

	release()
	held = false
	out.Reset()
	if err := runImport(t.Context(), &out, &rootOptions{}, f.export.dir, 300*time.Millisecond, nil); err != nil {
		t.Fatalf("import after the lock went = %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(f.data, "frostmail.db")); err != nil {
		t.Errorf("the import placed no database: %v", err)
	}
}

// A successful import starts maild again once the data lock is free, so that
// the window that was open reconnects by itself. The fake takes the lock to
// prove that the import released it first: a maild that starts beside a held
// lock exits with "another maild uses".
func TestImportStartsMaildOnceTheLockIsFree(t *testing.T) {
	f := newImportFixture(t)
	var calls int
	start := func(ctx context.Context) error {
		calls++
		if ctx.Err() != nil {
			t.Errorf("startMaild got a finished context: %v", ctx.Err())
		}
		release, err := datalock.Acquire(f.data)
		if err != nil {
			return err
		}
		release()
		return nil
	}
	var out bytes.Buffer
	if err := importInto(t.Context(), &out, &rootOptions{}, f.export.dir, 5*time.Second, time.Now(), start); err != nil {
		t.Fatalf("import = %v\n%s", err, out.String())
	}
	if calls != 1 {
		t.Errorf("startMaild ran %d times, want 1\n%s", calls, out.String())
	}
	if !strings.Contains(out.String(), "maild started with the imported accounts; open windows reconnect.") {
		t.Errorf("the import does not say that maild started again:\n%s", out.String())
	}
}
