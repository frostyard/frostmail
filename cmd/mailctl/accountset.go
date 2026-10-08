package main

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// newAccountSetCmd builds account set: change an account's sync window
// (ADR-0016), read-only mode and notifications.
func newAccountSetCmd(opts *rootOptions) *cobra.Command {
	var syncDays int64
	var readOnly, readWrite, notify, noNotify bool
	cmd := &cobra.Command{
		Use:   "set ID",
		Short: "Change an account's sync window, read-only mode and notifications",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid account id %q", args[0])
			}
			if readOnly && readWrite || notify && noNotify {
				return errors.New("give --read-only or --read-write, --notify or --no-notify, not both")
			}
			p := &api.AccountUpdateParams{ID: id}
			if cmd.Flags().Changed("sync-days") {
				p.SyncDays = &syncDays
			}
			if readOnly || readWrite {
				p.ReadOnly = &readOnly
			}
			if notify || noNotify {
				p.Notify = &notify
			}
			if p.SyncDays == nil && p.ReadOnly == nil && p.Notify == nil {
				return errors.New("nothing to change: give --sync-days, --read-only, --read-write, --notify or --no-notify")
			}
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			a, err := c.Account().Update(cmd.Context(), p)
			if err != nil {
				return err
			}
			window := "every message"
			if a.SyncDays > 0 {
				window = fmt.Sprintf("the last %d days", a.SyncDays)
			}
			mode := "read-write"
			if a.ReadOnly {
				mode = "read-only"
			}
			alerts := "notifies"
			if !a.Notify {
				alerts = "does not notify"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "account %d (%s): keeps %s, %s, %s\n", a.ID, a.Email, window, mode, alerts)
			return nil
		},
	}
	cmd.Flags().Int64Var(&syncDays, "sync-days", 0, "keep the mail of the last `n` days; 0 keeps every message")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "never change anything on the server")
	cmd.Flags().BoolVar(&readWrite, "read-write", false, "send, move, flag and delete on the server")
	cmd.Flags().BoolVar(&notify, "notify", false, "notify on new mail")
	cmd.Flags().BoolVar(&noNotify, "no-notify", false, "do not notify on new mail")
	return cmd
}
