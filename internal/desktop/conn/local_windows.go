package conn

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/frostyard/frostmail/api"
)

// dialLocal connects to maild's named pipe. With no maild yet (the app
// started before the sign-in's autostart, or maild stopped), it starts the
// maild.exe beside the app and waits up to 10 s for its pipe
// (docs/design/desktop.md, Windows); a second maild started meanwhile
// finds the pipe taken and exits.
func dialLocal(ctx context.Context, pipe, client string) (*api.Client, *api.Hello, error) {
	started := false
	deadline := time.Now().Add(10 * time.Second)
	for {
		c, h, err := api.Dial(ctx, pipe, client)
		if err == nil || !errors.Is(err, fs.ErrNotExist) || time.Now().After(deadline) {
			return c, h, err
		}
		if !started {
			if err := startMaild(); err != nil {
				return nil, nil, err
			}
			started = true
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// startMaild starts maild.exe without a console, outliving the app.
func startMaild() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	maild := filepath.Join(filepath.Dir(self), "maild.exe")
	cmd := exec.Command(maild)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", maild, err)
	}
	return cmd.Process.Release()
}
