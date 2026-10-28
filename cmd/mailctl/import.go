package main

// Placing a host maild's export in this store (docs/plans/0013-m6-the-
// standalone-desktop-flatpak.md, Phase 7;
// docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md, Moving from a
// host maild): the export is validated as export --check does, maild is
// stopped, the data lock is held, the store must be empty, and only then do
// the export's database, blobs and secrets take the store's place. What the
// store held moves aside, never out: the import never deletes it, and it
// never changes DIR.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/datalock"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/spf13/cobra"
)

const (
	// importLockWait is how long import waits for maild to stop and for the
	// data directory's lock, the two together.
	importLockWait = 30 * time.Second
	// importStampFormat names the stage directory and what import moves
	// aside: UTC, so that the names sort, and one import's files never
	// hide another's.
	importStampFormat = "20060102T150405Z"
)

// newImportCmd builds import: place an export in this build's empty store.
func newImportCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "import DIR",
		Short: "Place the mail of an export in DIR into this build's empty store",
		Long: `Place the mail of the export in DIR into this build's store, which must hold
no account, no message, no draft and nothing in the outbox: the export's
database, blobs and secrets take its place, and the database gets a new store
id, so that no keyring item is shared with another store.

DIR is validated first, in export --check's words, and a fault stops the
import before it changes anything. The maild that answers on the socket is
stopped, and the data directory's lock is held to the end, so that no maild
runs during the import. What the store held moves aside under one stamp, as
frostmail.db.before-import-<stamp>: the import never deletes it, and it never
changes DIR. Apart from stopping maild, it works on files and needs no
running maild.

DIR holds the secrets in plain text (secrets.json): remove it once the
accounts sync. Then open Frostmail, which starts maild with the imported
accounts.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clix.DryRun {
				return errors.New("import changes the store, so it has no dry run: use export --check DIR")
			}
			return runImport(cmd.Context(), cmd.OutOrStdout(), opts, args[0], importLockWait)
		},
	}
}

// runImport places the export in dir into this store, waiting up to wait for
// maild to stop and for its data lock.
func runImport(ctx context.Context, w io.Writer, opts *rootOptions, dir string, wait time.Duration) error {
	// 1. The export first, in export --check's words: any problem stops the
	// import before it changes anything.
	m, err := checkExport(ctx, w, dir)
	if err != nil {
		return err
	}
	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return err
	}
	if filepath.Clean(dir) == filepath.Clean(paths.DataDir) {
		return fmt.Errorf("%s is this store's data directory: export into another directory", dir)
	}
	socket := opts.socket
	if socket == "" {
		socket = paths.Socket
	}
	if err := os.MkdirAll(paths.DataDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", paths.DataDir, err)
	}

	// 2. No maild runs from here on: it is stopped, and the lock stays held
	// until the import ends.
	release, err := lockStore(ctx, socket, paths.DataDir, wait)
	if err != nil {
		return err
	}
	released := false
	unlock := func() {
		if !released {
			released = true
			release()
		}
	}
	defer unlock()

	// 3. An empty store takes the import: the two must not mix.
	switch ok, reason, err := store.Pristine(ctx, paths.DB); {
	case err != nil:
		return err
	case !ok:
		return fmt.Errorf("the store in %s holds %s: an import needs an empty store, so nothing was moved", paths.DataDir, reason)
	}

	// 4. Stage first: a fault leaves the store as it was.
	stamp := time.Now().UTC().Format(importStampFormat)
	stage := filepath.Join(paths.DataDir, ".import-"+stamp)
	if err := os.Mkdir(stage, 0o700); err != nil {
		return fmt.Errorf("make the stage: %w", err)
	}
	// The stage holds copies of the mail: it goes on every path.
	defer func() { _ = os.RemoveAll(stage) }()
	stageDB := filepath.Join(stage, "frostmail.db")
	if _, err := copyFile(filepath.Join(dir, "frostmail.db"), stageDB); err != nil {
		return fmt.Errorf("copy the export's database: %w", err)
	}
	sum, err := fileSHA256(stageDB)
	if err != nil {
		return err
	}
	if sum != m.get("db_sha256") {
		return fmt.Errorf("the copy of frostmail.db does not match its manifest's checksum: %s changed during the import, so nothing was moved", dir)
	}
	// A new store id: the export's database may carry the id of the store it
	// left, whose keyring items this store must not use.
	if _, err := store.NewStoreID(ctx, stageDB); err != nil {
		return fmt.Errorf("give the imported database a new store id: %w", err)
	}
	if _, _, err := copyBlobs(filepath.Join(dir, "blobs"), filepath.Join(stage, "blobs")); err != nil {
		return err
	}
	if _, err := copyFile(filepath.Join(dir, "secrets.json"), filepath.Join(stage, "secrets.json")); err != nil {
		return fmt.Errorf("copy secrets.json: %w", err)
	}

	// 5. and 6. The store's files move aside, and the export takes their
	// place, the database last.
	asideDB, err := placeImport(paths, stage, stamp)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(stage); err != nil {
		return fmt.Errorf("the store has the import, but its stage stays: remove %s: %w", stage, err)
	}

	// 7. The lock goes, and maild may start on the imported accounts.
	unlock()
	accounts := strings.Fields(m.get("accounts"))
	fmt.Fprintf(w, "imported %d %s from %s into %s\n", len(accounts), plural(int64(len(accounts)), "account", "accounts"), dir, paths.DataDir)
	for _, a := range accounts {
		id, email, _ := strings.Cut(a, ":")
		fmt.Fprintf(w, "  account %s: %s\n", id, email)
	}
	if asideDB != "" {
		fmt.Fprintf(w, "  the store's earlier database is now %s\n", asideDB)
	}
	fmt.Fprintf(w, "%s holds the secrets in plain text (secrets.json): remove it once the accounts sync.\n", dir)
	fmt.Fprintln(w, "Open Frostmail to start maild with the imported accounts.")
	return nil
}

// lockStore stops the maild that answers on socket, then holds dataDir's
// lock and returns its release function. A maild that answers is asked to
// stop, and waited for; a lock that another process holds is tried again,
// with each such maild asked to stop again, until wait has passed in all.
func lockStore(parent context.Context, socket, dataDir string, wait time.Duration) (func(), error) {
	ctx, cancel := context.WithTimeout(parent, wait)
	defer cancel()
	tick := time.NewTicker(shutdownPoll)
	defer tick.Stop()
	var last error
	for {
		if maildAnswers(ctx, socket) {
			// A maild that answers is asked to stop, and waited for: it
			// releases its lock as it goes.
			if err := runShutdown(ctx, io.Discard, socket, dataDir, wait); err != nil {
				return nil, err
			}
		}
		release, err := datalock.Acquire(dataDir)
		var busy *datalock.BusyError
		switch {
		case err == nil:
			return release, nil
		case !errors.As(err, &busy):
			return nil, fmt.Errorf("lock %s: %w", dataDir, err)
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("the data directory %s is still locked after %s: stop the maild that holds it, then import again: %w", dataDir, wait, last)
		case <-tick.C:
		}
	}
}

// placeImport moves what the store holds aside under stamp, then moves the
// staged export into its place: the blobs, then secrets.json, then the
// database. It returns the path of the database it moved aside, which is ""
// when the store had none. A fault puts back every move it made, and its
// error says what the data directory holds then.
func placeImport(paths config.Paths, stage, stamp string) (asideDB string, err error) {
	type move struct{ from, to string }
	var aside, placed []move
	// putBack undoes the aside moves, in reverse, and returns the names it
	// could not put back.
	putBack := func() []string {
		var stuck []string
		for i := len(aside) - 1; i >= 0; i-- {
			if err := os.Rename(aside[i].to, aside[i].from); err != nil {
				stuck = append(stuck, aside[i].to)
			}
		}
		return stuck
	}
	// failed says what the data directory holds after the fault.
	failed := func(cause error, stuck []string) error {
		if len(stuck) == 0 {
			return fmt.Errorf("import failed, and the store is as it was: %w", cause)
		}
		return fmt.Errorf("import failed, and what was moved could not all be put back: %s stays in %s: %w", strings.Join(stuck, ", "), paths.DataDir, cause)
	}
	// The WAL and the shared-memory file go before the database they belong
	// to, so that no -wal is left without its database.
	for _, name := range []string{"frostmail.db-wal", "frostmail.db-shm", "frostmail.db", "blobs", "secrets.json"} {
		from := filepath.Join(paths.DataDir, name)
		to := filepath.Join(paths.DataDir, asideName(name, stamp))
		if err := os.Rename(from, to); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return "", failed(err, putBack())
		}
		aside = append(aside, move{from: from, to: to})
		if name == "frostmail.db" {
			asideDB = to
		}
	}
	for _, m := range []move{
		{from: filepath.Join(stage, "blobs"), to: paths.Blobs},
		{from: filepath.Join(stage, "secrets.json"), to: filepath.Join(paths.DataDir, "secrets.json")},
		{from: filepath.Join(stage, "frostmail.db"), to: paths.DB},
	} {
		if err := os.Rename(m.from, m.to); err != nil {
			var stuck []string
			for i := len(placed) - 1; i >= 0; i-- {
				if err := os.Rename(placed[i].to, placed[i].from); err != nil {
					stuck = append(stuck, placed[i].to)
				}
			}
			return "", failed(err, append(stuck, putBack()...))
		}
		placed = append(placed, m)
	}
	return asideDB, nil
}

// asideName is where import moves the store's file name aside, under stamp:
// the database's name comes first, and the -wal and -shm that belong to it
// follow it.
func asideName(name, stamp string) string {
	switch name {
	case "frostmail.db-wal":
		return "frostmail.db.before-import-" + stamp + "-wal"
	case "frostmail.db-shm":
		return "frostmail.db.before-import-" + stamp + "-shm"
	}
	return name + ".before-import-" + stamp
}
