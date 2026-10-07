package main

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// parseIDs parses every argument as an int64 ID of the named kind. A bad
// one is an error naming the offending argument.
func parseIDs(kind string, args []string) ([]int64, error) {
	ids := make([]int64, len(args))
	for i, a := range args {
		id, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid %s id %q", kind, a)
		}
		ids[i] = id
	}
	return ids, nil
}

// messageWord is "message" for a count of one and "messages" otherwise.
func messageWord(n int) string {
	if n == 1 {
		return "message"
	}
	return "messages"
}

// newFlagCmd builds the flag command: set or clear seen, flagged and the
// flag color on one or more messages. Every change is validated before
// maild is dialed.
func newFlagCmd(opts *rootOptions) *cobra.Command {
	var seen, unseen, flagged, unflagged bool
	var color int
	cmd := &cobra.Command{
		Use:   "flag MESSAGE_ID...",
		Short: "Change flags on messages",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fl := cmd.Flags()
			seenSet, unseenSet := fl.Changed("seen"), fl.Changed("unseen")
			flagSet, unflagSet := fl.Changed("flag"), fl.Changed("unflag")
			colorSet := fl.Changed("color")
			if seenSet && unseenSet {
				return errors.New("--seen and --unseen cannot be combined")
			}
			if flagSet && unflagSet {
				return errors.New("--flag and --unflag cannot be combined")
			}
			if colorSet && (color < 0 || color > 7) {
				return fmt.Errorf("--color %d is not 0-7", color)
			}
			if !seenSet && !unseenSet && !flagSet && !unflagSet && !colorSet {
				return errors.New("no flag change requested: use --seen, --unseen, --flag, --unflag or --color")
			}
			ids, err := parseIDs("message", args)
			if err != nil {
				return err
			}
			var ch api.FlagChanges
			if seenSet {
				ch.Seen = &seen
			}
			if unseenSet {
				off := false
				ch.Seen = &off
			}
			if flagSet {
				ch.Flagged = &flagged
			}
			if unflagSet {
				off := false
				ch.Flagged = &off
			}
			if colorSet {
				col := int64(color)
				ch.FlagColor = &col
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: ids, Changes: ch}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated %d %s\n", len(ids), messageWord(len(ids)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&seen, "seen", false, "mark as seen")
	cmd.Flags().BoolVar(&unseen, "unseen", false, "mark as unread")
	cmd.Flags().BoolVar(&flagged, "flag", false, "flag the message")
	cmd.Flags().BoolVar(&unflagged, "unflag", false, "remove the flag")
	cmd.Flags().IntVar(&color, "color", 0, "flag color, 0 (none) to 7")
	return cmd
}

// newMvCmd builds the mv command: move messages to another mailbox of the
// same account.
func newMvCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "mv MAILBOX_ID MESSAGE_ID...",
		Short: "Move messages to another mailbox",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mbID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid mailbox id %q", args[0])
			}
			ids, err := parseIDs("message", args[1:])
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Message().Move(ctx, &api.MessageMoveParams{IDs: ids, MailboxID: mbID}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "moved %d %s\n", len(ids), messageWord(len(ids)))
			return nil
		},
	}
}

// newRmCmd builds the rm command: delete messages, moving them to Trash.
func newRmCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "rm MESSAGE_ID...",
		Short: "Delete messages (move them to Trash)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs("message", args)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: ids}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %d %s\n", len(ids), messageWord(len(ids)))
			return nil
		},
	}
}
