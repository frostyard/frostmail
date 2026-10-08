package main

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// newAccountPasswordCmd builds account password: replace the password (or
// app password) an account logs in with, read from stdin.
func newAccountPasswordCmd(opts *rootOptions) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "password ID",
		Short: "Replace the password an account logs in with",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid account id %q", args[0])
			}
			if !passwordStdin {
				return errors.New("give the password on stdin with --password-stdin")
			}
			password, err := readPasswordLine(cmd)
			if err != nil {
				return err
			}
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Account().SetPassword(cmd.Context(), &api.AccountSetPasswordParams{ID: id, Password: password}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "password stored for account %d\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password (or app password) from stdin")
	return cmd
}
