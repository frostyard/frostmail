//go:build !windows

package main

import (
	"errors"
	"io"
	"os"
)

// logOutput is where maild logs: stderr, which the user's systemd journal
// keeps.
func logOutput(string) (io.Writer, func(), error) {
	return os.Stderr, func() {}, nil
}

// setAutostart is Windows' (the Run key); on Linux the systemd user unit
// starts maild (make install).
func setAutostart(bool) error {
	return errors.New("-autostart is for Windows; on Linux, `make install` enables the systemd user unit")
}
