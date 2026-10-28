package main

// Tests of mailctl import: an export staged by runExport goes into a second,
// empty store.

import (
	"bytes"
	"database/sql"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
	_ "modernc.org/sqlite" // the driver of the SQL these tests write with
)

// importFixture is a staged export, and the second data directory the import
// goes into: two stores, as a host's maild and the Flatpak's are.
type importFixture struct {
	export exportFixture
	data   string // this store's data directory, which does not exist yet
}

// newImportFixture stages an export of the export fixture's store, with the
// store id a maild draws at its first start, and points the environment at a
// new data directory for the import.
func newImportFixture(t *testing.T) importFixture {
	t.Helper()
	ctx := t.Context()
	f := newExportFixture(t)
	db, err := store.Open(ctx, filepath.Join(f.data, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.StoreID(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if out, err := runMailctl(t, "export", f.dir); err != nil {
		t.Fatalf("export = %v\n%s", err, out)
	}
	data := filepath.Join(t.TempDir(), "data")
	t.Setenv("FROSTMAIL_DATA_DIR", data)
	t.Setenv("FROSTMAIL_SOCKET", filepath.Join(t.TempDir(), "maild.sock")) // nothing answers
	return importFixture{export: f, data: data}
}

// snapshotTree is every file under dir and its checksum, so that a test can
// say that a directory is as it was.
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		sum, err := fileSHA256(p)
		if err != nil {
			return err
		}
		files[p] = sum
		return nil
	})
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	return files
}

