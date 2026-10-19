package lodge_test

import (
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
		"usr/local/bin/lodge":                    "lodge",
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
	for _, p := range []string{"usr/local/bin/maild", "usr/local/bin/mailctl", "usr/lib/systemd/system/frostmail.socket",
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
