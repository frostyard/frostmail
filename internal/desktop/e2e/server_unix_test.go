//go:build e2e && !windows

package e2e

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// launcherStub stands in for the Flatpak launcher's --start-maild.
const launcherStub = `#!/bin/sh
[ "$1" = --start-maild ] || exit 2
nohup "$E2E_MAILD" >>"$E2E_ROOT/maild2.log" 2>&1 &
echo $! >"$E2E_ROOT/maild2.pid"
i=0
until "$E2E_MAILCTL" account list >/dev/null 2>&1; do
	i=$((i + 1))
	[ "$i" -gt 300 ] && exit 1
	sleep 0.1
done
`

// In the Flatpak the Server pane stops this computer's maild on the way to
// lodge, and starts it on the way back.
func TestServerPaneChangesMode(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.NotFoundHandler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	stubs := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubs, "frostmail"), []byte(launcherStub), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe("mailctl"), filepath.Join(stubs, "mailctl")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(stubs, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if b, err := os.ReadFile(filepath.Join(root, "maild2.pid")); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				if p, err := os.FindProcess(pid); err == nil {
					_ = p.Kill()
				}
			}
		}
	})
	s := start(t, options{path: stubs, fixture: []string{"-n", "2"}, env: []string{
		"FLATPAK_ID=org.frostyard.Frostmail", "E2E_MAILD=" + exe("maild"), "E2E_MAILCTL=" + exe("mailctl"), "E2E_ROOT=" + root}})
	config := filepath.Join(s.root, "config", "frostmail", "app.json")

	stopped := make(chan struct{})
	// Transfer ownership before starting the waiter: an early Fatal must
	// not leave the session cleanup calling Wait on this command too.
	maild := s.maild
	s.maild = nil
	go func() { _ = maild.Wait(); close(stopped) }()
	t.Cleanup(func() {
		_ = maild.Process.Kill()
		<-stopped
	})

	s.key(1, "CmdOrCtrl+,")
	w := s.waitWindow("^Settings$")
	s.press(w, s.waitNode(w, "treeItem", "^Server"))
	s.press(w, s.waitNode(w, "radio", "behind lodge"))
	field := s.waitNode(w, "textField", "lodge")
	address := "http://" + ln.Addr().String()
	s.must("act", map[string]any{"window": w, "id": field.ID, "action": "setValue", "text": address}, nil)
	s.idle(w)
	s.press(w, s.waitNode(w, "button", "^Save$"))
	eventually(t, "app.json naming lodge", func() bool {
		b, _ := os.ReadFile(config)
		return strings.Contains(string(b), address)
	})
	eventually(t, "maild stopped", func() bool {
		select {
		case <-stopped:
			return true
		default:
			return false
		}
	})
	s.waitFor(1, "lodge's address in the status bar", func(tree []node) bool { return find(tree, "", "Connection: Local") == nil })

	s.press(w, s.waitNode(w, "radio", "on this computer"))
	s.press(w, s.waitNode(w, "button", "^Save$"))
	eventually(t, "app.json without lodge", func() bool {
		b, err := os.ReadFile(config)
		return err == nil && !strings.Contains(string(b), address)
	})
	s.waitFor(1, "this computer's maild again", func(tree []node) bool { return find(tree, "", "Connection: Local") != nil })
}
