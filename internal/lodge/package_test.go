package lodge_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/frostyard/lodge/kit/manifest"
)

// stage runs packaging/lodge/stage.sh, as make lodge-package does.
func stage(t *testing.T, args ...string) (string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("lodge packages for Linux")
	}
	script, err := filepath.Abs("../../packaging/lodge/stage.sh")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", append([]string{script}, args...)...).CombinedOutput()
	return string(out), err
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestStagingKeepsASharedRoot stages into a lodge root that other apps and
// lodge itself use, as dev/lodge/run.sh does: only mail's files change.
func TestStagingKeepsASharedRoot(t *testing.T) {
	root, build := t.TempDir(), t.TempDir()
	others := map[string]string{
		"usr/share/lodge/apps/notes/app.json":    `{"id": "notes"}`,
		"usr/share/lodge/apps/notes/web/a.js":    "notes",
		"var/lib/lodge/state.json":               "lodge's state",
		"var/lib/frostnotes/vault/Plan.md":       "# Plan",
		"run/lodge/apps/notes.sock":              "",
		"usr/bin/lodge":                          "lodge",
		"usr/lib/systemd/system/lodge.service":   "[Unit]",
		"var/lib/frostmail/frostmail.db":         "mail's data",
		"usr/share/lodge/apps/mail/web/old.js":   "a stale bundle file",
		"usr/share/lodge/apps/mail/app.json":     `{"version": "old"}`,
		"usr/share/lodge/apps/mailx/app.json":    "another app",
		"usr/share/lodge/apps/mail.bak/app.json": "a backup",
	}
	for p, c := range others {
		write(t, filepath.Join(root, p), c)
	}
	write(t, filepath.Join(build, "maild"), "maild")
	write(t, filepath.Join(build, "mailctl"), "mailctl")
	write(t, filepath.Join(build, "web/index.html"), "<!doctype html>")
	write(t, filepath.Join(build, "web/icon.svg"), "<svg/>")

	if out, err := stage(t, root, "v1.2.3-4-gabc", build); err != nil {
		t.Fatalf("stage.sh: %v\n%s", err, out)
	}
	for p, c := range others {
		if strings.HasPrefix(p, "usr/share/lodge/apps/mail/") {
			continue
		}
		if got, err := os.ReadFile(filepath.Join(root, p)); err != nil || string(got) != c {
			t.Errorf("%s = %q, %v after staging; want %q kept", p, got, err, c)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "usr/share/lodge/apps/mail/web/old.js")); !os.IsNotExist(err) {
		t.Errorf("a stale file of mail's web build survived: %v", err)
	}
	m, err := manifest.Load(filepath.Join(root, "usr/share/lodge/apps/mail"))
	if err != nil {
		t.Fatalf("staged manifest: %v", err)
	}
	if m.Version != "v1.2.3-4-gabc" || m.ID != "mail" {
		t.Errorf("staged manifest %+v, want mail at v1.2.3-4-gabc", m)
	}
	for _, p := range []string{"usr/bin/maild", "usr/bin/mailctl", "usr/lib/systemd/system/frostmail.socket",
		"usr/lib/systemd/system/frostmail.service", "usr/lib/sysusers.d/frostmail.conf"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("%s not staged: %v", p, err)
		}
	}
}

func TestStagingRefusesAnEmptyRootOrSlash(t *testing.T) {
	build := t.TempDir()
	write(t, filepath.Join(build, "maild"), "maild")
	write(t, filepath.Join(build, "mailctl"), "mailctl")
	for _, dest := range []string{"", "/", "//", "/.", "/usr/.."} {
		if out, err := stage(t, dest, "v1", build); err == nil {
			t.Errorf("stage.sh %q succeeded:\n%s", dest, out)
		}
	}
	if out, err := stage(t, t.TempDir(), `v1"; rm -rf /tmp/x; "`, build); err == nil {
		t.Errorf("stage.sh with a version a manifest cannot carry succeeded:\n%s", out)
	}
}

