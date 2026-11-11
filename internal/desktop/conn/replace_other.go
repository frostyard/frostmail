//go:build !windows

package conn

import "os"

// replaceFile moves from over to.
func replaceFile(from, to string) error { return os.Rename(from, to) }
