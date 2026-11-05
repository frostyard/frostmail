//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostmail/internal/problemrec"
	"github.com/frostyard/frostmail/internal/problemstest"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// A database whose schema is newer than this maild stops it, and records
// store.migrate_failed with the error's text.
func TestMigrateFailureRecordsStoreMigrateFailed(t *testing.T) {
	sb := newSandbox(t, false)
	if err := os.MkdirAll(sb.data, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(t.Context(), filepath.Join(sb.data, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	err = db.Tx(t.Context(), func(tx *store.Tx) error {
		_, err := tx.ExecContext(t.Context(), `INSERT INTO schema_migrations VALUES (9999, 'future.sql', 'x')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	j := problemstest.Listen(t)
	err = runWith(t.Context(), nil, runOptions{problems: j.Recorder(t, problems.ComponentDaemon)})
	if !errors.Is(err, store.ErrMigrate) {
		t.Fatalf("run = %v, want a migrate error", err)
	}
	rec := j.NextError(t, problems.ComponentDaemon, codeMigrateFailed)
	if rec.Message() != err.Error() {
		t.Errorf("MESSAGE = %q, want %q", rec.Message(), err)
	}
	j.None(t)
}

// With FROSTMAIL_SECRETS=sealed, a key that cannot unseal the store stops
// maild, and records secrets.unseal_failed.
func TestUnsealFailureRecordsSecretsUnsealFailed(t *testing.T) {
	newSandbox(t, false)
	cred := t.TempDir()
	if err := os.WriteFile(filepath.Join(cred, secrets.CredentialName), []byte("too short"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FROSTMAIL_SECRETS", "sealed")
	t.Setenv("CREDENTIALS_DIRECTORY", cred)

	j := problemstest.Listen(t)
	err := runWith(t.Context(), nil, runOptions{problems: j.Recorder(t, problems.ComponentDaemon)})
	if !errors.Is(err, secrets.ErrUnseal) {
		t.Fatalf("run = %v, want an unseal error", err)
	}
	rec := j.NextError(t, problems.ComponentDaemon, codeUnsealFailed)
	if !strings.Contains(rec.Message(), secrets.CredentialName) {
		t.Errorf("MESSAGE = %q", rec.Message())
	}
	j.None(t)
}

// A start that goes well, and one of an inert recorder, record nothing.
func TestCleanStartRecordsNothing(t *testing.T) {
	newSandbox(t, false)
	j := problemstest.Listen(t)
	for _, rec := range []*problems.Recorder{
		problemrec.New("dev", problems.ComponentDaemon, problems.WithSocket(j.Path)),
		nil,
	} {
		ctx, cancel := context.WithCancel(t.Context())
		if err := runWith(ctx, nil, runOptions{problems: rec, serving: cancel}); err != nil {
			t.Fatal(err)
		}
		cancel()
	}
	j.None(t)
}

// The helper processes of the panic tests: the test binary runs itself with
// helperEnv naming what to do, and socketEnv the test journal.
const (
	helperEnv = "FROSTMAIL_MAILD_PROBLEMS_HELPER"
	socketEnv = "FROSTMAIL_MAILD_PROBLEMS_SOCKET"
)

// runHelper runs the test name again as a helper process doing mode, in
// the test's environment (its sandbox), and returns its output.
func runHelper(t *testing.T, name, mode, socket string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+name+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode, socketEnv+"="+socket)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// helperRecorder is the recorder of a helper process: maild's own, made from
// the build version v1.2.3 that a release's ldflags give.
func helperRecorder() *problems.Recorder {
	version = "v" + problemstest.Version
	return problemrec.New(version, problems.ComponentDaemon, problems.WithSocket(os.Getenv(socketEnv)))
}

// hasMainFrame reports whether frames name the function fn of maild's main
// package. A test binary names the package by its import path; a built
// maild names it main, which the v1 frame pattern allows as well.
func hasMainFrame(frames []string, fn string) bool {
	return slices.ContainsFunc(frames, func(f string) bool {
		return f == "github.com/frostyard/frostmail/cmd/maild."+fn || f == "main."+fn
	})
}

// panicInMain panics, as a bug of maild's main goroutine would.
func panicInMain() {
	panic("maild test: a panic in main")
}

// A panic of the main goroutine under `defer rec.RecoverPanic()`, as main
// has it, records a panic with maild's frames, and the process still dies
// of it.
func TestMainPanicRecordsItsFrames(t *testing.T) {
	if os.Getenv(helperEnv) == "panic" {
		rec := helperRecorder()
		defer rec.RecoverPanic()
		panicInMain()
		return
	}
	j := problemstest.Listen(t)
	out, err := runHelper(t, "TestMainPanicRecordsItsFrames", "panic", j.Path)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || !strings.Contains(out, "maild test: a panic in main") {
		t.Fatalf("the helper = %v, want it to die of its panic:\n%s", err, out)
	}
	rec := j.NextPanic(t, problems.ComponentDaemon)
	if rec.Message() != "maild test: a panic in main" || !hasMainFrame(rec.Frames, "panicInMain") {
		t.Fatalf("the record = %q, frames %q", rec.Message(), rec.Frames)
	}
	j.None(t)
}

// A goroutine's crash, which no recover sees, is kept by CrashOutput and
// recorded by the next start of maild, with its frames.
func TestCrashIsRecordedAtTheNextStart(t *testing.T) {
	switch os.Getenv(helperEnv) {
	case "crash":
		crashing := func() { go crashInGoroutine() }
		_ = runWith(t.Context(), nil, runOptions{problems: helperRecorder(), serving: crashing})
		t.Fatal("maild stopped without the crash")
		return
	case "start":
		ctx, cancel := context.WithCancel(t.Context())
		if err := runWith(ctx, nil, runOptions{problems: helperRecorder(), serving: cancel}); err != nil {
			t.Fatal(err)
		}
		return
	}
	newSandbox(t, false)
	j := problemstest.Listen(t)
	out, err := runHelper(t, "TestCrashIsRecordedAtTheNextStart", "crash", j.Path)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || !strings.Contains(out, "maild test: a crash in a goroutine") {
		t.Fatalf("the crash helper = %v, want it to crash:\n%s", err, out)
	}
	j.None(t) // nothing recovers a goroutine's panic

	if out, err := runHelper(t, "TestCrashIsRecordedAtTheNextStart", "start", j.Path); err != nil {
		t.Fatalf("the next start = %v:\n%s", err, out)
	}
	rec := j.NextPanic(t, problems.ComponentDaemon)
	if !strings.Contains(rec.Message(), "maild test: a crash in a goroutine") || !hasMainFrame(rec.Frames, "crashInGoroutine") {
		t.Fatalf("the record = %q, frames %q", rec.Message(), rec.Frames)
	}
	j.None(t)
}

// crashInGoroutine panics on a goroutine of its own.
func crashInGoroutine() {
	panic("maild test: a crash in a goroutine")
}
