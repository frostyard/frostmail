//go:build !windows

package datalock

import (
	"errors"
	"io/fs"
	"testing"
)

func TestAcquireAdmitsOneHolder(t *testing.T) {
	dir := t.TempDir()
	release, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Acquire(dir)
	var busy *BusyError
	if !errors.As(err, &busy) || busy.Dir != dir || err.Error() != "another maild uses "+dir {
		t.Fatalf("second Acquire = %v; want a BusyError saying %q", err, "another maild uses "+dir)
	}
	release()
	release2, err := Acquire(dir)
	if err != nil {
		t.Fatalf("Acquire after release = %v", err)
	}
	release2()
	other, err := Acquire(t.TempDir()) // another directory has its own lock
	if err != nil {
		t.Fatal(err)
	}
	other()
}

func TestAcquireInAMissingDirectoryIsNotBusy(t *testing.T) {
	_, err := Acquire(t.TempDir() + "/missing")
	var busy *BusyError
	if err == nil || errors.As(err, &busy) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Acquire in a missing directory = %v; want the open error", err)
	}
}
