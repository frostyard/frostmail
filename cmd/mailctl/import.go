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
	"os/exec"
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
	// importStartWait is how long the Flatpak's launcher gets to start maild
	// again after the import: its own start waits about 10 s for maild.
	importStartWait = 60 * time.Second
	// flatpakLauncher is the Flatpak's launcher, whose --start-maild starts
	// maild, as the Server pane's return to local mode does.
	flatpakLauncher = "/app/bin/frostmail"
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
accounts sync. In the Flatpak, import starts maild again with the imported
accounts, through the launcher (/app/bin/frostmail --start-maild), so open
windows reconnect; elsewhere, open Frostmail to start maild. An import that
fails after it stopped maild starts it again too.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clix.DryRun {
				return errors.New("import changes the store, so it has no dry run: use export --check DIR")
			}
			return runImport(cmd.Context(), cmd.OutOrStdout(), opts, args[0], importLockWait, maildStarter(os.Getenv))
		},
	}
}

// runImport places the export in dir into this store, waiting up to wait for
// maild to stop and for its data lock. startMaild, when not nil, starts maild
// again after the import, and after a fault that stopped it.
func runImport(ctx context.Context, w io.Writer, opts *rootOptions, dir string, wait time.Duration, startMaild startMaildFunc) error {
	return importInto(ctx, w, opts, dir, wait, time.Now(), startMaild)
}

// startMaildFunc starts maild again after an import stopped it, so that a
// window that was open reconnects. It is nil where nothing starts maild.
type startMaildFunc func(ctx context.Context) error

// maildStarter is the start of this machine: the Flatpak's launcher, and
// nothing on the host, which starts maild with its own service manager.
func maildStarter(getenv func(string) string) startMaildFunc {
	if !config.InFlatpak(getenv) {
		return nil
	}
	return startFlatpakMaild
}

// startFlatpakMaild runs the launcher's --start-maild, which starts maild in
// a new sandbox and waits for it: exit 0 means maild answers. Its last line
// of output says why when it fails.
func startFlatpakMaild(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, importStartWait)
	defer cancel()
	out, err := exec.CommandContext(ctx, flatpakLauncher, "--start-maild").CombinedOutput()
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
		return fmt.Errorf("%s --start-maild did not finish within %s: %w", flatpakLauncher, importStartWait, err)
	}
	if last := lastLine(string(out)); last != "" {
		return fmt.Errorf("%s --start-maild: %s: %w", flatpakLauncher, last, err)
	}
	return fmt.Errorf("%s --start-maild: %w", flatpakLauncher, err)
}

// lastLine is the last non-empty line of s, or "".
func lastLine(s string) string {
	var last string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			last = line
		}
	}
	return last
}

// importInto is runImport with the clock of the import's stamp, so that a
// test can name the files an import leaves.
func importInto(ctx context.Context, w io.Writer, opts *rootOptions, dir string, wait time.Duration, now time.Time, startMaild startMaildFunc) (err error) {
	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return err
	}
	// The store's path becomes part of a SQLite URI.
	if strings.ContainsAny(paths.DB, "?#") {
		return fmt.Errorf("the store's path %q contains ? or #, which SQLite reads as part of a query: set FROSTMAIL_DATA_DIR to a path without them", paths.DB)
	}
	// The store and DIR must be two trees: import reads all of DIR, and it
	// moves and removes files in the store.
	if err := separateStore(dir, paths.DataDir); err != nil {
		return err
	}

	// 1. The export, in export --check's words: any problem stops the import
	// before it changes anything.
	m, err := checkExport(ctx, w, dir)
	if err != nil {
		return err
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
	release, stopped, err := lockStore(ctx, socket, paths.DataDir, wait)
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
	// A fault leaves the store as it was. The maild that this import stopped
	// starts again once the lock is free, so that a window that was open
	// reconnects: a maild that did not run before stays stopped, and the
	// start's own fault changes neither the store nor this error.
	defer func() {
		unlock()
		if err == nil || !stopped || startMaild == nil {
			return
		}
		if startErr := startMaild(context.WithoutCancel(ctx)); startErr != nil {
			fmt.Fprintln(w, startErr)
			fmt.Fprintln(w, "Open Frostmail to start maild again.")
			return
		}
		fmt.Fprintln(w, "maild started again; open windows reconnect.")
	}()

	// 3. An empty store takes the import: the two must not mix.
	switch ok, reason, err := store.Pristine(ctx, paths.DB); {
	case err != nil:
		return err
	case !ok:
		return fmt.Errorf("the store in %s holds %s: an import needs an empty store, so nothing was moved", paths.DataDir, reason)
	}

	// 4. Stage first: a fault leaves the store as it was. The stamp names the
	// stage and what moves aside, and it is free of an earlier import's names.
	stamp, err := importStamp(paths.DataDir, now)
	if err != nil {
		return err
	}
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
	// maild starts on the imported accounts now, where this machine has a
	// launcher to start it with: an open window reconnects by itself.
	var startErr error
	if startMaild != nil {
		startErr = startMaild(ctx)
	}
	switch {
	case startMaild == nil:
		fmt.Fprintln(w, "Open Frostmail to start maild with the imported accounts.")
	case startErr != nil:
		fmt.Fprintln(w, startErr)
		fmt.Fprintln(w, "Open Frostmail to start maild with the imported accounts.")
	default:
		fmt.Fprintln(w, "maild started with the imported accounts; open windows reconnect.")
	}
	return nil
}

