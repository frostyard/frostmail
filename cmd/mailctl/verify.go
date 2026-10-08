package main

// The safety check from the command line (docs/design/accounts.md, Safety
// check): compare the stored mail with the server, changing nothing.

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// errDiffers ends verify when a folder does not match the server.
var errDiffers = errors.New("the stored mail differs from the server")

// newVerifyCmd builds verify: check accounts against their servers.
func newVerifyCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "verify [ACCOUNT_ID]",
		Short: "Compare the stored mail with the server, changing nothing",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			ids, err := syncTargets(ctx, c, arg)
			if err != nil {
				return err
			}
			var reports []*api.VerifyReport
			for _, id := range ids {
				r, err := c.Account().Verify(ctx, &api.AccountVerifyParams{ID: id})
				if err != nil {
					return fmt.Errorf("account %d: %w", id, err)
				}
				reports = append(reports, r)
			}
			written, err := clix.OutputJSON(reports)
			if err != nil {
				return err
			}
			ok := true
			for _, r := range reports {
				if !written {
					printReport(cmd.OutOrStdout(), r)
				}
				ok = ok && r.Ok
			}
			if !ok {
				return errDiffers
			}
			return nil
		},
	}
}

// printReport writes one account's report: a line per folder, and what
// differs under it.
func printReport(w io.Writer, r *api.VerifyReport) {
	state := "ok"
	if !r.Ok {
		state = "DIFFERS"
	}
	fmt.Fprintf(w, "account %d: %s\n", r.AccountID, state)
	for _, mb := range r.Mailboxes {
		fmt.Fprintf(w, "  %s: %d on the server, %d here\n", mb.Path, mb.Server, mb.Local)
		if len(mb.MissingLocally) > 0 {
			fmt.Fprintf(w, "    missing here: UID %s\n", joinUIDs(mb.MissingLocally))
		}
		if len(mb.MissingOnServer) > 0 {
			fmt.Fprintf(w, "    missing on the server: UID %s\n", joinUIDs(mb.MissingOnServer))
		}
		if mb.FlagDiffs > 0 {
			fmt.Fprintf(w, "    flags differ: %d %s\n", mb.FlagDiffs, plural(mb.FlagDiffs, "message", "messages"))
		}
		if mb.LabelDiffs > 0 {
			fmt.Fprintf(w, "    labels differ: %d %s\n", mb.LabelDiffs, plural(mb.LabelDiffs, "message", "messages"))
		}
	}
}

func joinUIDs(uids []int64) string {
	s := make([]string, len(uids))
	for i, u := range uids {
		s[i] = strconv.FormatInt(u, 10)
	}
	return strings.Join(s, ", ")
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
