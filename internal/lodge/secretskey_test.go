package lodge_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubCreds writes a systemd-creds stand-in. It checks its arguments, counts
// the bytes on stdin into COUNT, records a call in CALLS, and writes a
// marker (never the bytes) to its output file. FAIL makes it fail.
func stubCreds(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "systemd-creds")
	body := `#!/bin/sh
echo called >> "$CALLS"
[ "$#" = 5 ] && [ "$1" = encrypt ] && [ "$2" = --with-key=auto ] && [ "$3" = --name=frostmail-secrets-key ] && [ "$4" = - ] || exit 64
if [ -n "${STARTED:-}" ]; then
	echo started > "$STARTED"
	exec sleep 30
fi
wc -c | tr -d ' ' > "$COUNT"
[ -z "${FAIL:-}" ] || exit 1
echo sealed-marker > "$5"
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// keyScriptCmd is the script's command with the stub's environment, plus
// extra. KEY_SHELL names the shell that runs it (default sh).
func keyScriptCmd(t *testing.T, store, stubDir string, extra ...string) *exec.Cmd {
	t.Helper()
	script, err := filepath.Abs("../../packaging/lodge/make-secrets-key.sh")
	if err != nil {
		t.Fatal(err)
	}
	shell := os.Getenv("KEY_SHELL")
	if shell == "" {
		shell = "sh"
	}
	cmd := exec.Command(shell, script)
	cmd.Env = append(os.Environ(),
		"CREDSTORE="+store,
		"SYSTEMD_CREDS="+filepath.Join(stubDir, "systemd-creds"),
		"CALLS="+filepath.Join(stubDir, "calls"),
		"COUNT="+filepath.Join(stubDir, "count"),
		"FAIL=")
	cmd.Env = append(cmd.Env, extra...)
	return cmd
}

func runKeyScript(t *testing.T, fail bool, store, stubDir string, extra ...string) (string, error) {
	t.Helper()
	if fail {
		extra = append(extra, "FAIL=1")
	}
	out, err := keyScriptCmd(t, store, stubDir, extra...).CombinedOutput()
	return string(out), err
}

func noTemp(t *testing.T, store string) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(store, ".frostmail-secrets-key.*")) // the private directory
	if err != nil || len(left) > 0 {
		t.Errorf("temporary files or directories left: %v, %v", left, err)
	}
}

func TestMakeSecretsKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the key script is for Linux")
	}

	t.Run("makes the directory and the key", func(t *testing.T) {
		stubDir := t.TempDir()
		stubCreds(t, stubDir)
		store := filepath.Join(t.TempDir(), "credstore.encrypted")
		if out, err := runKeyScript(t, false, store, stubDir); err != nil {
			t.Fatalf("script: %v\n%s", err, out)
		}
		di, err := os.Stat(store)
		if err != nil || di.Mode().Perm() != 0o700 {
			t.Errorf("directory %v, %v; want mode 0700", di, err)
		}
		ki, err := os.Stat(filepath.Join(store, "frostmail-secrets-key"))
		if err != nil || ki.Mode().Perm() != 0o600 {
			t.Errorf("key %v, %v; want mode 0600", ki, err)
		}
		if n, _ := os.ReadFile(filepath.Join(stubDir, "count")); strings.TrimSpace(string(n)) != "32" {
			t.Errorf("the stub read %q bytes, want 32", n)
		}
		noTemp(t, store)
	})

	t.Run("keeps a key that exists", func(t *testing.T) {
		stubDir := t.TempDir()
		stubCreds(t, stubDir)
		store := t.TempDir()
		key := filepath.Join(store, "frostmail-secrets-key")
		if err := os.WriteFile(key, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := runKeyScript(t, false, store, stubDir); err != nil {
			t.Fatalf("script: %v\n%s", err, out)
		}
		if b, _ := os.ReadFile(key); string(b) != "old" {
			t.Errorf("key is %q, want it unchanged", b)
		}
		if _, err := os.Stat(filepath.Join(stubDir, "calls")); !os.IsNotExist(err) {
			t.Errorf("systemd-creds was called: %v", err)
		}
		noTemp(t, store)
	})

	t.Run("a failing systemd-creds leaves no key", func(t *testing.T) {
		stubDir := t.TempDir()
		stubCreds(t, stubDir)
		store := t.TempDir()
		if out, err := runKeyScript(t, true, store, stubDir); err == nil {
			t.Fatalf("script succeeded:\n%s", out)
		}
		if _, err := os.Stat(filepath.Join(store, "frostmail-secrets-key")); !os.IsNotExist(err) {
			t.Errorf("a key exists after a failure: %v", err)
		}
		noTemp(t, store)
	})

	t.Run("a failed random source publishes no key", func(t *testing.T) {
		stubDir := t.TempDir()
		stubCreds(t, stubDir)
		store := t.TempDir()
		out, err := runKeyScript(t, false, store, stubDir, "URANDOM="+filepath.Join(stubDir, "missing"))
		if err == nil {
			t.Fatalf("script succeeded:\n%s", out)
		}
		if _, err := os.Stat(filepath.Join(store, "frostmail-secrets-key")); !os.IsNotExist(err) {
			t.Errorf("a key exists after a short read: %v", err)
		}
		noTemp(t, store)
	})
}
