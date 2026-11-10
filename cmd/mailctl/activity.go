package main

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/spf13/cobra"
)

// levels orders activity levels, for --level.
var levels = []api.ActivityLevel{api.ActivityLevelDebug, api.ActivityLevelInfo, api.ActivityLevelWarn, api.ActivityLevelError}

// newActivityCmd prints what maild does in the background (ADR-0055): the
// newest entries, and with --follow the live ones, written as the desktop
// console writes them.
func newActivityCmd(opts *rootOptions) *cobra.Command {
	var follow bool
	var limit int64
	var level string
	cmd := &cobra.Command{
		Use: "activity", Short: "Show what maild does in the background", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			least := api.ActivityLevel(level)
			if !least.Valid() {
				return fmt.Errorf("--level must be debug, info, warn or error")
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			var seq int64
			if follow { // before reading the recent entries, so none falls between
				sub, err := c.Activity().Subscribe(ctx, &api.ActivitySubscribeParams{})
				if err != nil {
					return err
				}
				seq = sub.Seq
			}
			recent, err := c.Activity().Recent(ctx, &api.ActivityRecentParams{Limit: &limit})
			if err != nil {
				return err
			}
			if !follow {
				if written, err := clix.OutputJSON(recent); err != nil || written {
					return err
				}
			}
			n := &clientNames{c: c}
			w := cmd.OutOrStdout()
			for _, e := range recent {
				if err := printActivity(ctx, w, e, n, least); err != nil {
					return err
				}
				seq = max(seq, e.Seq)
			}
			if !follow {
				return nil
			}
			return followActivity(ctx, c, w, n, least, seq)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep printing entries as maild records them")
	cmd.Flags().Int64Var(&limit, "limit", 50, "how many of the newest entries to print first (1 to 500)")
	cmd.Flags().StringVar(&level, "level", "info", "the least level to print: debug, info, warn or error")
	return cmd
}

// followActivity prints the live entries newer than seq until ctx ends or
// the connection closes; a dropped entry (seq 0) is always new.
func followActivity(ctx context.Context, c *api.Client, w io.Writer, n *clientNames, least api.ActivityLevel, seq int64) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case env, ok := <-c.Notifications():
			if !ok {
				return c.Err()
			}
			ev, err := api.DecodeEvent(env.Event, env.Data)
			if err != nil {
				continue
			}
			a, ok := ev.(api.ActivityAdded)
			if !ok || (a.Entry.Seq != 0 && a.Entry.Seq <= seq) {
				continue
			}
			seq = max(seq, a.Entry.Seq)
			if err := printActivity(ctx, w, a.Entry, n, least); err != nil {
				return err
			}
		}
	}
}

func printActivity(ctx context.Context, w io.Writer, e api.ActivityEntry, n *clientNames, least api.ActivityLevel) error {
	if slices.Index(levels, e.Level) < slices.Index(levels, least) {
		return nil
	}
	n.load(ctx, e)
	source, text := activity.Line(e, n)
	_, err := fmt.Fprintf(w, "%s  %-5s  %s  %s\n", e.At.Local().Format("15:04:05"), e.Level, source, text)
	return err
}

// clientNames names accounts by their address and mailboxes by their name,
// as the app shows them; it asks maild again for an ID it does not know.
type clientNames struct {
	c         *api.Client
	accounts  map[int64]string
	mailboxes map[int64]string
}

func (n *clientNames) load(ctx context.Context, e api.ActivityEntry) {
	if n.accounts == nil || (e.AccountID != nil && n.accounts[*e.AccountID] == "") {
		n.accounts = map[int64]string{}
		if list, err := n.c.Account().List(ctx, &api.AccountListParams{}); err == nil {
			for _, a := range list {
				n.accounts[a.ID] = a.Email
			}
		}
	}
	if n.mailboxes == nil || (e.MailboxID != nil && n.mailboxes[*e.MailboxID] == "") {
		n.mailboxes = map[int64]string{}
		if list, err := n.c.Mailbox().List(ctx, &api.MailboxListParams{}); err == nil {
			for _, mb := range list {
				n.mailboxes[mb.ID] = mb.Name
			}
		}
	}
}

func (n *clientNames) Account(id int64) string { return n.accounts[id] }
func (n *clientNames) Mailbox(id int64) string { return n.mailboxes[id] }
