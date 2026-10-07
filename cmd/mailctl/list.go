package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// printMessages writes the message table to w: ID, FLAGS, DATE, FROM,
// SUBJECT, in the order the view returns them (newest first). With no rows
// it prints "no messages" instead of the table.
func printMessages(w io.Writer, rows []api.MessageSummary) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "no messages")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tFLAGS\tDATE\tFROM\tSUBJECT")
	for _, r := range rows {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n",
			r.ID, flagString(r.Flags), r.Date.Local().Format("2006-01-02 15:04"),
			fromString(r.From), truncateSubject(r.Subject))
	}
	return tw.Flush()
}

// flagString renders a message's flags for the list: N when unseen, then !
// when flagged, so N!, N, ! or empty.
func flagString(f api.Flags) string {
	var b strings.Builder
	if !f.Seen {
		b.WriteByte('N')
	}
	if f.Flagged {
		b.WriteByte('!')
	}
	return b.String()
}

// fromString is the sender's display name, or the address when the name is
// empty.
func fromString(a api.Address) string {
	if a.Name != "" {
		return a.Name
	}
	return a.Address
}

// truncateSubject shortens a subject longer than 46 runes to its first 45
// runes plus an ellipsis.
func truncateSubject(s string) string {
	r := []rune(s)
	if len(r) > 46 {
		return string(r[:45]) + "…"
	}
	return s
}

// runView opens a view for the query, reads up to limit rows, and prints
// them as a table or JSON. The view is closed when the command returns.
func runView(cmd *cobra.Command, opts *rootOptions, q api.ViewQuery, limit int) error {
	ctx := cmd.Context()
	c, _, err := opts.dial(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	info, err := c.View().Open(ctx, &api.ViewOpenParams{Query: q})
	if err != nil {
		return err
	}
	defer c.View().Close(ctx, &api.ViewCloseParams{ID: info.ID})
	end := info.Count
	if int64(limit) < end {
		end = int64(limit)
	}
	rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: info.ID, Start: 0, End: end})
	if err != nil {
		return err
	}
	if written, err := clix.OutputJSON(rows); err != nil || written {
		return err
	}
	return printMessages(cmd.OutOrStdout(), rows)
}

// newMailboxesCmd builds the mailboxes command: a table of mailboxes with
// their counts, or JSON.
func newMailboxesCmd(opts *rootOptions) *cobra.Command {
	var account string
	cmd := &cobra.Command{
		Use:   "mailboxes",
		Short: "List mailboxes with their counts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			p := &api.MailboxListParams{}
			if account != "" {
				id, err := strconv.ParseInt(account, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid account id %q", account)
				}
				p.AccountID = &id
			}
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			mbs, err := c.Mailbox().List(ctx, p)
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(mbs); err != nil || written {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tACCOUNT\tROLE\tTOTAL\tUNREAD\tPATH")
			for _, m := range mbs {
				fmt.Fprintf(w, "%d\t%d\t%s\t%d\t%d\t%s\n", m.ID, m.AccountID, m.Role, m.Total, m.Unread, m.Path)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "limit to one account (default: all)")
	return cmd
}

// newLsCmd builds the ls command: a mailbox's messages, newest first.
func newLsCmd(opts *rootOptions) *cobra.Command {
	var limit int
	var unread, flagged bool
	cmd := &cobra.Command{
		Use:   "ls MAILBOX_ID",
		Short: "List a mailbox's messages, newest first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mbID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid mailbox id %q", args[0])
			}
			yes := true
			q := api.ViewQuery{MailboxID: &mbID}
			if unread {
				q.Unread = &yes
			}
			if flagged {
				q.Flagged = &yes
			}
			return runView(cmd, opts, q, limit)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "maximum number of messages")
	cmd.Flags().BoolVar(&unread, "unread", false, "only unread messages")
	cmd.Flags().BoolVar(&flagged, "flagged", false, "only flagged messages")
	return cmd
}

// newSearchCmd builds the search command: full-text search over messages.
func newSearchCmd(opts *rootOptions) *cobra.Command {
	var account string
	var limit int
	cmd := &cobra.Command{
		Use:   "search TERMS...",
		Short: "Search messages by full-text terms",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := api.ViewQuery{}
			if account != "" {
				id, err := strconv.ParseInt(account, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid account id %q", account)
				}
				q.AccountID = &id
			}
			text := strings.Join(args, " ")
			q.Text = &text
			return runView(cmd, opts, q, limit)
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "limit to one account (default: all)")
	cmd.Flags().IntVar(&limit, "limit", 50, "maximum number of messages")
	return cmd
}