// TestStagedLayout stages into an empty root and checks core ADR-0058's /usr
// layout: nothing outside usr/, the exact files, and the units' rules.
func TestStagedLayout(t *testing.T) {
	root, build := t.TempDir(), t.TempDir()
	write(t, filepath.Join(build, "maild"), "maild")
	write(t, filepath.Join(build, "mailctl"), "mailctl")
	write(t, filepath.Join(build, "web/index.html"), "<!doctype html>")
	write(t, filepath.Join(build, "web/icon.svg"), "<svg/>")
	if out, err := stage(t, root, "v1.2.3", build); err != nil {
		t.Fatalf("stage.sh: %v\n%s", err, out)
	}

	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel != "." && rel != "usr" && !strings.HasPrefix(rel, "usr"+string(filepath.Separator)) {
			t.Errorf("staged %s outside usr/", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, b := range []string{"maild", "mailctl"} {
		fi, err := os.Stat(filepath.Join(root, "usr/bin", b))
		if err != nil {
			t.Fatalf("usr/bin/%s not staged: %v", b, err)
		}
		if fi.Mode().Perm() != 0o755 {
			t.Errorf("usr/bin/%s mode %v, want 0755", b, fi.Mode().Perm())
		}
	}
	units := []string{"frostmail.service", "frostmail.socket"}
	for _, f := range append([]string{"usr/lib/sysusers.d/frostmail.conf", "usr/share/lodge/apps/mail/web/index.html",
		"usr/share/lodge/apps/mail/web/icon.svg"}, prefixed("usr/lib/systemd/system/", units)...) {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("%s not staged: %v", f, err)
		}
	}
	m, err := manifest.Load(filepath.Join(root, "usr/share/lodge/apps/mail"))
	if err != nil || m.Version != "v1.2.3" {
		t.Errorf("staged manifest %+v, %v; want version v1.2.3", m, err)
	}

	unitDir := filepath.Join(root, "usr/lib/systemd/system")
	for _, u := range units {
		b, err := os.ReadFile(filepath.Join(unitDir, u))
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range strings.Split(string(b), "\n") {
			switch {
			case strings.HasPrefix(l, "RequiredBy="):
				t.Errorf("%s has %q (core ADR-0030)", u, l)
			case strings.HasPrefix(l, "Condition"):
				t.Errorf("%s has %q (an upheld unit with an unmet condition is retried forever)", u, l)
			case strings.HasSuffix(u, ".socket") && strings.HasPrefix(l, "DefaultDependencies="):
				t.Errorf("%s has %q; a socket keeps its default dependencies", u, l)
			case strings.HasSuffix(u, ".service") && strings.HasPrefix(l, "ExecStart="):
				bin := strings.Fields(strings.TrimPrefix(l, "ExecStart="))[0]
				if filepath.Dir(bin) != "/usr/bin" {
					t.Errorf("%s ExecStart %q is not in /usr/bin", u, bin)
				} else if _, err := os.Stat(filepath.Join(root, bin)); err != nil {
					t.Errorf("%s runs %s, which the script does not stage: %v", u, bin, err)
				}
			}
		}
	}

	sys, err := os.ReadFile(filepath.Join(root, "usr/lib/sysusers.d/frostmail.conf"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(sys), "\n")
	has := func(prefix string) bool {
		for _, l := range lines {
			if strings.HasPrefix(l, prefix) {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"g lodge -", "u frostmail ", "m frostmail lodge-apps"} {
		if !has(want) {
			t.Errorf("sysusers.d entry lacks a %q line", want)
		}
	}
	for _, l := range lines {
		if f := strings.Fields(l); len(f) == 3 && f[0] == "m" && f[1] == "frostmail" && f[2] == "lodge" {
			t.Errorf("sysusers.d entry puts frostmail in group lodge: %q", l)
		}
	}

	analyze, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Log("no systemd-analyze: verify skipped")
		return
	}
	dir := t.TempDir()
	for _, u := range units {
		b, err := os.ReadFile(filepath.Join(unitDir, u))
		if err != nil {
			t.Fatal(err)
		}
		ls := strings.Split(string(b), "\n")
		for i, l := range ls {
			if strings.HasPrefix(l, "ExecStart=") {
				ls[i] = "ExecStart=/bin/sh -c true" // verify wants the binary to exist
			}
		}
		write(t, filepath.Join(dir, u), strings.Join(ls, "\n"))
	}
	cmd := exec.Command(analyze, "verify", "./frostmail.socket", "./frostmail.service")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("systemd-analyze verify: %v\n%s", err, out)
	}
}

func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}
