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
	root.PersistentFlags().StringVar(&opts.socket, "socket", "", "maild socket (default: $XDG_RUNTIME_DIR/frostmail/maild.sock)")
	root.AddCommand(
		newHelloCmd(opts),
		newAccountCmd(opts),
		newMailboxesCmd(opts),
		newLsCmd(opts),
		newSearchCmd(opts),
		newShowCmd(opts),
		newSyncCmd(opts),
	)
	return root
}

// dial connects to maild and completes rpc.hello. The caller closes the client.
func (o *rootOptions) dial(ctx context.Context) (*api.Client, *api.Hello, error) {
	socket := o.socket
	if socket == "" {
		paths, err := config.Resolve(os.Getenv)
		if err != nil {
			return nil, nil, err
		}
		socket = paths.Socket
	}
	return api.Dial(ctx, socket, "mailctl "+version)
}
