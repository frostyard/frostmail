package store

// Placing an export in an empty store (mailctl import,
// docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md): Pristine says
// whether the store holds nothing yet, and NewStoreID gives an imported
// database an id of its own. Both work on files with database/sql alone, as
// Export does: neither migrates the database, and neither needs a maild.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Pristine reports whether the store at path holds nothing that an import
// would mix with: no account, no message, no draft and nothing in the
// outbox. A database that does not exist is pristine, and stays absent (the
// first maild makes it). reason says what a store that is not pristine
// holds. The database is opened as it is: not migrated, and not changed.
func Pristine(ctx context.Context, path string) (ok bool, reason string, err error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return true, "", nil
	} else if err != nil {
		return false, "", fmt.Errorf("check the store: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return false, "", fmt.Errorf("check the store: %w", err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)

	var held []string
	for _, t := range []struct{ table, one, many string }{
		{"accounts", "account", "accounts"},
		{"messages", "message", "messages"},
		{"drafts", "draft", "drafts"},
		{"outbox", "outbox row", "outbox rows"},
	} {
		var n int64
		// A store from before a table existed holds none of its rows.
		if err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM `+t.table).Scan(&n); err != nil && !strings.Contains(err.Error(), "no such table") {
			return false, "", fmt.Errorf("check the store: count %s: %w", t.table, err)
		}
		if n > 0 {
			held = append(held, fmt.Sprintf("%d %s", n, plural(n, t.one, t.many)))
		}
	}
	if len(held) == 0 {
		return true, "", nil
	}
	return false, strings.Join(held, ", "), nil
}

// NewStoreID gives the database at path a new store id: 32 hex characters,
// drawn at random and saved with INSERT OR REPLACE in store_info (ADR-0046),
// so that two stores never share keyring items. It returns "" and no error
// when the database has no store_info table, a schema from before migration
// 19: maild makes an id after it migrates. path must be a copy that no maild
// uses: it is not migrated, and no -wal or -shm file stays beside it.
func NewStoreID(ctx context.Context, path string) (string, error) {
	// DELETE rather than the store's WAL, so that the copy has no -wal file
	// to leave behind; maild sets WAL again when it opens the store.
	sqlDB, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(DELETE)")
	if err != nil {
		return "", fmt.Errorf("new store id: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	id, err := writeStoreID(ctx, sqlDB)
	if cerr := sqlDB.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("new store id: %w", err)
	}
	for _, p := range []string{path + "-wal", path + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("new store id: remove %s: %w", p, err)
		}
	}
	return id, nil
}

// plural is one or many for a count, in a reason's words.
func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func writeStoreID(ctx context.Context, sqlDB *sql.DB) (string, error) {
	var tables int
	if err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'store_info'`).Scan(&tables); err != nil {
		return "", err
	}
	if tables == 0 {
		return "", nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("draw: %w", err)
	}
	id := hex.EncodeToString(raw)
	if _, err := sqlDB.ExecContext(ctx, `INSERT OR REPLACE INTO store_info (key, value) VALUES (?, ?)`, storeIDKey, id); err != nil {
		return "", fmt.Errorf("save: %w", err)
	}
	return id, nil
}
