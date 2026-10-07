package rpcserver

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Listen creates the maild socket at path, mode 0600, in a directory that
// must be private (0700 and owned by the user; it is created if missing). A
// stale socket is replaced; a live one means maild is already running.
func Listen(path string) (net.Listener, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("socket directory: %w", err)
	}
	if err := checkPrivateDir(dir); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(path); err == nil {
		if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
			_ = c.Close()
			return nil, fmt.Errorf("maild is already listening on %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("listen: %w", err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(true)
	return ln, nil
}

func checkPrivateDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	switch {
	case !fi.IsDir():
		return fmt.Errorf("socket directory %s is not a directory", dir)
	case !ok || int(st.Uid) != os.Getuid():
		return fmt.Errorf("socket directory %s is not owned by uid %d", dir, os.Getuid())
	case fi.Mode().Perm()&0o077 != 0:
		return fmt.Errorf("socket directory %s has mode %v; want 0700", dir, fi.Mode().Perm())
	}
	return nil
}
