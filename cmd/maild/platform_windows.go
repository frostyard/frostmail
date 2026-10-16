package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// maxLog is the size past which maild.log is rotated at start (one old
// file is kept).
const maxLog = 10 << 20

// logOutput is where maild logs on Windows: <data>\logs\maild.log, and
// stderr too when maild has a console. Built as a GUI program, maild run
// at sign-in has none (ADR-0038).
func logOutput(dataDir string) (io.Writer, func(), error) {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("log directory: %w", err)
	}
	path := filepath.Join(dir, "maild.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLog {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("log file: %w", err)
	}
	var w io.Writer = f
	if h, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE); err == nil && h != 0 && h != windows.InvalidHandle {
		w = io.MultiWriter(f, os.Stderr)
	}
	return w, func() { _ = f.Close() }, nil
}

const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "Frostmail"
)

// setAutostart starts maild when the user signs in to Windows, or stops
// doing so: the per-user Run key names this executable (ADR-0038).
func setAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the Run key: %w", err)
	}
	defer func() { _ = k.Close() }()
	if !on {
		if err := k.DeleteValue(runValue); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("remove the Run value: %w", err)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	return k.SetStringValue(runValue, `"`+exe+`"`)
}
