package main

// mailctl send and mailctl outbox (docs/design/send.md, Operational notes).
// Task T-0041 implements both; the stubs refuse to run.

import (
	"errors"

	"github.com/spf13/cobra"
)

var errNotYet = errors.New("not implemented yet (task T-0041)")

// newSendCmd builds the send command: compose a message from flags and
// stdin and queue it.
func newSendCmd(_ *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "send",
		Short: "Compose a message from flags and stdin and send it",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return errNotYet },
	}
}

// newOutboxCmd builds the outbox command: list, cancel and retry messages
// on their way out.
func newOutboxCmd(_ *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "outbox",
		Short: "List messages not yet sent",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return errNotYet },
	}
}
