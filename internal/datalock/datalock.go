// Package datalock is the one-maild-per-data-directory lock: an exclusive
// flock on <data dir>/maild.lock, held for the life of the process or until
// released. maild takes it before it opens the database, and mailctl takes
// it, only to release it, to learn that a stopped maild has let go of it
// (docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md). It is a
// no-op on Windows, whose single instance is the named pipe (ADR-0038).
package datalock

import "fmt"

// FileName is the lock file in the data directory.
const FileName = "maild.lock"

// BusyError says another process holds the lock of Dir.
type BusyError struct{ Dir string }

func (e *BusyError) Error() string { return fmt.Sprintf("another maild uses %s", e.Dir) }
