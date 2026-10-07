package main

import (
	"fmt"

	"github.com/frostyard/clix"
	"github.com/spf13/cobra"
)

// newHelloCmd builds the hello command: the smallest end-to-end check
// that maild is running.
func newHelloCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "hello",
		Short: "Check that maild is running and print its identity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, h, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			if written, err := clix.OutputJSON(h); err != nil || written {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s (protocol %d)\n", h.Server, h.Protocol)
			return nil
		},
	}
}
