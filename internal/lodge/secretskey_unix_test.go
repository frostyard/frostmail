//go:build unix

package lodge_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestMakeSecretsKeySignalLeavesNothing sends SIGTERM to the script's process
// group during the encrypt, as systemd's stop does to the cgroup. The script
// must exit non-zero and leave no key and no temporary directory.
func TestMakeSecretsKeySignalLeavesNothing(t *testing.T) {
	stubDir := t.TempDir()
	stubCreds(t, stubDir)
	store := t.TempDir()
	fifo := filepath.Join(stubDir, "started")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := keyScriptCmd(t, store, stubDir, "STARTED="+fifo)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	killGroup := func(sig syscall.Signal) { _ = syscall.Kill(-cmd.Process.Pid, sig) }
	timeout := time.AfterFunc(20*time.Second, func() { killGroup(syscall.SIGKILL) })
	defer timeout.Stop()

	// Opening the FIFO blocks until the stub opens it for writing.
	opened := make(chan struct{})
	go func() {
		if f, err := os.Open(fifo); err == nil {
			f.Close()
		}
		close(opened)
	}()
	select {
	case <-opened:
	case err := <-done:
		t.Fatalf("the script ended before the encrypt started: %v", err)
	case <-t.Context().Done():
		killGroup(syscall.SIGKILL)
		t.Fatal("test ended while waiting for the stub")
	}

	killGroup(syscall.SIGTERM)
	if err := <-done; err == nil {
		t.Error("the script succeeded after SIGTERM")
	}
	if _, err := os.Stat(filepath.Join(store, "frostmail-secrets-key")); !os.IsNotExist(err) {
		t.Errorf("a key exists after SIGTERM: %v", err)
	}
	noTemp(t, store)
}
