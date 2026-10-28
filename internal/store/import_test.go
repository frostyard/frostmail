package store

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// emptyStore is a database this build made and closed, with nothing in it: a
// store as the app's first launch leaves one.
func emptyStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "frostmail.db")
	d, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	return path
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

func TestPristine(t *testing.T) {
	for name, tc := range map[string]struct {
		fill func(t *testing.T, path string)
		want string // "" means pristine
	}{
		"nothing": {func(*testing.T, string) {}, ""},
		"an account": {func(t *testing.T, path string) {
			d, err := Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			insertAccount(t, d, sampleAccount("a@mailtest.test"))
		}, "1 account"},
		"a message": {func(t *testing.T, path string) {
			execSQL(t, path, `INSERT INTO messages (account_id, internal_date) VALUES (1, '2026-10-28T00:00:00.000Z')`)
		}, "1 message"},
		"a draft": {func(t *testing.T, path string) {
			execSQL(t, path, `INSERT INTO drafts (account_id, content_json, msgid_hdr, updated_at, created_at)
				VALUES (1, '{}', 'd@x.test', '2026-10-28T00:00:00.000Z', '2026-10-28T00:00:00.000Z')`)
		}, "1 draft"},
		"an outbox row": {func(t *testing.T, path string) {
			execSQL(t, path, `INSERT INTO outbox (account_id, state, send_at, blob_id, msgid_hdr, from_addr, rcpt_json, created_at, updated_at)
				VALUES (1, 'queued', '2026-10-28T00:00:00.000Z', 'blob', 'm@x.test', 'a@x.test', '[]', '2026-10-28T00:00:00.000Z', '2026-10-28T00:00:00.000Z')`)
		}, "1 outbox row"},
		"all four": {func(t *testing.T, path string) {
			execSQL(t, path,
				`INSERT INTO messages (account_id, internal_date) VALUES (1, '2026-10-28T00:00:00.000Z')`,
				`INSERT INTO drafts (account_id, content_json, msgid_hdr, updated_at, created_at)
					VALUES (1, '{}', 'd@x.test', '2026-10-28T00:00:00.000Z', '2026-10-28T00:00:00.000Z')`,
				`INSERT INTO outbox (account_id, state, send_at, blob_id, msgid_hdr, from_addr, rcpt_json, created_at, updated_at)
					VALUES (1, 'queued', '2026-10-28T00:00:00.000Z', 'blob', 'm@x.test', 'a@x.test', '[]', '2026-10-28T00:00:00.000Z', '2026-10-28T00:00:00.000Z')`)
			d, err := Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			insertAccount(t, d, sampleAccount("a@mailtest.test"))
			insertAccount(t, d, sampleAccount("b@mailtest.test"))
		}, "2 accounts, 1 message, 1 draft, 1 outbox row"},
	} {
		t.Run(name, func(t *testing.T) {
			path := emptyStore(t)
			tc.fill(t, path)
			ok, reason, err := Pristine(t.Context(), path)
			if err != nil {
				t.Fatalf("Pristine = %v", err)
			}
			if tc.want == "" {
				if !ok || reason != "" {
					t.Fatalf("Pristine = %v, %q; want a pristine store", ok, reason)
				}
				return
			}
			if ok || reason != tc.want {
				t.Fatalf("Pristine = %v, %q; want %q", ok, reason, tc.want)
			}
		})
	}
}

// A store with no database is pristine, and checking it makes none: the
// first maild makes one.
func TestPristineOfAMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frostmail.db")
	ok, reason, err := Pristine(t.Context(), path)
	if err != nil || !ok || reason != "" {
		t.Fatalf("Pristine = %v, %q, %v; want a pristine store", ok, reason, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat %s = %v; want no file", path, err)
	}
}

