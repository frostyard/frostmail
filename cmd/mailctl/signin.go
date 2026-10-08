package main

// Signing in from the command line (docs/design/accounts.md): the user's
// OAuth client, connecting an account by its address, and OAuth sign-in
// through the browser.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// newOAuthCmd builds the oauth command: the OAuth clients maild signs in with.
func newOAuthCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oauth",
		Short: "Manage the OAuth clients maild signs in with",
	}
	cmd.AddCommand(newOAuthSetClientCmd(opts), newOAuthShowCmd(opts))
	return cmd
}

// newOAuthSetClientCmd builds oauth set-client: store the client maild
// signs in to a provider with, its secret optionally read from stdin.
func newOAuthSetClientCmd(opts *rootOptions) *cobra.Command {
	var clientID string
	var secretStdin bool
	cmd := &cobra.Command{
		Use:   "set-client PROVIDER",
		Short: "Store the OAuth client maild signs in to a provider with",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := args[0]
			p := &api.OauthSetClientParams{Provider: api.OAuthProvider(provider), ClientID: clientID}
			if secretStdin {
				secret, err := readPasswordLine(cmd)
				if err != nil {
					return err
				}
				p.ClientSecret = &secret
			}
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			if _, err := c.Oauth().SetClient(cmd.Context(), p); err != nil {
				return err
			}
			if secretStdin {
				fmt.Fprintf(cmd.OutOrStdout(), "%s client set with its secret\n", provider)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "%s client set\n", provider)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&clientID, "client-id", "", "the OAuth client ID")
	cmd.Flags().BoolVar(&secretStdin, "secret-stdin", false, "read the client secret from the first line of stdin")
	return cmd
}

// newOAuthShowCmd builds oauth show: the client stored for a provider.
func newOAuthShowCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show PROVIDER",
		Short: "Show the OAuth client stored for a provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := args[0]
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			cl, err := c.Oauth().GetClient(cmd.Context(), &api.OauthGetClientParams{Provider: api.OAuthProvider(provider)})
			if err != nil {
				return err
			}
			secret := "no secret"
			if cl.HasSecret {
				secret = "secret stored"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s client %s (%s)\n", provider, cl.ClientID, secret)
			return nil
		},
	}
	return cmd
}

// newAccountConnectCmd builds account connect: add an account by its
// address, finding its servers, and sign it in with a password or OAuth.
func newAccountConnectCmd(opts *rootOptions) *cobra.Command {
	var name string
	var useOAuth, passwordStdin, readOnly, noNotify, noWait bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "connect EMAIL",
		Short: "Add an account by its address, finding its servers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]
			if useOAuth == passwordStdin {
				return errors.New("sign in with either --password-stdin or --oauth")
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			disc, err := c.Account().Discover(ctx, &api.AccountDiscoverParams{Email: email})
			if err != nil {
				return err
			}
			if disc.IMAP == nil || disc.SMTP == nil {
				domain := email[strings.LastIndexByte(email, '@')+1:]
				return fmt.Errorf("no servers found for %s; use mailctl account add with --imap and --smtp", domain)
			}
			if !useOAuth || !noWait {
				if _, err := c.Events().Subscribe(ctx, nil); err != nil {
					return fmt.Errorf("subscribe to events: %w", err)
				}
			}
			var password string
			if passwordStdin {
				if password, err = readPasswordLine(cmd); err != nil {
					return err
				}
			}
			auth := api.AuthKindPassword
			if useOAuth {
				auth = api.AuthKindOAuth2
			}
			notify := !noNotify
			acct, err := c.Account().Create(ctx, &api.AccountCreateParams{
				Kind:        disc.Kind,
				Email:       email,
				DisplayName: name,
				Auth:        auth,
				IMAP:        disc.IMAP,
				SMTP:        disc.SMTP,
				ReadOnly:    &readOnly,
				Notify:      &notify,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added account %d (%s, %s)\n", acct.ID, acct.Email, acct.Kind)
			if passwordStdin {
				if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: acct.ID, Password: password}); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "password stored")
				return nil
			}
			return signIn(ctx, cmd.OutOrStdout(), c, acct.ID, noWait, timeout)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name used in From headers")
	cmd.Flags().BoolVar(&useOAuth, "oauth", false, "sign in with OAuth instead of a password")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password from the first line of stdin")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "never change anything on the server")
	cmd.Flags().BoolVar(&noNotify, "no-notify", false, "do not notify on new mail")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "print the sign-in address and return without waiting")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "how long to wait for the sign-in")
	return cmd
}

// newAccountAuthorizeCmd builds account authorize: sign an OAuth account
// in through the browser.
func newAccountAuthorizeCmd(opts *rootOptions) *cobra.Command {
	var noWait bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "authorize ID",
		Short: "Sign an OAuth account in through the browser",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid account id %q", args[0])
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if !noWait {
				if _, err := c.Events().Subscribe(ctx, nil); err != nil {
					return fmt.Errorf("subscribe to events: %w", err)
				}
			}
			return signIn(ctx, cmd.OutOrStdout(), c, id, noWait, timeout)
		},
	}
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "print the sign-in address and return without waiting")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "how long to wait for the sign-in")
	return cmd
}

// signIn starts an OAuth sign-in for an account, prints the address to
// open in the browser, and unless told not to waits for the browser to
// finish it.
func signIn(ctx context.Context, w io.Writer, c *api.Client, id int64, noWait bool, timeout time.Duration) error {
	res, err := c.Account().Authorize(ctx, &api.AccountAuthorizeParams{ID: id})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "open this address in your browser to sign in:")
	fmt.Fprintln(w, res.URL)
	if noWait {
		return nil
	}
	return waitForSignIn(ctx, w, c, id, timeout)
}

// waitForSignIn reads events until the account reports signed in, or the
// timeout passes first. Creating an account also emits account.changed, so
// each event is checked with account.get rather than trusted.
func waitForSignIn(ctx context.Context, w io.Writer, c *api.Client, id int64, timeout time.Duration) error {
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
			changed, isAccount := ev.(api.AccountChanged)
			if !isAccount || changed.ID != id {
				continue
			}
			a, err := c.Account().Get(ctx, &api.AccountGetParams{ID: id})
			if err != nil {
				return err
			}
			if a.SignedIn {
				fmt.Fprintln(w, "signed in")
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("not signed in after %s", timeout)
		}
	}
}
