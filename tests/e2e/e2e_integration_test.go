//go:build integration

// Package e2e runs the real maild and mailctl binaries against the
// frostmail-mailtest container (make e2e). These tests produce the M1 exit
// evidence (docs/plans/0003-m1-headless-read-path.md, phase 5).
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// binaries builds maild and mailctl once per test run.
func binaries(t *testing.T) (maild, mailctl string) {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("go", "build", "-o", dir, "../../cmd/maild", "../../cmd/mailctl")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return filepath.Join(dir, "maild"), filepath.Join(dir, "mailctl")
}

// env is one maild installation: its directories and socket.
type env struct {
	t       *testing.T
	maild   string
	mailctl string
	vars    []string
	proc    *exec.Cmd
	log     *bytes.Buffer
}

func newEnv(t *testing.T) *env {
	t.Helper()
	if os.Getenv("FROSTMAIL_E2E") != "1" {
		t.Skip("end-to-end tests need the seeded mail server; run make mailtest-seed, then make e2e")
	}
	maild, mailctl := binaries(t)
	data := t.TempDir()
	run, err := os.MkdirTemp("", "fme2e")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(run) })
	return &env{
		t: t, maild: maild, mailctl: mailctl,
		vars: append(os.Environ(),
			"FROSTMAIL_DATA_DIR="+filepath.Join(data, "data"),
			"FROSTMAIL_CACHE_DIR="+filepath.Join(data, "cache"),
			"FROSTMAIL_SOCKET="+filepath.Join(run, "maild.sock"),
			"FROSTMAIL_INSECURE_TLS=1",
		),
	}
}

// start runs maild and waits for its socket.
func (e *env) start() {
	e.t.Helper()
	e.log = &bytes.Buffer{}
	e.proc = exec.Command(e.maild)
	e.proc.Env = e.vars
	e.proc.Stderr = e.log
	if err := e.proc.Start(); err != nil {
		e.t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := e.ctl("hello"); err == nil {
			return
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("maild did not start:\n%s", e.log)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// kill stops maild with SIGKILL, as a crash would.
func (e *env) kill() {
	e.t.Helper()
	_ = e.proc.Process.Signal(syscall.SIGKILL)
	_ = e.proc.Wait()
}

func (e *env) stop() {
	_ = e.proc.Process.Signal(syscall.SIGTERM)
	_ = e.proc.Wait()
}

func (e *env) ctl(args ...string) (string, error) {
	cmd := exec.Command(e.mailctl, args...)
	cmd.Env = e.vars
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if err != nil {
		return out.String(), fmt.Errorf("mailctl %s: %w\n%s", strings.Join(args, " "), err, errOut.String())
	}
	return out.String(), nil
}

func (e *env) ctlStdin(stdin string, args ...string) error {
	cmd := exec.Command(e.mailctl, args...)
	cmd.Env = e.vars
	cmd.Stdin = strings.NewReader(stdin)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mailctl %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

func (e *env) addAccount(user string) {
	e.t.Helper()
	host := os.Getenv("FROSTMAIL_IT_HOST")
	err := e.ctlStdin("frostmail-test\n", "account", "add", user+"@mailtest.test", "--imap", host+":993",
		"--smtp", host+":587", "--password-stdin")
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) inbox() api.Mailbox {
	e.t.Helper()
	out, err := e.ctl("--json", "mailboxes")
	if err != nil {
		e.t.Fatal(err)
	}
	var mbs []api.Mailbox
	if err := json.Unmarshal([]byte(out), &mbs); err != nil {
		e.t.Fatalf("%v: %q", err, out)
	}
	for _, mb := range mbs {
		if mb.Role == api.MailboxRoleInbox {
			return mb
		}
	}
	return api.Mailbox{}
}

// TestCrashDuringInitialSync kills maild with SIGKILL partway through the
// initial sync of 5,000 messages, restarts it, and checks that local state
// equals the server's: every message once.
func TestCrashDuringInitialSync(t *testing.T) {
	e := newEnv(t)
	e.start()
	e.addAccount("test4")
	deadline := time.Now().Add(60 * time.Second)
	for e.inbox().Total < 1000 {
		if time.Now().After(deadline) {
			t.Fatalf("sync did not reach 1,000 messages:\n%s", e.log)
		}
		time.Sleep(20 * time.Millisecond)
	}
	before := e.inbox().Total
	e.kill()
	t.Logf("killed maild with %d of 5000 messages stored", before)
	if before >= 5000 {
		t.Skip("sync finished before the kill; nothing to resume")
	}

	e.start()
	defer e.stop()
	if _, err := e.ctl("sync", "1", "--wait", "--timeout", "2m"); err != nil {
		t.Fatal(err)
	}
	inbox := e.inbox()
	if inbox.Total != 5000 {
		t.Fatalf("INBOX total after resume = %d, want 5000", inbox.Total)
	}
	out, err := e.ctl("--json", "ls", fmt.Sprint(inbox.ID), "--limit", "10000")
	if err != nil {
		t.Fatal(err)
	}
	var rows []api.MessageSummary
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	subjects := map[string]int{}
	for _, r := range rows {
		ids[r.ID] = true
		subjects[r.Subject+r.Date.String()]++
	}
	if len(rows) != 5000 || len(ids) != 5000 {
		t.Fatalf("listed %d rows, %d distinct IDs; want 5000", len(rows), len(ids))
	}
	for k, n := range subjects {
		if n > 1 {
			t.Fatalf("message %q stored %d times", k, n)
		}
	}
}