// NewStoreID replaces the id a copy carries, so that the two stores never
// share the keyring items that the id names.
func TestNewStoreIDReplacesTheID(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "frostmail.db")
	d, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	old, err := d.StoreID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := NewStoreID(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 32 || got == old || strings.Trim(got, "0123456789abcdef") != "" {
		t.Fatalf("NewStoreID = %q, want 32 hex characters other than %q", got, old)
	}
	again, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if id, err := again.StoreID(ctx); err != nil || id != got {
		t.Fatalf("the store's id after NewStoreID = %q, %v; want %q", id, err, got)
	}
}

// A database from before migration 19 has no store_info table: NewStoreID
// writes nothing, and maild makes an id after it migrates.
func TestNewStoreIDWithoutTheTableDoesNothing(t *testing.T) {
	path := emptyStore(t)
	execSQL(t, path, `DROP TABLE store_info`) // a schema before migration 19
	id, err := NewStoreID(t.Context(), path)
	if err != nil || id != "" {
		t.Fatalf("NewStoreID = %q, %v; want no id and no error", id, err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tables int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM sqlite_master WHERE name = 'store_info'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("NewStoreID made the store_info table")
	}
}

// A WAL and a shared-memory file never stay beside a database that
// NewStoreID closed: the copy moves into the store as one file.
func TestNewStoreIDLeavesNoWAL(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "frostmail.db")
	d, err := Open(ctx, path) // the store's WAL
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStoreID(ctx, path); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path + "-wal", path + "-shm"} {
		if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s stays: %v", p, err)
		}
	}
}

// A crash can leave committed rows in the -wal file that no checkpoint has
// moved into the database. Pristine reads them, and it never checkpoints them
// away: the database and the -wal are as they were.
func TestPristineReadsACrashLeftWALWithoutChangingIt(t *testing.T) {
	ctx := t.Context()
	live := t.TempDir()
	d, err := Open(ctx, filepath.Join(live, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	insertAccount(t, d, sampleAccount("a@mailtest.test"))
	// The three files as a crash leaves them: the row is still in the -wal.
	crash := t.TempDir()
	for _, name := range []string{"frostmail.db", "frostmail.db-wal", "frostmail.db-shm"} {
		b, err := os.ReadFile(filepath.Join(live, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(crash, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(crash, "frostmail.db")
	// The -shm is SQLite's own cache and it rewrites it; the database and
	// the -wal hold the rows, and must stay as they are.
	before := map[string]string{}
	for _, name := range []string{"frostmail.db", "frostmail.db-wal"} {
		sum, err := fileSHA256Of(filepath.Join(crash, name))
		if err != nil {
			t.Fatal(err)
		}
		before[name] = sum
	}
	ok, reason, err := Pristine(ctx, path)
	if err != nil {
		t.Fatalf("Pristine = %v", err)
	}
	if ok || reason != "1 account" {
		t.Fatalf("Pristine = %v, %q; want the account in the -wal", ok, reason)
	}
	for name, was := range before {
		sum, err := fileSHA256Of(filepath.Join(crash, name))
		if err != nil {
			t.Errorf("%s after Pristine: %v", name, err)
			continue
		}
		if sum != was {
			t.Errorf("%s changed: %s, was %s", name, sum, was)
		}
	}
}

// fileSHA256Of is the checksum of a file.
func fileSHA256Of(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

// The path becomes part of a SQLite URI: a ? or # in it is refused before
// anything is opened or made.
func TestPristineAndNewStoreIDRefuseAQueryInThePath(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	for _, name := range []string{"frostmail.db?x", "frostmail.db#x"} {
		path := filepath.Join(dir, name)
		if _, _, err := Pristine(ctx, path); err == nil || !strings.Contains(err.Error(), "contains ? or #") {
			t.Errorf("Pristine(%s) = %v; want the path refused", name, err)
		}
		if _, err := NewStoreID(ctx, path); err == nil || !strings.Contains(err.Error(), "contains ? or #") {
			t.Errorf("NewStoreID(%s) = %v; want the path refused", name, err)
		}
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s was made: %v", name, err)
		}
	}
}
