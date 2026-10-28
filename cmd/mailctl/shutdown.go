package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/datalock"
)

const (
	// shutdownWait is how long shutdown waits for maild to stop answering.
	shutdownWait = 30 * time.Second
	// shutdownPoll is how often it asks again.
	shutdownPoll = 100 * time.Millisecond
)

// newShutdownCmd builds shutdown: stop maild and wait until it has stopped.
func newShutdownCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "shutdown",
		Short: "Stop maild and wait until it no longer answers",
		Long: `Call rpc.shutdown, which makes maild finish the work in hand, release its
data directory's lock and exit, then wait up to 30 seconds in all: until the
socket no longer answers rpc.hello, and until it can take the data
directory's lock itself, which it releases at once. It exits 0 once maild has
stopped, or at once when no maild answers ("maild is not running"), and
non-zero when maild still answers or still holds the lock after 30 seconds.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socket, err := opts.socketPath()
			if err != nil {
				return err
			}
			// The data directory, as maild resolves it; the socket is the one
			// this command dials, so no default socket is needed.
			paths, err := config.Resolve(func(k string) string {
				if k == "FROSTMAIL_SOCKET" {
					return socket
				}
				return os.Getenv(k)
			})
			if err != nil {
				return err
			}
			return runShutdown(cmd.Context(), cmd.OutOrStdout(), socket, paths.DataDir, shutdownWait)
		},
	}
}

// runShutdown stops the maild on socket and waits up to wait, in all, for it
// to stop answering rpc.hello and then to let go of dataDir's lock: a maild
// that has closed its socket can still be finishing its last transaction.
func runShutdown(ctx context.Context, w io.Writer, socket, dataDir string, wait time.Duration) error {
	c, _, err := api.Dial(ctx, socket, "mailctl shutdown "+version)
	if err != nil {
		var rpcErr *api.Error
		if errors.As(err, &rpcErr) {
			return err // something answers, but not as maild does
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		fmt.Fprintln(w, "maild is not running")
		return nil
	}
	err = c.RPC().Shutdown(ctx, &api.RPCShutdownParams{})
	_ = c.Close()
	if err != nil {
		return fmt.Errorf("rpc.shutdown: %w", err)
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	// until retries try every shutdownPoll, and says whether it succeeded
	// before the deadline; its error is the one to report.
	until := func(try func() (bool, error), timeout error) error {
		tick := time.NewTicker(shutdownPoll)
		defer tick.Stop()
		for {
			done, err := try()
			if err != nil || done {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-deadline.C:
				return timeout
			case <-tick.C:
			}
		}
	}
	err = until(func() (bool, error) { return !maildAnswers(ctx, socket), ctx.Err() },
		fmt.Errorf("maild still answers on %s after %s", socket, wait))
	if err != nil {
		return err
	}
	err = until(func() (bool, error) { return lockFree(dataDir) },
		fmt.Errorf("maild still holds the lock of %s after %s", dataDir, wait))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "maild stopped")
	return nil
}

// lockFree takes dataDir's lock without waiting and releases it at once. A
// data directory that does not exist has no holder.
func lockFree(dataDir string) (bool, error) {
	release, err := datalock.Acquire(dataDir)
	var busy *datalock.BusyError
	switch {
	case err == nil:
		release()
		return true, nil
	case errors.As(err, &busy):
		return false, nil
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	}
	return false, err
}
