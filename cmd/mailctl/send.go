package main

// mailctl send and mailctl outbox (docs/design/send.md, Operational
// notes). send composes a message from flags and stdin, queues it as a
// draft and hands it to the outbox, optionally waiting for the outcome.
// outbox lists, cancels and retries messages on their way out.

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/mail"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// parseAddrs turns address flag values into addresses. A value that
// net/mail cannot parse is an error naming it.
func parseAddrs(values []string) ([]api.Address, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]api.Address, 0, len(values))
	for _, v := range values {
		a, err := mail.ParseAddress(v)
		if err != nil {
			return nil, fmt.Errorf("invalid address %q", v)
		}
		out = append(out, api.Address{Name: a.Name, Address: a.Address})
	}
	return out, nil
}

// absPaths makes attachment paths absolute: maild reads them itself.
func absPaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		a, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("attachment %q: %w", p, err)
		}
		out = append(out, a)
	}
	return out, nil
}

// plainToHTML turns a plain-text body into the editor's HTML: escaped,
// line breaks as <br>, wrapped in one paragraph.
func plainToHTML(body string) string {
	s := strings.ReplaceAll(strings.TrimRight(body, "\r\n"), "\r\n", "\n")
	s = strings.ReplaceAll(html.EscapeString(s), "\n", "<br>")
	return "<p>" + s + "</p>"
}

// newDraft fills a new draft with the recipients, subject and body, the
// created draft's own HTML (the signature block) last, and attaches the
// files. The draft is kept when any step fails.
func newDraft(ctx context.Context, c *api.Client, account int64, to, cc, bcc []api.Address, subject, body string, files []string) (*api.Draft, error) {
	p := &api.DraftCreateParams{Kind: api.DraftKindNew}
	if account != 0 {
		p.AccountID = &account
	}
	dr, err := c.Draft().Create(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("create draft: %w", err)
	}
	dr.Content.To, dr.Content.Cc, dr.Content.Bcc = to, cc, bcc
	dr.Content.Subject = subject
	dr.Content.HTML = body + dr.Content.HTML
	if _, err := c.Draft().Update(ctx, &api.DraftUpdateParams{ID: dr.ID, Content: dr.Content}); err != nil {
		return nil, fmt.Errorf("update draft %d: %w; the draft is kept", dr.ID, err)
	}
	for _, f := range files {
		if _, err := c.Draft().Attach(ctx, &api.DraftAttachParams{ID: dr.ID, Path: f}); err != nil {
			return nil, fmt.Errorf("attach %s: %w; the draft is kept", f, err)
		}
	}
	return dr, nil
}

// failedError reports why an outbox message failed, from its row.
func failedError(ctx context.Context, c *api.Client, id int64) error {
	items, err := c.Outbox().List(ctx, &api.OutboxListParams{})
	if err != nil {
		return fmt.Errorf("message %d not sent: %w", id, err)
	}
	for _, it := range items {
		if it.ID == id && it.Error != nil {
			return fmt.Errorf("message %d not sent: %s", id, *it.Error)
		}
	}
	return fmt.Errorf("message %d not sent", id)
}

// waitForSend reads events until the outbox message reaches a final
// state, or the timeout passes first.
func waitForSend(ctx context.Context, w io.Writer, c *api.Client, id int64, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		select {
		case env, ok := <-c.Notifications():
			if !ok {
				return errors.New("connection to maild ended while waiting")
			}
			ev, err := api.DecodeEvent(env.Event, env.Data)
			if err != nil {
				continue
			}
			o, isOutbox := ev.(api.OutboxChanged)
			if !isOutbox || o.ID != id {
				continue
			}
			switch {
			case o.Deleted:
				return fmt.Errorf("message %d was cancelled", id)
			case o.State == api.OutboxStateFailed:
				return failedError(ctx, c, id)
			case o.State == api.OutboxStateSent:
				if !clix.JSONOutput {
					fmt.Fprintln(w, "sent")
				}
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("message %d not sent after %s", id, timeout)
		}
	}
}

