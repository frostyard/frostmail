// Package settings holds the client's settings that are this computer's,
// with no UI: the change between this computer's maild and lodge's
// (docs/specs/settings-ui.md, Server pane), the Server pane's local facts,
// and the close-to-tray preference.
package settings

import "context"

// Options are what a change of mode runs: the app's own configuration, and
// the steps that stop and start this computer's maild.
type Options struct {
	// ConfigFile is app.json's path.
	ConfigFile string
	// LeaveLocal saves and closes what keeps this computer's maild's
	// drafts open, and returns once they have saved: a change to lodge runs
	// it before StopLocal. It may be nil.
	LeaveLocal func()
	// StopLocal stops this computer's maild and returns once it has
	// stopped: mailctl shutdown.
	StopLocal func(ctx context.Context) error
	// StartLocal starts this computer's maild and returns once it answers:
	// frostmail --start-maild. Its error is the launcher's last line.
	StartLocal func(ctx context.Context) error
}
