//go:build linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// maild's own log moves to one old copy when it holds more than 1 MiB, and
// a new one starts. A smaller log stays, and a missing one is made.
func TestTheLogFileRotatesAtOneMebibyte(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maild.log")
	old := filepath.Join(dir, "maild.log.1")
	open := func() {
		t.Helper()
		f, err := openLogFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	noOld := func() {
		t.Helper()
		if _, err := os.Stat(old); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("an old log = %v; want none", err)
		}
	}

	open() // a missing file is made, empty and private
	if fi, err := os.Stat(path); err != nil || fi.Size() != 0 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the new log = %v, %v; want an empty file of mode 0600", fi, err)
	}
	noOld()

	writeFile(t, path, "a line\n") // a small file stays
	open()
	if got, err := os.ReadFile(path); err != nil || string(got) != "a line\n" {
		t.Fatalf("the small log = %q, %v; want it kept", got, err)
	}
	noOld()

	writeFile(t, path, string(bytes.Repeat([]byte("x"), logLimit+1))) // over 1 MiB moves, replacing the old copy
	writeFile(t, old, "an older log\n")
	open()
	if fi, err := os.Stat(path); err != nil || fi.Size() != 0 {
		t.Fatalf("the log after rotation = %v, %v; want an empty file", fi, err)
	}
	if fi, err := os.Stat(old); err != nil || fi.Size() != logLimit+1 {
		t.Fatalf("the old log = %v, %v; want the big file", fi, err)
	}

	writeFile(t, path, string(bytes.Repeat([]byte("y"), logLimit))) // exactly 1 MiB stays
	open()
	if fi, err := os.Stat(path); err != nil || fi.Size() != logLimit {
		t.Fatalf("a log of exactly 1 MiB = %v, %v; want it kept", fi, err)
	}
}

// logTo points stdout and stderr at the file, so slog and a panic land in it.
// It changes the process's descriptors, so a child process runs it.
func TestLogToRedirectsStdoutAndStderr(t *testing.T) {
	if path := os.Getenv("MAILD_TEST_LOGTO"); path != "" {
		if err := logTo(path); err != nil {
			fmt.Fprintln(os.Stderr, "logTo:", err)
			os.Exit(3)
		}
		fmt.Fprintln(os.Stdout, "to stdout")
		fmt.Fprintln(os.Stderr, "to stderr")
		os.Exit(0)
	}
	path := filepath.Join(t.TempDir(), "maild.log")
	writeFile(t, path, "before\n")
	cmd := exec.Command(os.Args[0], "-test.run=^TestLogToRedirectsStdoutAndStderr$")
	cmd.Env = append(os.Environ(), "MAILD_TEST_LOGTO="+path)
	if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 {
		t.Fatalf("child = %q, %v; want no output on its own descriptors", out, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "before\nto stdout\nto stderr\n" {
		t.Fatalf("the log = %q, %v", got, err)
	}
}
