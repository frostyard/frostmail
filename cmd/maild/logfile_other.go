//go:build !linux

package main

import "errors"

// logTo is Linux's; on Windows maild writes <data>\logs\maild.log itself
// (logOutput).
func logTo(string) error { return errors.New("-log is for Linux") }
