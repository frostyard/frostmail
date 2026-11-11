package workflowcheck

import (
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/frostyard/frostmail/internal/config"
)

// TestGitignoreEntriesStandAlone: every entry has its own line and the file
// ends with a newline, so an entry added later cannot join the last one,
// as one did and left /.envrc, which holds local secrets, not ignored.
func TestGitignoreEntriesStandAlone(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasSuffix(text, "\n") {
		t.Error(".gitignore has no final newline")
	}
	lines := strings.Split(text, "\n")
	for _, want := range []string{"/.env", "/.envrc", "/.mcp.json", "/resources/"} {
		if !slices.Contains(lines, want) {
			t.Errorf(".gitignore has no line %q", want)
		}
	}
}

// TestDevAppFakeMaild is the stand-in for maild that
// TestDevAppKeepsAwayFromTheUsersMaild runs: it makes the socket and, as maild
// does, ends cleanly on SIGTERM.
func TestDevAppFakeMaild(t *testing.T) {
	if os.Getenv("DEV_APP_FAKE_MAILD") == "" {
		t.Skip("run by TestDevAppKeepsAwayFromTheUsersMaild")
	}
	path := os.Getenv("FROSTMAIL_SOCKET")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM)
	<-stop
}

// TestDevAppKeepsAwayFromTheUsersMaild runs scripts/dev-app.sh with stand-ins
// for maild and the app: what the app is given must name neither the user's
// socket nor the user's app.json.
func TestDevAppKeepsAwayFromTheUsersMaild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	// Short: a socket's path has a limit.
	dir, err := os.MkdirTemp("", "devapp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	maild, app, got := filepath.Join(dir, "maild"), filepath.Join(dir, "app"), filepath.Join(dir, "env")
	script := "#!/bin/sh\nexec '" + self + "' -test.run='^TestDevAppFakeMaild$' -test.timeout=1m\n"
	if err := os.WriteFile(maild, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app, []byte("#!/bin/sh\nenv > '"+got+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	home, run := filepath.Join(dir, "home"), filepath.Join(dir, "run")
	if err := os.MkdirAll(run, 0o700); err != nil {
		t.Fatal(err)
	}
	user := map[string]string{"HOME": home, "XDG_RUNTIME_DIR": run, "XDG_CONFIG_HOME": filepath.Join(home, ".config")}
	cmd := exec.CommandContext(t.Context(), "bash", "scripts/dev-app.sh")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "XDG_RUNTIME_DIR=" + run,
		"XDG_CONFIG_HOME=" + user["XDG_CONFIG_HOME"], "MAILD=" + maild, "APP_BINARY=" + app, "DEV_APP_FAKE_MAILD=1"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dev-app.sh: %v\n%s", err, out)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for line := range strings.Lines(string(data)) {
		if k, v, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "="); ok {
			seen[k] = v
		}
	}
	userEnv := func(k string) string { return user[k] }
	appEnv := func(k string) string { return seen[k] }

	socket, err := config.DefaultSocket(userEnv)
	if err != nil {
		t.Fatal(err)
	}
	if s := seen["FROSTMAIL_SECRETS"]; s != "file" {
		t.Errorf("the dev run's secrets = %q, want its own file, not the user's keyring", s)
	}
	if s := seen["FROSTMAIL_SOCKET"]; s == "" || s == socket {
		t.Errorf("the app's socket = %q, the user's is %q", s, socket)
	}
	userConfig, err := config.AppConfigFile(userEnv)
	if err != nil {
		t.Fatal(err)
	}
	devConfig, err := config.AppConfigFile(appEnv)
	if err != nil {
		t.Fatal(err)
	}
	if devConfig == userConfig || strings.HasPrefix(devConfig, home) {
		t.Errorf("the app's app.json = %q, the user's is %q", devConfig, userConfig)
	}
}
