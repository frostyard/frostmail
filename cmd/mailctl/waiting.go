package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// newWaitingCmd prints the waiting queries' threads (thread.awaitingReply
// and thread.awaitingOthers; ADR-0042), to check their rules against a
// store.
func newWaitingCmd(opts *rootOptions) *cobra.Command {
	var others bool
	var limit, waiting, within int64
	cmd := &cobra.Command{
		Use: "waiting", Short: "List conversations waiting on a reply from you, or with --others from others", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			flag := func(name string, v int64) *int64 {
				if cmd.Flags().Changed(name) {
					return &v
				}
				return nil
			}
			var got *api.Awaiting
			if others {
				got, err = c.Thread().AwaitingOthers(ctx, &api.ThreadAwaitingOthersParams{Limit: flag("limit", limit),
					WaitingDays: flag("waiting-days", waiting), WithinDays: flag("within-days", within)})
			} else {
				got, err = c.Thread().AwaitingReply(ctx, &api.ThreadAwaitingReplyParams{Limit: flag("limit", limit),
					WaitingDays: flag("waiting-days", waiting), WithinDays: flag("within-days", within)})
			}
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(got); err != nil || written {
				return err
			}
			return printWaiting(cmd.OutOrStdout(), got, others)
		},
	}
	cmd.Flags().BoolVar(&others, "others", false, "conversations waiting on others: your messages with no reply")
	cmd.Flags().Int64Var(&limit, "limit", 0, "at most this many (1 to 25; default 5, or 10 with --others)")
	cmd.Flags().Int64Var(&waiting, "waiting-days", 0, "at least this many days old (0 to 30; default 0, or 5 with --others)")
	cmd.Flags().Int64Var(&within, "within-days", 0, "less than this many days old (1 to 90; default 30)")
	return cmd
}

func printWaiting(w io.Writer, a *api.Awaiting, others bool) error {
	if len(a.Threads) == 0 {
		_, err := fmt.Fprintln(w, "nothing waiting")
		return err
	}
	who := "FROM"
	if others {
		who = "TO"
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "ID\tDAYS\tVIP\t%s\tSUBJECT\n", who)
	for _, t := range a.Threads {
		vip := "-"
		if t.Vip {
			vip = "*"
		}
		person := t.Message.From.Address
		if others {
			var to []string
			for _, r := range t.Recipients {
				to = append(to, r.Address)
			}
			person = strings.Join(to, ", ")
		}
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\n", t.Message.ID, t.WaitingDays, vip, pimCell(person), pimCell(t.Message.Subject))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if more := a.Total - int64(len(a.Threads)); more > 0 {
		_, err := fmt.Fprintf(w, "and %d more\n", more)
		return err
	}
	return nil
}