// lockStore stops the maild that answers on socket, then holds dataDir's
// lock and returns its release function. A maild that answers is asked to
// stop, and waited for; a lock that another process holds is tried again,
// with each such maild asked to stop again, until wait has passed in all.
// stopped says that a maild answered and was asked to stop, so that the
// caller can start it again.
func lockStore(parent context.Context, socket, dataDir string, wait time.Duration) (release func(), stopped bool, err error) {
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
				return nil, stopped, err
			}
			stopped = true
		}
		release, err := datalock.Acquire(dataDir)
		var busy *datalock.BusyError
		switch {
		case err == nil:
			return release, stopped, nil
		case !errors.As(err, &busy):
			return nil, stopped, fmt.Errorf("lock %s: %w", dataDir, err)
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, stopped, fmt.Errorf("the data directory %s is still locked after %s: stop the maild that holds it, then import again: %w", dataDir, wait, last)
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

// separateStore refuses a dir that overlaps this store's data directory:
// import reads all of DIR, and it moves and removes files in the store, so
// the two must be two trees. Symlinks are resolved, so an alias of the data
// directory counts as the directory itself. It changes nothing.
func separateStore(dir, dataDir string) error {
	d, err := resolvePath(dir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dir, err)
	}
	data, err := resolvePath(dataDir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dataDir, err)
	}
	switch {
	case d == data:
		return fmt.Errorf("%s is this store's data directory: export into another directory", dir)
	case within(d, data):
		return fmt.Errorf("%s is inside this store's data directory (%s): export into a directory outside it", dir, dataDir)
	case within(data, d):
		return fmt.Errorf("this store's data directory (%s) is inside %s: export into a directory that does not hold it", dataDir, dir)
	}
	return nil
}

// within reports whether path is dir itself, or inside it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

// resolvePath is p as an absolute path whose symlinks are resolved. Only the
// part of p that exists is resolved: a missing tail cannot hold a symlink,
// and EvalSymlinks fails on a path that does not exist.
func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rest := ""
	for cur := abs; ; cur = filepath.Dir(cur) {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(real, rest), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur { // nothing of the path exists
			return abs, nil
		}
		rest = filepath.Join(filepath.Base(cur), rest)
	}
}

// importStamp is the stamp of this import's stage and aside names at now:
// the UTC second, and -2, -3 and so on when that second already named files
// in dataDir, so that an import never replaces an earlier import's files.
func importStamp(dataDir string, now time.Time) (string, error) {
	base := now.UTC().Format(importStampFormat)
	for i := 1; i <= 100; i++ {
		stamp := base
		if i > 1 {
			stamp = fmt.Sprintf("%s-%d", base, i)
		}
		taken, err := stampTaken(dataDir, stamp)
		if err != nil {
			return "", err
		}
		if !taken {
			return stamp, nil
		}
	}
	return "", fmt.Errorf("%s holds 100 sets of before-import names of %s: remove the ones you no longer need, then import again", dataDir, base)
}

// stampTaken reports whether the stage directory or any file that import
// moves aside already carries stamp, so that os.Rename would replace it.
func stampTaken(dataDir, stamp string) (bool, error) {
	names := []string{".import-" + stamp}
	for _, name := range []string{"frostmail.db", "frostmail.db-wal", "frostmail.db-shm", "blobs", "secrets.json"} {
		names = append(names, asideName(name, stamp))
	}
	for _, name := range names {
		_, err := os.Lstat(filepath.Join(dataDir, name))
		switch {
		case err == nil:
			return true, nil
		case !errors.Is(err, fs.ErrNotExist):
			return false, fmt.Errorf("check %s: %w", name, err)
		}
	}
	return false, nil
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
