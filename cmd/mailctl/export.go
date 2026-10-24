package main

// Moving the mail to another machine (docs/plans/0011-m8-mail-in-lodge.md,
// Phase 7): export copies the stopped maild's database, blobs and secrets
// into a directory; export --check says whether this build can import it.
// Both work offline, on files, and never call maild over RPC.

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/spf13/cobra"
)

// exportFormat is the version of an export directory's layout.
const exportFormat = 1

// newExportCmd builds export: copy the mail to a directory to move it, or
// check such a directory.
func newExportCmd(opts *rootOptions) *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "export DIR",
		Short: "Copy the database, blobs and secrets to DIR, to move the mail to another machine",
		Long: `Copy the stopped maild's database, message blobs and secrets into the new
directory DIR (mode 0700): frostmail.db, blobs/, secrets.json and MANIFEST.
It works on files and never calls maild; it refuses while maild answers on
the socket, so that the copy is a still one (systemctl --user stop
maild.service). DIR holds the secrets in plain text: remove it after the move.

With --check, validate an export against this build, changing nothing: the
manifest, the database's checksum, its schema, the blob count and the
secrets file.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if check {
				return runExportCheck(cmd.Context(), cmd.OutOrStdout(), args[0])
			}
			if clix.DryRun {
				return errors.New("export makes a copy, so it has no dry run: use export --check DIR on an existing export")
			}
			return runExport(cmd.Context(), cmd.OutOrStdout(), opts, args[0])
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "validate the export in DIR against this build instead of making one")
	return cmd
}

// runExport makes the export in dir, which must not exist. It removes what
// it made when it fails, so that a partial export never holds secrets.
func runExport(ctx context.Context, w io.Writer, opts *rootOptions, dir string) (err error) {
	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return err
	}
	socket := opts.socket
	if socket == "" {
		socket = paths.Socket
	}
	if _, err := os.Lstat(dir); err == nil {
		return fmt.Errorf("%s exists: export into a new directory", dir)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(paths.DB); err != nil {
		return fmt.Errorf("no database to export: %w", err)
	}
	if maildAnswers(ctx, socket) {
		return fmt.Errorf("maild answers on %s: stop it first (systemctl --user stop maild.service), so that the export is a still copy", socket)
	}

	if err := os.Mkdir(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()

	dbCopy := filepath.Join(dir, "frostmail.db")
	snap, err := store.Export(ctx, paths.DB, dbCopy)
	if err != nil {
		return err
	}
	// VACUUM INTO makes the file with the process's umask: mail is private.
	if err := os.Chmod(dbCopy, 0o600); err != nil {
		return err
	}
	blobs, blobBytes, err := copyBlobs(paths.Blobs, filepath.Join(dir, "blobs"))
	if err != nil {
		return err
	}
	found, missing, err := exportSecrets(ctx, w, filepath.Join(paths.DataDir, "secrets.json"), filepath.Join(dir, "secrets.json"), snap.Accounts)
	if err != nil {
		return err
	}
	sum, err := fileSHA256(dbCopy)
	if err != nil {
		return err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return fmt.Errorf("make an export id: %w", err)
	}
	host, herr := os.Hostname()
	if herr != nil || host == "" {
		host = "unknown"
	}
	var accounts []string
	for _, a := range snap.Accounts {
		accounts = append(accounts, fmt.Sprintf("%d:%s", a.ID, a.Email))
	}
	manifest := fmt.Sprintf("format=%d\nid=%s\ncreated=%s\nhost=%s\nschema=%d\ndb_sha256=%s\nblobs=%d\nblob_bytes=%d\naccounts=%s\nsecrets=%d\n",
		exportFormat, hex.EncodeToString(id), store.FormatTime(time.Now()), host, snap.SchemaVersion, sum,
		blobs, blobBytes, strings.Join(accounts, " "), found)
	// Last: a partial export has no manifest.
	if err := os.WriteFile(filepath.Join(dir, "MANIFEST"), []byte(manifest), 0o600); err != nil {
		return fmt.Errorf("write the manifest: %w", err)
	}

	fmt.Fprintf(w, "exported to %s\n", dir)
	for _, a := range snap.Accounts {
		fmt.Fprintf(w, "  account %d: %s\n", a.ID, a.Email)
	}
	for _, a := range missing {
		fmt.Fprintf(w, "warning: account %d (%s) has no stored credential: it will need Sign In Again after the move\n", a.ID, a.Email)
	}
	fmt.Fprintf(w, "  %d blobs (%d bytes), %d secrets, schema %d\n", blobs, blobBytes, found, snap.SchemaVersion)
	fmt.Fprintf(w, "%s holds the secrets in plain text (secrets.json, mode 0600): remove it after the move.\n", dir)
	return nil
}

// maildAnswers reports whether a maild answers on socket.
func maildAnswers(ctx context.Context, socket string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c, _, err := api.Dial(ctx, socket, "mailctl export "+version)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// copyBlobs copies the blob store at src into the new directory dst
// (directories 0700, files 0600) and counts its files and bytes. A missing
// src is no blobs. Files of a Put that never finished are left behind.
func copyBlobs(src, dst string) (files, bytes int64, err error) {
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return 0, 0, os.Mkdir(dst, 0o700)
	}
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.Mkdir(target, 0o700)
		case strings.HasPrefix(d.Name(), ".put-"):
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("copy blobs: %s is not a regular file", p)
		}
		n, err := copyFile(p, target)
		files++
		bytes += n
		return err
	})
	if err != nil {
		return 0, 0, fmt.Errorf("copy blobs: %w", err)
	}
	return files, bytes, nil
}

// copyFile copies src to the new file dst with mode 0600.
func copyFile(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return n, err
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("checksum %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// exportSecrets reads the secrets the exported accounts need from the
// store maild would use (file is its plain file) and writes the found ones
// to the new plain file dst, mode 0600. It returns how many it wrote and
// the accounts that have no credential. It never prints a value.
func exportSecrets(ctx context.Context, w io.Writer, file, dst string, accounts []store.ExportedAccount) (int, []store.ExportedAccount, error) {
	reader, err := secrets.OpenReader(ctx, file)
	if err != nil {
		return 0, nil, fmt.Errorf("open secrets: %w", err)
	}
	// An empty file first, so that an export without secrets still has one.
	if err := os.WriteFile(dst, []byte("{}\n"), 0o600); err != nil {
		return 0, nil, fmt.Errorf("write secrets: %w", err)
	}
	out := secrets.NewFile(dst)
	have := map[string]bool{}
	copyKey := func(key string) error {
		v, err := reader.Get(ctx, key)
		if errors.Is(err, secrets.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read secret %s: %w", key, err)
		}
		if err := out.Set(ctx, key, v); err != nil {
			return fmt.Errorf("write secret %s: %w", key, err)
		}
		have[key] = true
		return nil
	}
	for _, a := range accounts {
		for _, key := range []string{secrets.AccountPassword(a.ID), secrets.RefreshToken(a.ID)} {
			if err := copyKey(key); err != nil {
				return 0, nil, err
			}
		}
	}
	for _, p := range []api.OAuthProvider{api.OAuthProviderGoogle, api.OAuthProviderMicrosoft} {
		if err := copyKey(secrets.OAuthClientSecret(string(p))); err != nil {
			return 0, nil, err
		}
	}
	var missing []store.ExportedAccount
	for _, a := range accounts {
		if !have[secrets.Credential(a.ID, a.OAuth)] {
			missing = append(missing, a)
		}
	}
	return len(have), missing, nil
}

// manifestFile is a parsed MANIFEST.
type manifestFile struct {
	values map[string]string
}

func (m manifestFile) get(key string) string { return m.values[key] }

func (m manifestFile) int(key string) (int64, error) {
	n, err := strconv.ParseInt(m.values[key], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("MANIFEST %s=%q is not a number", key, m.values[key])
	}
	return n, nil
}

func readManifest(path string) (manifestFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return manifestFile{}, fmt.Errorf("MANIFEST is missing or unreadable (a partial export?): %w", err)
	}
	defer f.Close()
	m := manifestFile{values: map[string]string{}}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			m.values[k] = v
		}
	}
	return m, sc.Err()
}

// runExportCheck validates the export in dir against this build: it changes
// nothing, and fails with every problem it found.
func runExportCheck(ctx context.Context, w io.Writer, dir string) error {
	m, err := readManifest(filepath.Join(dir, "MANIFEST"))
	if err != nil {
		return err
	}
	if m.get("format") != strconv.Itoa(exportFormat) {
		return fmt.Errorf("MANIFEST format is %q, not %d: this mailctl cannot read the export", m.get("format"), exportFormat)
	}
	var problems []string
	problem := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
		fmt.Fprintf(w, "problem: %s\n", problems[len(problems)-1])
	}
	ok := func(format string, args ...any) { fmt.Fprintf(w, "ok: %s\n", fmt.Sprintf(format, args...)) }
	ok("MANIFEST: format %d, export %s from %s, made %s", exportFormat, m.get("id"), m.get("host"), m.get("created"))

	// The database: the checksum, then the schema.
	sum, err := fileSHA256(filepath.Join(dir, "frostmail.db"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		problem("frostmail.db is missing")
	case err != nil:
		problem("%v", err)
	case sum != m.get("db_sha256"):
		problem("frostmail.db does not match the manifest's checksum: the copy is damaged or changed")
	default:
		ok("frostmail.db matches its checksum")
	}
	schema, serr := m.int("schema")
	known, kerr := store.KnownSchema()
	switch {
	case serr != nil:
		problem("%v", serr)
	case kerr != nil:
		return kerr
	case schema > int64(known):
		problem("the export's schema is %d, newer than this maild's %d: install a newer maild here first", schema, known)
	default:
		ok("schema %d (this build migrates up to %d)", schema, known)
	}

	// The blobs.
	want, berr := m.int("blobs")
	if berr != nil {
		problem("%v", berr)
	} else if got, err := countBlobs(filepath.Join(dir, "blobs")); err != nil {
		problem("%v", err)
	} else if got != want {
		problem("blobs/ holds %d files, the manifest says %d", got, want)
	} else {
		ok("%d blobs", got)
	}

	// The secrets.
	secretsFile := filepath.Join(dir, "secrets.json")
	var held map[string]string
	fi, err := os.Stat(secretsFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		problem("secrets.json is missing")
	case err != nil:
		problem("%v", err)
	case runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0:
		problem("secrets.json has mode 0%o, want 0600: it holds secrets in plain text", fi.Mode().Perm())
	default:
		if held, err = secrets.NewFile(secretsFile).All(); err != nil {
			problem("secrets.json does not parse: %v", err)
		} else {
			ok("secrets.json: %d keys", len(held))
		}
	}

	// The accounts, and the ones that will need Sign In Again.
	var listed []string
	var warnings []string
	for _, field := range strings.Fields(m.get("accounts")) {
		listed = append(listed, field)
		idStr, email, _ := strings.Cut(field, ":")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			problem("MANIFEST accounts: %q is not id:email", field)
			continue
		}
		if held == nil {
			continue
		}
		_, pw := held[secrets.AccountPassword(id)]
		_, rt := held[secrets.RefreshToken(id)]
		if !pw && !rt {
			warnings = append(warnings, fmt.Sprintf("warning: account %d (%s) has no credential in secrets.json: it will need Sign In Again after the move", id, email))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("the export in %s is not usable here: %d problem(s)", dir, len(problems))
	}
	fmt.Fprintf(w, "accounts: %s\n", strings.Join(listed, " "))
	for _, line := range warnings {
		fmt.Fprintln(w, line)
	}
	return nil
}

// countBlobs counts the files under dir; a missing dir holds none.
func countBlobs(dir string) (int64, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	var n int64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && !strings.HasPrefix(d.Name(), ".put-") {
			n++
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("count blobs: %w", err)
	}
	return n, nil
}
