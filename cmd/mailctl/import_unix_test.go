//go:build !windows

package main

// This test needs a real data lock, which internal/datalock makes a no-op on
// Windows.

import (
	"bytes"
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
	err = runImport(t.Context(), &out, &rootOptions{}, f.export.dir, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "is still locked") {
		t.Fatalf("import beside a held lock = %v\n%s", err, out.String())
	}
	refusedCleanly(t, f.data, "")

	release()
	held = false
	out.Reset()
	if err := runImport(t.Context(), &out, &rootOptions{}, f.export.dir, 300*time.Millisecond); err != nil {
		t.Fatalf("import after the lock went = %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(f.data, "frostmail.db")); err != nil {
		t.Errorf("the import placed no database: %v", err)
	}
}
