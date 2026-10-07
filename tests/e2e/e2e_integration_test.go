//go:build integration

// Package e2e runs the real maild and mailctl binaries against the
// frostmail-mailtest container (make e2e). These tests produce the M1 exit
// evidence (docs/plans/0003-m1-headless-read-path.md, phase 5).
package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
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
	socket  string
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
	socket := filepath.Join(run, "maild.sock")
	return &env{
		t: t, maild: maild, mailctl: mailctl, socket: socket,
		vars: append(os.Environ(),
			"FROSTMAIL_DATA_DIR="+filepath.Join(data, "data"),
			"FROSTMAIL_CACHE_DIR="+filepath.Join(data, "cache"),
			"FROSTMAIL_SOCKET="+socket,
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

// memory reads maild's resident set and its peak (VmRSS, VmHWM) in MiB.
func (e *env) memory() (rss, peak int64) {
	e.t.Helper()
	f, err := os.Open(fmt.Sprintf("/proc/%d/status", e.proc.Process.Pid))
	if err != nil {
		e.t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), ":")
		kb, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
		switch k {
		case "VmRSS":
			rss = kb / 1024
		case "VmHWM":
			peak = kb / 1024
		}
	}
	return rss, peak
}

// serverCount asks the server how many messages a user's INBOX holds.
func serverCount(t *testing.T, user string) uint32 {
	t.Helper()
	s, err := imapx.Open(t.Context(), imapx.DialOptions{Host: os.Getenv("FROSTMAIL_IT_HOST"), Port: 993,
		TLS: api.TLSModeTLS, Username: user + "@mailtest.test", Password: "frostmail-test", InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	st, err := s.Status(t.Context(), "INBOX")
	if err != nil {
		t.Fatal(err)
	}
	return st.Messages
}

// TestLargeMailbox syncs test5's 50,000 messages (exit criteria 1 and 5):
// every message arrives, maild's peak RSS stays under 300 MB, and a search
// over the mailbox answers in under 150 ms.
func TestLargeMailbox(t *testing.T) {
	e := newEnv(t)
	want := serverCount(t, "test5")
	if want < 50000 {
		t.Fatalf("test5 INBOX holds %d messages; run make mailtest-seed", want)
	}
	e.start()
	defer e.stop()
	start := time.Now()
	e.addAccount("test5")
	if _, err := e.ctl("sync", "1", "--wait", "--timeout", "20m"); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	rss, peak := e.memory()
	t.Logf("initial sync of %d messages: %v; RSS %d MiB, peak %d MiB", want, took.Round(time.Second), rss, peak)
	if peak >= 300 {
		t.Errorf("peak RSS %d MiB, want under 300", peak)
	}

	ctx := t.Context()
	c, _, err := api.Dial(ctx, e.socket, "e2e")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	inbox := e.inbox()
	all, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Total != int64(want) || all.Count != int64(want) {
		t.Fatalf("INBOX total %d, view count %d; server holds %d", inbox.Total, all.Count, want)
	}
	newest, err := c.View().Range(ctx, &api.ViewRangeParams{ID: all.ID, Start: 0, End: 1})
	if err != nil || len(newest) != 1 {
		t.Fatalf("range: %v, %d rows", err, len(newest))
	}

	// Search the way mailctl search does: open, read the first page, close.
	words := strings.Fields(strings.TrimPrefix(newest[0].Subject, "Re: "))
	queries := []string{words[0], strings.Join(words[:min(2, len(words))], " "), newest[0].From.Name}
	for _, q := range queries {
		var times []time.Duration
		var hits int64
		for range 10 {
			start := time.Now()
			v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Text: &q}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 50}); err != nil {
				t.Fatal(err)
			}
			if err := c.View().Close(ctx, &api.ViewCloseParams{ID: v.ID}); err != nil {
				t.Fatal(err)
			}
			times = append(times, time.Since(start))
			hits = v.Count
		}
		slices.Sort(times)
		t.Logf("search %q: %d hits, median %v, max %v", q, hits, times[len(times)/2].Round(time.Microsecond), times[len(times)-1].Round(time.Microsecond))
		if hits == 0 {
			t.Errorf("search %q found nothing", q)
		}
		if times[len(times)/2] >= 150*time.Millisecond {
			t.Errorf("search %q: median %v, want under 150 ms", q, times[len(times)/2])
		}
	}
}
