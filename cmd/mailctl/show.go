package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// showOutput is the JSON shape of the show command: the message and its
// body together.
type showOutput struct {
	Message *api.Message `json:"message"`
	Body    *api.Body    `json:"body"`
}

// addressString renders one address as "Name <addr>", or just the address
// when the display name is empty.
func addressString(a api.Address) string {
	if a.Name == "" {
		return a.Address
	}
	return a.Name + " <" + a.Address + ">"
}

// addressList joins addresses with ", ".
func addressList(as []api.Address) string {
	parts := make([]string, len(as))
	for i, a := range as {
		parts[i] = addressString(a)
	}
	return strings.Join(parts, ", ")
}

// attachmentNames lists the filenames of a message's attachments: parts
// with a filename whose disposition is not inline.
func attachmentNames(msg *api.Message) []string {
	var names []string
	for _, p := range msg.Parts {
		if p.Filename != "" && p.Disposition != "inline" {
			names = append(names, p.Filename)
		}
	}
	return names
}

// printMessage writes one message's headers and readable body text to w.
func printMessage(w io.Writer, msg *api.Message, body *api.Body) error {
	fmt.Fprintf(w, "From: %s\n", addressString(msg.Summary.From))
	fmt.Fprintf(w, "To: %s\n", addressList(msg.To))
	if len(msg.Cc) > 0 {
		fmt.Fprintf(w, "Cc: %s\n", addressList(msg.Cc))
	}
	fmt.Fprintf(w, "Date: %s\n", msg.Summary.Date.Local().Format("2006-01-02 15:04"))
	fmt.Fprintf(w, "Subject: %s\n", msg.Summary.Subject)
	if names := attachmentNames(msg); len(names) > 0 {
		fmt.Fprintf(w, "Attachments: %s\n", strings.Join(names, ", "))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, body.Text)
	return nil
}

// newShowCmd builds the show command: one message's headers and readable
// text, fetching the body from the server when it is not stored locally.
func newShowCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "show MESSAGE_ID",
		Short: "Show one message",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid message id %q", args[0])
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			msg, err := c.Message().Get(ctx, &api.MessageGetParams{ID: id})
			if err != nil {
				return err
			}
			body, err := c.Message().Body(ctx, &api.MessageBodyParams{ID: id})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(showOutput{Message: msg, Body: body}); err != nil || written {
				return err
			}
			return printMessage(cmd.OutOrStdout(), msg, body)
		},
	}
}

// syncTargets resolves the accounts to sync: the one named by arg, or every
// account in ID order.
func syncTargets(ctx context.Context, c *api.Client, arg string) ([]int64, error) {
	if arg != "" {
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid account id %q", arg)
		}
		return []int64{id}, nil
	}
	accounts, err := c.Account().List(ctx, &api.AccountListParams{})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	slices.Sort(ids)
	return ids, nil
}

// syncError builds the error that ends the command on a terminal sync phase.
func syncError(st api.SyncStatus) error {
	msg := fmt.Sprintf("account %d: %s", st.AccountID, st.Phase)
	if st.Error != nil && *st.Error != "" {
		msg += ": " + *st.Error
	}
	return errors.New(msg)
}

// joinIDs renders account IDs for an error message.
func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ", ")
}

// waitSync reads sync.progress events until every target account is up to
// date, a terminal phase arrives, or timeout passes. A phase other than
// idle marks an account as having started; idle after that marks it done.
func waitSync(ctx context.Context, c *api.Client, ids []int64, timeout time.Duration) error {
	started := make(map[int64]bool, len(ids))
	done := make(map[int64]bool, len(ids))
	remaining := len(ids)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for remaining > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			var waiting []int64
			for _, id := range ids {
				if !done[id] {
					waiting = append(waiting, id)
				}
			}
			return fmt.Errorf("timeout waiting for accounts %s", joinIDs(waiting))
		case env, ok := <-c.Notifications():
			if !ok {
				return fmt.Errorf("connection to maild ended: %w", c.Err())
			}
			if env.Event != "sync.progress" {
				continue
			}
			ev, err := api.DecodeEvent(env.Event, env.Data)
			if err != nil {
				return err
			}
			st, ok := ev.(api.SyncProgress)
			if !ok || !slices.Contains(ids, st.Status.AccountID) || done[st.Status.AccountID] {
				continue
			}
			switch st.Status.Phase {
			case api.SyncPhaseUnauthorized, api.SyncPhaseFailed:
				return syncError(st.Status)
			case api.SyncPhaseIdle:
				if started[st.Status.AccountID] {
					done[st.Status.AccountID] = true
					remaining--
				}
			default:
				started[st.Status.AccountID] = true
			}
		}
	}
	return nil
}

// newSyncCmd builds the sync command: ask maild to sync now, and with
// --wait report when every account is up to date.
func newSyncCmd(opts *rootOptions) *cobra.Command {
	var wait bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "sync [ACCOUNT_ID]",
		Short: "Ask maild to sync accounts now",
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
			if !wait {
				for _, id := range ids {
					if err := c.Sync().Now(ctx, &api.SyncNowParams{AccountID: id}); err != nil {
						return err
					}
					fmt.Fprintf(cmd.OutOrStdout(), "account %d: sync requested\n", id)
				}
				return nil
			}
			if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
				return err
			}
			for _, id := range ids {
				if err := c.Sync().Now(ctx, &api.SyncNowParams{AccountID: id}); err != nil {
					return err
				}
			}
			if err := waitSync(ctx, c, ids, timeout); err != nil {
				return err
			}
			for _, id := range ids {
				fmt.Fprintf(cmd.OutOrStdout(), "account %d: up to date\n", id)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "wait until every account is up to date")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "how long to wait with --wait")
	return cmd
}
