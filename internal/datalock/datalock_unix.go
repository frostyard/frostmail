//go:build !windows

package datalock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Acquire takes an exclusive lock on dir/maild.lock without waiting, and
// returns the function that releases it; the lock also ends with the
// process, even by a crash. dir must exist. A lock another process holds
// is a *BusyError. A second maild on the same data directory fails here at
// once, as the socket's "already listening" check cannot stop one that uses
// another socket, such as one in another Flatpak sandbox.
func Acquire(dir string) (release func(), err error) {
	f, err := os.OpenFile(filepath.Join(dir, FileName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, &BusyError{Dir: dir}
		}
		return nil, fmt.Errorf("lock %s: %w", dir, err)
	}
	return func() { _ = f.Close() }, nil
}
