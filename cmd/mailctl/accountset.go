package main

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// newAccountSetCmd builds account set: change an account's sync window
// (ADR-0016), read-only mode, notifications and the erasing of old Trash and
// Junk mail (ADR-0051).
func newAccountSetCmd(opts *rootOptions) *cobra.Command {
	var syncDays, eraseTrashDays, eraseJunkDays int64
	var readOnly, readWrite, notify, noNotify bool
	cmd := &cobra.Command{
		Use:   "set ID",
		Short: "Change an account's sync window, read-only mode, notifications and erasing of old Trash and Junk mail",
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
			if cmd.Flags().Changed("erase-trash-days") {
				p.EraseTrashDays = &eraseTrashDays
			}
			if cmd.Flags().Changed("erase-junk-days") {
				p.EraseJunkDays = &eraseJunkDays
			}
			if p.SyncDays == nil && p.ReadOnly == nil && p.Notify == nil && p.EraseTrashDays == nil && p.EraseJunkDays == nil {
				return errors.New("nothing to change: give --sync-days, --read-only, --read-write, --notify, --no-notify, " +
					"--erase-trash-days or --erase-junk-days")
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
			fmt.Fprintf(cmd.OutOrStdout(), "account %d (%s): keeps %s, %s, %s, %s, %s\n", a.ID, a.Email, window, mode, alerts,
				erases("Trash", a.EraseTrashDays), erases("Junk", a.EraseJunkDays))
			return nil
		},
	}
	cmd.Flags().Int64Var(&syncDays, "sync-days", 0, "keep the mail of the last `n` days; 0 keeps every message")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "never change anything on the server")
	cmd.Flags().BoolVar(&readWrite, "read-write", false, "send, move, flag and delete on the server")
	cmd.Flags().BoolVar(&notify, "notify", false, "notify on new mail")
	cmd.Flags().BoolVar(&noNotify, "no-notify", false, "do not notify on new mail")
	cmd.Flags().Int64Var(&eraseTrashDays, "erase-trash-days", 0, "erase what has been `n` days in Trash: 0 (never), 1, 7 or 30")
	cmd.Flags().Int64Var(&eraseJunkDays, "erase-junk-days", 0, "erase what has been `n` days in Junk: 0 (never), 1, 7 or 30")
	return cmd
}

// erases says what maild does with a mailbox's old mail, for the output
// line: "erases Trash after 7 days", or "never erases Trash".
func erases(name string, days *int64) string {
	if days == nil || *days == 0 {
		return "never erases " + name
	}
	return fmt.Sprintf("erases %s after %d days", name, *days)
}
