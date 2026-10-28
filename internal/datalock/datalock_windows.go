package datalock

// Acquire does nothing on Windows: a second maild finds the named pipe
// taken (ADR-0038). The lock, and the Flatpak that needs it, are Linux's.
func Acquire(string) (release func(), err error) { return func() {}, nil }
