package main

import (
	"context"
	"os"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/spf13/cobra"
)

// rootOptions are the persistent flags every subcommand shares.
type rootOptions struct {
	socket string
}

func newRootCmd() *cobra.Command {
	opts := &rootOptions{}
	root := &cobra.Command{
		Use:           "mailctl",
		Short:         "Control and inspect maild, the frostmail engine",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&opts.socket, "socket", "", `maild socket (default: $FROSTMAIL_SOCKET, else $XDG_RUNTIME_DIR/frostmail/maild.sock, in the Flatpak $XDG_RUNTIME_DIR/app/org.frostyard.Frostmail/frostmail/maild.sock; on Windows \\.\pipe\frostmail-<your SID>)`)
	root.AddCommand(
		newHelloCmd(opts),
		newShutdownCmd(opts),
		newAccountCmd(opts),
		newMailboxesCmd(opts),
		newLsCmd(opts),
		newSearchCmd(opts),
		newShowCmd(opts),
		newSyncCmd(opts),
		newFlagCmd(opts),
		newMvCmd(opts),
		newRmCmd(opts),
		newSendCmd(opts),
		newOutboxCmd(opts),
		newOAuthCmd(opts),
		newVerifyCmd(opts),
		newPeopleCmd(opts),
		newCalCmd(opts),
		newTasksCmd(opts),
		newWaitingCmd(opts),
		newRulesCmd(opts),
		newExportCmd(opts),
		newImportCmd(opts),
	)
	return root
}

// socketPath is the socket to dial: --socket, else $FROSTMAIL_SOCKET, else
// the default for this machine (in the Flatpak, the app's shared one).
func (o *rootOptions) socketPath() (string, error) {
	if o.socket != "" {
		return o.socket, nil
	}
	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return "", err
	}
	return paths.Socket, nil
}

// dial connects to maild and completes rpc.hello. The caller closes the client.
func (o *rootOptions) dial(ctx context.Context) (*api.Client, *api.Hello, error) {
	socket, err := o.socketPath()
	if err != nil {
		return nil, nil, err
	}
	return api.Dial(ctx, socket, "mailctl "+version)
}
