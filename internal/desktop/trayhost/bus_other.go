//go:build !linux

package trayhost

// nameHasOwner reports true: Windows and macOS always show the icon, and
// have no watcher to ask.
func nameHasOwner(string) (bool, error) { return true, nil }