// dbSum is the checksum of the data directory's database, or "" when it has
// none.
func dbSum(t *testing.T, data string) string {
	t.Helper()
	sum, err := fileSHA256(filepath.Join(data, "frostmail.db"))
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

// storeIDIn reads the store id in the database at path with SQL alone, so
// that no migration and no maild is needed; it is "" when the database has
// none.
func storeIDIn(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id string
	err = db.QueryRowContext(t.Context(), `SELECT value FROM store_info WHERE key = 'store_id'`).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !strings.Contains(err.Error(), "no such table") {
		t.Fatalf("read the store id of %s: %v", path, err)
	}
	return id
}

// execSQL writes rows with SQL alone, without the store's pragmas: SQLite
// checks a foreign key only when it is asked to, so a row can stand here
// without the account it names. That is how a store that holds one kind of
// row and nothing else is made.
func execSQL(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.ExecContext(t.Context(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// makePristine makes the data directory with the empty database that the
// app's first launch leaves: no database at all is the other way.
func makePristine(t *testing.T, data string) {
	t.Helper()
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(t.Context(), filepath.Join(data, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// addAccount puts an account in the store at data.
func addAccount(t *testing.T, data string) {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(data, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	srv := store.ServerConfig{Host: "imap.mailtest.test", Port: 993, TLS: api.TLSModeTLS, Username: "u"}
	a := store.Account{
		Kind: api.AccountKindIMAP, Email: "old@mailtest.test", Auth: api.AuthKindPassword,
		IMAP: srv, SMTP: srv,
	}
	if err := db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.InsertAccount(ctx, a)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// emails is the emails of the store's accounts, as store.Open lists them.
func emails(t *testing.T, path string) []string {
	t.Helper()
	ctx := t.Context()
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	accounts, err := db.ListAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range accounts {
		out = append(out, a.Email)
	}
	return out
}

// refusedCleanly is what a refused import must leave: no database moved
// aside, no stage directory, and the database's checksum as it was.
func refusedCleanly(t *testing.T, data, before string) {
	t.Helper()
	if m, _ := filepath.Glob(filepath.Join(data, "*before-import*")); len(m) != 0 {
		t.Errorf("the refused import moved %v aside", m)
	}
	if m, _ := filepath.Glob(filepath.Join(data, ".import-*")); len(m) != 0 {
		t.Errorf("the refused import left the stage %v", m)
	}
	if after := dbSum(t, data); after != before {
		t.Errorf("the database changed to %q, was %q", after, before)
	}
}

func TestImportIntoAnEmptyDataDirectory(t *testing.T) {
	f := newImportFixture(t)
	before := snapshotTree(t, f.export.dir)
	exportID := storeIDIn(t, filepath.Join(f.export.dir, "frostmail.db"))
	if len(exportID) != 32 {
		t.Fatalf("the export's store id = %q; want the one the fixture drew", exportID)
	}

	out, err := runMailctl(t, "import", f.export.dir)
	if err != nil {
		t.Fatalf("import = %v\n%s", err, out)
	}
	for _, want := range []string{
		"account 1: pw@mailtest.test",
		"account 2: oa@mailtest.test",
		f.export.dir + " holds the secrets in plain text (secrets.json): remove it once the accounts sync.",
		"Open Frostmail to start maild with the imported accounts.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the import's output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret-") {
		t.Errorf("the import's output prints a secret:\n%s", out)
	}

	// The store: the export's accounts, blobs, secrets and a new store id.
	want := []string{"pw@mailtest.test", "oa@mailtest.test"}
	if got := emails(t, filepath.Join(f.data, "frostmail.db")); !slices.Equal(got, want) {
		t.Errorf("the store lists %v, want %v", got, want)
	}
	if got, err := countBlobs(filepath.Join(f.data, "blobs")); err != nil || got != 2 {
		t.Errorf("the store holds %d blobs, %v; want 2", got, err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(f.data, "secrets.json"))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("secrets.json: %v, %v; want mode 0600", fi, err)
		}
	}
	source, err := secrets.NewFile(filepath.Join(f.export.dir, "secrets.json")).All()
	if err != nil {
		t.Fatal(err)
	}
	placed, err := secrets.NewFile(filepath.Join(f.data, "secrets.json")).All()
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(source, placed) {
		t.Errorf("the store's secrets.json holds %v, want the export's %v", placed, source)
	}
	if id := storeIDIn(t, filepath.Join(f.data, "frostmail.db")); id == exportID || len(id) != 32 {
		t.Errorf("the store's id = %q, want a new one, not the export's %q", id, exportID)
	}

	// The data directory holds the store and the lock, and no leftover.
	entries, err := os.ReadDir(f.data)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.Name() != "maild.lock" { // the data lock, taken and released
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	if got := strings.Join(names, " "); got != "blobs frostmail.db secrets.json" {
		t.Errorf("the data directory holds %s", got)
	}

	// The export is untouched: import never changes DIR.
	if after := snapshotTree(t, f.export.dir); !maps.Equal(before, after) {
		t.Errorf("the export changed:\n before: %v\n after: %v", before, after)
	}
}

func TestImportMovesThePristineDatabaseAside(t *testing.T) {
	f := newImportFixture(t)
	makePristine(t, f.data)
	was := dbSum(t, f.data)

	out, err := runMailctl(t, "import", f.export.dir)
	if err != nil {
		t.Fatalf("import = %v\n%s", err, out)
	}
	aside, err := filepath.Glob(filepath.Join(f.data, "frostmail.db.before-import-*Z"))
	if err != nil || len(aside) != 1 {
		t.Fatalf("the databases moved aside = %v, %v; want one", aside, err)
	}
	if !strings.Contains(out, aside[0]) {
		t.Errorf("the import does not say where the earlier database is (%s):\n%s", aside[0], out)
	}
	if sum, err := fileSHA256(aside[0]); err != nil || sum != was {
		t.Errorf("the database moved aside is %q, %v; want the empty one %q", sum, err, was)
	}
	if ok, reason, err := store.Pristine(t.Context(), aside[0]); err != nil || !ok {
		t.Errorf("the database moved aside = %v, %q, %v; want the empty store", ok, reason, err)
	}
	want := []string{"pw@mailtest.test", "oa@mailtest.test"}
	if got := emails(t, filepath.Join(f.data, "frostmail.db")); !slices.Equal(got, want) {
		t.Errorf("the store lists %v, want %v", got, want)
	}
	if m, _ := filepath.Glob(filepath.Join(f.data, ".import-*")); len(m) != 0 {
		t.Errorf("the import left the stage %v", m)
	}
}

func TestImportRefusesAndChangesNothing(t *testing.T) {
	known, err := store.KnownSchema()
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		store  func(t *testing.T, data string)
		damage func(t *testing.T, dir string)
		want   string
	}{
		"a store with an account": {func(t *testing.T, data string) {
			makePristine(t, data)
			addAccount(t, data)
		}, nil, "1 account: an import needs an empty store"},
		"a store with a message": {func(t *testing.T, data string) {
			makePristine(t, data)
			execSQL(t, filepath.Join(data, "frostmail.db"),
				`INSERT INTO messages (account_id, internal_date) VALUES (1, '2026-10-28T00:00:00.000Z')`)
		}, nil, "1 message: an import needs an empty store"},
		"a store with a draft": {func(t *testing.T, data string) {
			makePristine(t, data)
			execSQL(t, filepath.Join(data, "frostmail.db"), `INSERT INTO drafts (account_id, content_json, msgid_hdr, updated_at, created_at)
				VALUES (1, '{}', 'd@x.test', '2026-10-28T00:00:00.000Z', '2026-10-28T00:00:00.000Z')`)
		}, nil, "1 draft: an import needs an empty store"},
		"a store with an outbox row": {func(t *testing.T, data string) {
			makePristine(t, data)
			execSQL(t, filepath.Join(data, "frostmail.db"), `INSERT INTO outbox (account_id, state, send_at, blob_id, msgid_hdr, from_addr, rcpt_json, created_at, updated_at)
				VALUES (1, 'queued', '2026-10-28T00:00:00.000Z', 'blob', 'm@x.test', 'a@x.test', '[]', '2026-10-28T00:00:00.000Z', '2026-10-28T00:00:00.000Z')`)
		}, nil, "1 outbox row: an import needs an empty store"},
		"an export with a damaged database": {makePristine, func(t *testing.T, dir string) {
			p := filepath.Join(dir, "frostmail.db")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			b[len(b)-1] ^= 0xff
			if err := os.WriteFile(p, b, 0o600); err != nil {
				t.Fatal(err)
			}
		}, "does not match the manifest's checksum"},
		"an export with no MANIFEST": {makePristine, func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "MANIFEST")); err != nil {
				t.Fatal(err)
			}
		}, "MANIFEST is missing"},
		"an export with a newer schema": {makePristine, func(t *testing.T, dir string) {
			p := filepath.Join(dir, "MANIFEST")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(strings.Replace(string(b), "schema="+itoa(known), "schema="+itoa(known+1), 1)), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "install a newer maild here first"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newImportFixture(t)
			tc.store(t, f.data)
			if tc.damage != nil {
				tc.damage(t, f.export.dir)
			}
			before := dbSum(t, f.data)
			out, err := runMailctl(t, "import", f.export.dir)
			if err == nil {
				t.Fatalf("the import went through:\n%s", out)
			}
			if got := out + err.Error(); !strings.Contains(got, tc.want) {
				t.Errorf("the refusal lacks %q:\n%s\n%v", tc.want, out, err)
			}
			refusedCleanly(t, f.data, before)
		})
	}
}

// A maild that answers is asked to stop before the store changes: the import
// goes through once its socket is silent.
func TestImportAsksAMaildToStop(t *testing.T) {
	f := newImportFixture(t)
	srv := rpctest.Start(t)
	t.Setenv("FROSTMAIL_SOCKET", srv.Socket)
	out, err := runMailctl(t, "import", f.export.dir)
	if err != nil {
		t.Fatalf("import = %v\n%s", err, out)
	}
	<-srv.Stopped()
	want := []string{"pw@mailtest.test", "oa@mailtest.test"}
	if got := emails(t, filepath.Join(f.data, "frostmail.db")); !slices.Equal(got, want) {
		t.Errorf("the store lists %v, want %v", got, want)
	}
}

// A fault after the moves began puts the store back: placeImport is where
// the store changes, so it undoes its own moves.
func TestPlaceImportPutsTheStoreBack(t *testing.T) {
	data := t.TempDir()
	paths := config.Paths{DataDir: data, DB: filepath.Join(data, "frostmail.db"), Blobs: filepath.Join(data, "blobs")}
	for _, name := range []string{"frostmail.db", "frostmail.db-wal", "secrets.json"} {
		if err := os.WriteFile(filepath.Join(data, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(paths.Blobs, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Blobs, "blob"), []byte("blob"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, data)

	// A stage without its blobs: the export's first move fails.
	stage := filepath.Join(data, ".import-20261028T120000Z")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"frostmail.db", "secrets.json"} {
		if err := os.WriteFile(filepath.Join(stage, name), []byte("imported"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := placeImport(paths, stage, "20261028T120000Z")
	if err == nil || !strings.Contains(err.Error(), "the store is as it was") {
		t.Fatalf("placeImport = %v; want a fault that put the store back", err)
	}
	if err := os.RemoveAll(stage); err != nil { // as import does after a fault
		t.Fatal(err)
	}
	if after := snapshotTree(t, data); !maps.Equal(before, after) {
		t.Errorf("the store changed:\n before: %v\n after: %v", before, after)
	}
}

func TestImportHasNoDryRun(t *testing.T) {
	f := newImportFixture(t)
	out, err := runMailctl(t, "--dry-run", "import", f.export.dir)
	if err == nil || !strings.Contains(err.Error(), "no dry run") {
		t.Fatalf("import --dry-run = %v\n%s", err, out)
	}
	if _, err := os.Stat(f.data); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("import --dry-run made %s: %v", f.data, err)
	}
}

// DIR and the store must be two trees: whatever lies inside the other would
// change, and a symlink alias counts as the directory itself.
func TestImportRefusesADirectoryThatOverlapsTheStore(t *testing.T) {
	for name, tc := range map[string]struct {
		dir  func(t *testing.T, data string) string
		want string
	}{
		"the data directory": {func(t *testing.T, data string) string {
			makePristine(t, data)
			return data
		}, "is this store's data directory"},
		"inside the data directory": {func(t *testing.T, data string) string {
			makePristine(t, data)
			dir := filepath.Join(data, "blobs", "export-here")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			return dir
		}, "is inside this store's data directory"},
		"the data directory inside DIR": {func(t *testing.T, data string) string {
			return filepath.Dir(data)
		}, "export into a directory that does not hold it"},
		"a symlink alias of the data directory": {func(t *testing.T, data string) string {
			if runtime.GOOS == "windows" {
				t.Skip("Windows needs a privilege for a symlink")
			}
			makePristine(t, data)
			link := filepath.Join(t.TempDir(), "alias")
			if err := os.Symlink(data, link); err != nil {
				t.Fatal(err)
			}
			return link
		}, "is this store's data directory"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newImportFixture(t)
			dir := tc.dir(t, f.data)
			var before map[string]string
			if _, err := os.Stat(f.data); err == nil {
				before = snapshotTree(t, f.data)
			}
			out, err := runMailctl(t, "import", dir)
			if err == nil {
				t.Fatalf("the import went through:\n%s", out)
			}
			if got := out + err.Error(); !strings.Contains(got, tc.want) {
				t.Errorf("the refusal lacks %q:\n%s\n%v", tc.want, out, err)
			}
			if before != nil {
				if after := snapshotTree(t, f.data); !maps.Equal(before, after) {
					t.Errorf("the data directory changed:\n before: %v\n after: %v", before, after)
				}
			}
			if m, _ := filepath.Glob(filepath.Join(f.data, ".import-*")); len(m) != 0 {
				t.Errorf("the refused import left the stage %v", m)
			}
			if m, _ := filepath.Glob(filepath.Join(f.data, "*before-import*")); len(m) != 0 {
				t.Errorf("the refused import moved %v aside", m)
			}
		})
	}
}

// A second import in the same second takes the next stamp: it never replaces
// a name that an earlier import already moved a file aside to.
func TestImportStampIsFreeOfAnEarlierImportsNames(t *testing.T) {
	f := newImportFixture(t)
	makePristine(t, f.data)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	earlier := filepath.Join(f.data, "frostmail.db.before-import-20261028T120000Z")
	content := []byte("the database of an earlier import")
	if err := os.WriteFile(earlier, content, 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := importInto(t.Context(), &out, &rootOptions{}, f.export.dir, 5*time.Second, now); err != nil {
		t.Fatalf("import = %v\n%s", err, out.String())
	}
	if b, err := os.ReadFile(earlier); err != nil || !bytes.Equal(b, content) {
		t.Errorf("the earlier import's file = %q, %v; want it unchanged", b, err)
	}
	if _, err := os.Stat(filepath.Join(f.data, "frostmail.db.before-import-20261028T120000Z-2")); err != nil {
		t.Errorf("the aside database of this import: %v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(f.data, ".import-*")); len(m) != 0 {
		t.Errorf("the import left the stage %v", m)
	}
}

// The store's path becomes part of a SQLite URI: a ? or # in it is refused
// before the import changes or makes any file.
func TestImportRefusesAStorePathWithAQuery(t *testing.T) {
	f := newImportFixture(t)
	data := filepath.Join(t.TempDir(), "da?ta")
	t.Setenv("FROSTMAIL_DATA_DIR", data)
	out, err := runMailctl(t, "import", f.export.dir)
	if err == nil || !strings.Contains(err.Error(), "contains ? or #") {
		t.Fatalf("import = %v\n%s", err, out)
	}
	// Windows cannot name a file with ?, so Stat fails there with another
	// error than ErrNotExist: only a Stat that succeeds finds a made file.
	if _, err := os.Stat(data); err == nil {
		t.Errorf("the refused import made %s", data)
	}
}
