package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

// parseServer parses a server given as host:port[/mode] into a ServerConfig.
// Without a mode, ports 993 and 465 mean tls and 143, 587 and 25 mean
// starttls; any other port needs an explicit mode.
func parseServer(s, username string) (api.ServerConfig, error) {
	mode := ""
	if i := strings.LastIndex(s, "/"); i >= 0 {
		mode, s = s[i+1:], s[:i]
	}
	var tls api.TLSMode
	switch mode {
	case "":
	case "tls":
		tls = api.TLSModeTLS
	case "starttls":
		tls = api.TLSModeStartTLS
	case "insecure":
		tls = api.TLSModeInsecure
	default:
		return api.ServerConfig{}, fmt.Errorf("unknown TLS mode %q: want tls, starttls or insecure", mode)
	}
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return api.ServerConfig{}, fmt.Errorf("parse server %q: %w", s, err)
	}
	if host == "" {
		return api.ServerConfig{}, fmt.Errorf("empty host in server %q", s)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return api.ServerConfig{}, fmt.Errorf("bad port in server %q: %w", s, err)
	}
	if port < 1 || port > 65535 {
		return api.ServerConfig{}, fmt.Errorf("port %d out of range in server %q", port, s)
	}
	if tls == "" {
		switch port {
		case 993, 465:
			tls = api.TLSModeTLS
		case 143, 587, 25:
			tls = api.TLSModeStartTLS
		default:
			return api.ServerConfig{}, fmt.Errorf("port %d has no default TLS mode: add /tls, /starttls or /insecure", port)
		}
	}
	return api.ServerConfig{Host: host, Port: int64(port), TLS: tls, Username: username}, nil
}

// serverString renders a ServerConfig the way account list shows it:
// host:port/mode.
func serverString(s api.ServerConfig) string {
	return fmt.Sprintf("%s:%d/%s", s.Host, s.Port, s.TLS)
}

// readPasswordLine reads the first line of the command's stdin, trimming the
// line ending. A final line without a newline is not an error.
func readPasswordLine(cmd *cobra.Command) (string, error) {
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// newAccountCmd builds the account command: managing accounts from the
// command line until the app has a wizard.
func newAccountCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Manage mail accounts",
	}
	cmd.AddCommand(
		newAccountAddCmd(opts),
		newAccountConnectCmd(opts),
		newAccountAuthorizeCmd(opts),
		newAccountListCmd(opts),
		newAccountRmCmd(opts),
		newAccountPasswordCmd(opts),
		newAccountSetCmd(opts),
	)
	return cmd
}

// newAccountAddCmd builds account add: create an account, optionally with a
// password read from stdin.
func newAccountAddCmd(opts *rootOptions) *cobra.Command {
	var name, imapStr, smtpStr, username string
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "add EMAIL",
		Short: "Add a mail account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]
			if imapStr == "" || smtpStr == "" {
				return errors.New("both --imap and --smtp are required")
			}
			if username == "" {
				username = email
			}
			imap, err := parseServer(imapStr, username)
			if err != nil {
				return fmt.Errorf("--imap: %w", err)
			}
			smtp, err := parseServer(smtpStr, username)
			if err != nil {
				return fmt.Errorf("--smtp: %w", err)
			}
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			acct, err := c.Account().Create(cmd.Context(), &api.AccountCreateParams{
				Kind:        api.AccountKindIMAP,
				Email:       email,
				DisplayName: name,
				Auth:        api.AuthKindPassword,
				IMAP:        &imap,
				SMTP:        &smtp,
			})
			if err != nil {
				return err
			}
			if passwordStdin {
				password, err := readPasswordLine(cmd)
				if err != nil {
					return err
				}
				if err := c.Account().SetPassword(cmd.Context(), &api.AccountSetPasswordParams{ID: acct.ID, Password: password}); err != nil {
					return err
				}
			}
			if written, err := clix.OutputJSON(acct); err != nil || written {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added account %d (%s)\n", acct.ID, acct.Email)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name used in From headers")
	cmd.Flags().StringVar(&imapStr, "imap", "", "IMAP server as host:port[/mode]")
	cmd.Flags().StringVar(&smtpStr, "smtp", "", "SMTP server as host:port[/mode]")
	cmd.Flags().StringVar(&username, "username", "", "login name (default: the email address)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password from the first line of stdin")
	return cmd
}

// newAccountListCmd builds account list: a table of accounts, or JSON.
func newAccountListCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List accounts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			accounts, err := c.Account().List(cmd.Context(), &api.AccountListParams{})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(accounts); err != nil || written {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tEMAIL\tIMAP\tSMTP")
			for _, a := range accounts {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", a.ID, a.Email, serverString(a.IMAP), serverString(a.SMTP))
			}
			return w.Flush()
		},
	}
}

// newAccountRmCmd builds account rm: remove an account and everything
// stored for it.
func newAccountRmCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "rm ID",
		Short: "Remove an account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid account id %q", args[0])
			}
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Account().Delete(cmd.Context(), &api.AccountDeleteParams{ID: id}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed account %d\n", id)
			return nil
		},
	}
}
