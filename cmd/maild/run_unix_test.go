//go:build !windows

package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/datalock"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// sandbox is one maild's environment in a temporary directory.
type sandbox struct {
	data, appJSON, info, socket string
}

// newSandbox points maild at temporary directories, with no keyring and no
// session bus, and no sync, as a Flatpak instance (FLATPAK_ID) when flatpak.
func newSandbox(t *testing.T, flatpak bool) sandbox {
	t.Helper()
	root := t.TempDir()
	socket, cleanup, err := rpctest.SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	sb := sandbox{
		data: filepath.Join(root, "data"), socket: socket,
		appJSON: filepath.Join(root, "config", "frostmail", "app.json"),
		info:    filepath.Join(root, "flatpak-info"),
	}
	writeFile(t, sb.info, "[Instance]\napp-commit=c0ffee\n")
	for k, v := range map[string]string{
		"FROSTMAIL_DATA_DIR": sb.data, "FROSTMAIL_CACHE_DIR": filepath.Join(root, "cache"),
		"FROSTMAIL_SOCKET": socket, "XDG_CONFIG_HOME": filepath.Join(root, "config"),
		"FROSTMAIL_FLATPAK_INFO": sb.info, "FROSTMAIL_SECRETS": "file", "FROSTMAIL_SYNC": "off",
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=" + filepath.Join(root, "no-bus"),
	} {
		t.Setenv(k, v)
	}
	if flatpak {
		t.Setenv("FLATPAK_ID", config.AppID)
	} else {
		t.Setenv("FLATPAK_ID", "")
	}
	return sb
}

func (sb sandbox) writeAppJSON(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(sb.appJSON), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sb.appJSON, content)
}

func (sb sandbox) noDatabase(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(sb.data, "frostmail.db")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the database = %v; want none: maild must not open it", err)
	}
}

// start runs maild; done receives run's result when it returns.
func (sb sandbox) start(t *testing.T, ctx context.Context) (ready <-chan struct{}, done <-chan error) {
	t.Helper()
	r, d := make(chan struct{}), make(chan error, 1)
	go func() { d <- runWith(ctx, nil, func() { close(r) }) }()
	return r, d
}

func (sb sandbox) release(t *testing.T) {
	t.Helper()
	release, err := datalock.Acquire(sb.data)
	if err != nil {
		t.Fatalf("the data lock is still held: %v", err)
	}
	release()
}

