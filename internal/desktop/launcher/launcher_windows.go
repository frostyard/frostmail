//go:build windows

// Package launcher shows the All Inboxes count on the app's launcher icon.
// Windows has no Unity LauncherEntry signal, and the taskbar's overlay
// badge is out of scope, so this build's entry does nothing: the app compiles
// and runs the same way, and its count reaches no icon.
package launcher

import "log/slog"

// Entry is the app's launcher entry; on Windows it sends nothing.
type Entry struct{}

// Open returns an entry that does nothing: Windows has no session bus and
// no LauncherEntry signal.
func Open(*slog.Logger, func() int64) *Entry { return &Entry{} }

// Changed says the count may have changed; on Windows it does nothing.
func (*Entry) Changed() {}

// Close stops the entry; on Windows there is nothing to stop.
func (*Entry) Close() {}
