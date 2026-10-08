package main

// Signing in from the command line (docs/design/accounts.md): the user's
// OAuth client, connecting an account by its address, and OAuth sign-in.
// Task T-0053 implements the commands; the stubs refuse to run.

import (
	"errors"

	"github.com/spf13/cobra"
)

var errSignInNotYet = errors.New("not implemented yet (task T-0053)")

// newOAuthCmd builds the oauth command: the OAuth clients maild signs in with.
func newOAuthCmd(_ *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "oauth",
		Short: "Manage the OAuth clients maild signs in with",
		RunE:  func(*cobra.Command, []string) error { return errSignInNotYet },
	}
}

// newAccountConnectCmd builds account connect: add an account by its address.
func newAccountConnectCmd(_ *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "connect EMAIL",
		Short: "Add an account by its address, finding its servers",
		Args:  cobra.ExactArgs(1),
		RunE:  func(*cobra.Command, []string) error { return errSignInNotYet },
	}
}

// newAccountAuthorizeCmd builds account authorize: sign an OAuth account in.
func newAccountAuthorizeCmd(_ *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "authorize ID",
		Short: "Sign an OAuth account in through the browser",
		Args:  cobra.ExactArgs(1),
		RunE:  func(*cobra.Command, []string) error { return errSignInNotYet },
	}
}