// rpc.shutdown answers, then maild stops as on SIGTERM: run returns nil, the
// lock is free and the socket answers no more. rpc.hello reports the build.
func TestRPCShutdownStopsMaildAfterItAnswers(t *testing.T) {
	sb := newSandbox(t, true)
	ready, done := sb.start(t, t.Context())
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("maild stopped before it served: %v", err)
	}
	c, h, err := api.Dial(t.Context(), sb.socket, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if h.Build == nil || *h.Build != "c0ffee" {
		t.Fatalf("hello build = %v; want c0ffee", h.Build)
	}
	// The caller has its answer, and only then does maild stop.
	if err := c.RPC().Shutdown(t.Context(), &api.RPCShutdownParams{}); err != nil {
		t.Fatalf("rpc.shutdown = %v; want its result", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("maild stopped with %v; want a clean stop", err)
	}
	sb.release(t)
	if c2, _, err := api.Dial(t.Context(), sb.socket, "test"); err == nil {
		_ = c2.Close()
		t.Fatal("the socket still answers after shutdown")
	}
}

// Outside the Flatpak, hello has no build.
func TestHelloHasNoBuildOutsideTheFlatpak(t *testing.T) {
	sb := newSandbox(t, false)
	ctx, cancel := context.WithCancel(t.Context())
	ready, done := sb.start(t, ctx)
	<-ready
	c, h, err := api.Dial(ctx, sb.socket, "test")
	if err != nil {
		t.Fatal(err)
	}
	if h.Build != nil {
		t.Fatalf("hello build = %q; want none", *h.Build)
	}
	_ = c.Close()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A second maild on the same data directory exits at once, whatever its
// socket, and leaves the first one's files alone.
func TestSecondMaildOnTheDataDirExits(t *testing.T) {
	sb := newSandbox(t, false)
	if err := os.MkdirAll(sb.data, 0o700); err != nil {
		t.Fatal(err)
	}
	release, err := datalock.Acquire(sb.data)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	err = run(t.Context(), nil)
	if err == nil || err.Error() != "another maild uses "+sb.data {
		t.Fatalf("run = %v; want %q", err, "another maild uses "+sb.data)
	}
	sb.noDatabase(t)
	if _, err := os.Stat(sb.socket); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the socket = %v; want none", err)
	}
}

// With app.json naming lodge, a Flatpak's maild takes the lock, reads the
// mode, releases the lock and exits 0 before the database opens or the
// socket is made. maild never writes app.json.
func TestFlatpakMaildExitsWhenAppJSONNamesLodge(t *testing.T) {
	sb := newSandbox(t, true)
	const appJSON = `{"lodge":"https://lodge.example.ts.net","other":true}`
	sb.writeAppJSON(t, appJSON)
	if err := run(t.Context(), nil); err != nil {
		t.Fatalf("run = %v; want nil (exit 0)", err)
	}
	sb.noDatabase(t)
	sb.release(t)
	if _, err := os.Stat(sb.socket); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the socket = %v; want none", err)
	}
	if got, err := os.ReadFile(sb.appJSON); err != nil || string(got) != appJSON {
		t.Fatalf("app.json = %q, %v; want it unchanged", got, err)
	}
}

// A malformed app.json stops a Flatpak's maild with an error that names it,
// and no database is opened.
func TestFlatpakMaildStopsOnMalformedAppJSON(t *testing.T) {
	sb := newSandbox(t, true)
	sb.writeAppJSON(t, `{"lodge": `)
	err := run(t.Context(), nil)
	if err == nil || !strings.Contains(err.Error(), sb.appJSON) {
		t.Fatalf("run = %v; want an error that names %s", err, sb.appJSON)
	}
	sb.noDatabase(t)
	sb.release(t)
}

// Outside the Flatpak, app.json is not read: a maild on the host serves
// whatever it says (the host's maild is lodge's, or the owner's own).
func TestHostMaildIgnoresAppJSON(t *testing.T) {
	sb := newSandbox(t, false)
	sb.writeAppJSON(t, `{"lodge":"https://lodge.example.ts.net"}`)
	ctx, cancel := context.WithCancel(t.Context())
	ready, done := sb.start(t, ctx)
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("maild stopped: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// -mode and -build run without the lock, the data directory or the socket.
func TestModeAndBuildFlags(t *testing.T) {
	sb := newSandbox(t, false)
	sb.writeAppJSON(t, `{"lodge":"https://l.example"}`)
	for _, args := range [][]string{{"-mode"}, {"-build"}} {
		if err := run(t.Context(), args); err != nil {
			t.Fatalf("run %v = %v", args, err)
		}
	}
	if err := run(t.Context(), []string{"-mode", "-build"}); err == nil {
		t.Fatal("-mode with -build: want an error")
	}
	if _, err := os.Stat(sb.data); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the data directory = %v; want none made", err)
	}
	sb.writeAppJSON(t, `nope`)
	if err := run(t.Context(), []string{"-mode"}); err == nil || !strings.Contains(err.Error(), "app.json") {
		t.Fatalf("-mode with a malformed file = %v; want an error naming it", err)
	}
	if err := os.Remove(sb.info); err != nil {
		t.Fatal(err)
	}
	if err := run(t.Context(), []string{"-build"}); err == nil {
		t.Fatal("-build without .flatpak-info: want an error")
	}
}