// newSendCmd builds the send command: compose a message from flags and
// stdin and queue it, with --wait following it out of the outbox.
func newSendCmd(opts *rootOptions) *cobra.Command {
	var to, cc, bcc, attach []string
	var subject string
	var account int64
	var isHTML, wait bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Compose a message from flags and stdin and send it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			recips, err := parseAddrs(to)
			if err != nil {
				return err
			}
			ccs, err := parseAddrs(cc)
			if err != nil {
				return err
			}
			bccs, err := parseAddrs(bcc)
			if err != nil {
				return err
			}
			if len(recips)+len(ccs)+len(bccs) == 0 {
				return errors.New("no recipients: use --to, --cc or --bcc")
			}
			files, err := absPaths(attach)
			if err != nil {
				return err
			}
			raw, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read body: %w", err)
			}
			body := string(raw)
			if !isHTML {
				body = plainToHTML(body)
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if wait {
				if _, err := c.Events().Subscribe(ctx, nil); err != nil {
					return fmt.Errorf("subscribe to events: %w", err)
				}
			}
			dr, err := newDraft(ctx, c, account, recips, ccs, bccs, subject, body, files)
			if err != nil {
				return err
			}
			item, err := c.Draft().Send(ctx, &api.DraftSendParams{ID: dr.ID})
			if err != nil {
				return fmt.Errorf("send draft %d: %w; the draft is kept", dr.ID, err)
			}
			written, err := clix.OutputJSON(item)
			if err != nil {
				return err
			}
			if !written {
				fmt.Fprintf(cmd.OutOrStdout(), "queued message %d from draft %d\n", item.ID, dr.ID)
			}
			if !wait {
				return nil
			}
			return waitForSend(ctx, cmd.OutOrStdout(), c, item.ID, timeout)
		},
	}
	cmd.Flags().StringArrayVar(&to, "to", nil, "recipient, \"Name <addr>\" or addr (repeatable)")
	cmd.Flags().StringArrayVar(&cc, "cc", nil, "carbon copy (repeatable)")
	cmd.Flags().StringArrayVar(&bcc, "bcc", nil, "blind carbon copy (repeatable)")
	cmd.Flags().StringVar(&subject, "subject", "", "the subject line")
	cmd.Flags().StringArrayVar(&attach, "attach", nil, "attach a file by path (repeatable)")
	cmd.Flags().Int64Var(&account, "account", 0, "send from this account (default: maild's choice)")
	cmd.Flags().BoolVar(&isHTML, "html", false, "stdin is already HTML")
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for the message to go out")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "how long --wait waits")
	return cmd
}

// printOutbox writes the outbox table to w: ID, STATE, ATTEMPTS, SUBJECT,
// ERROR. With no rows it prints "outbox is empty" instead of the table.
func printOutbox(w io.Writer, rows []api.OutboxItem) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "outbox is empty")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATE\tATTEMPTS\tSUBJECT\tERROR")
	for _, r := range rows {
		e := "-"
		if r.Error != nil {
			e = *r.Error
		}
		fmt.Fprintf(tw, "%d\t%s\t%d\t%s\t%s\n", r.ID, r.State, r.Attempts, truncateSubject(r.Subject), e)
	}
	return tw.Flush()
}

// newOutboxCmd builds the outbox command: list, cancel and retry messages
// on their way out.
func newOutboxCmd(opts *rootOptions) *cobra.Command {
	var account int64
	cmd := &cobra.Command{
		Use:   "outbox",
		Short: "List messages not yet sent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			p := &api.OutboxListParams{}
			if account != 0 {
				p.AccountID = &account
			}
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			items, err := c.Outbox().List(ctx, p)
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(items); err != nil || written {
				return err
			}
			return printOutbox(cmd.OutOrStdout(), items)
		},
	}
	cmd.Flags().Int64Var(&account, "account", 0, "limit to one account (default: all)")
	cmd.AddCommand(newOutboxCancelCmd(opts), newOutboxRetryCmd(opts))
	return cmd
}

// newOutboxCancelCmd builds outbox cancel: stop a queued message and keep
// its draft.
func newOutboxCancelCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "cancel OUTBOX_ID",
		Short: "Cancel a queued message and keep its draft",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs("outbox", args)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			dr, err := c.Outbox().Cancel(ctx, &api.OutboxCancelParams{ID: ids[0]})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cancelled message %d; draft %d kept\n", ids[0], dr.ID)
			return nil
		},
	}
}

// newOutboxRetryCmd builds outbox retry: queue a failed message again.
func newOutboxRetryCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "retry OUTBOX_ID",
		Short: "Queue a failed message again, now",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs("outbox", args)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Outbox().Retry(ctx, &api.OutboxRetryParams{ID: ids[0]}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "queued message %d again\n", ids[0])
			return nil
		},
	}
}
