package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// ExportedAccount is an account a Snapshot holds.
type ExportedAccount struct {
	ID    int64
	Email string
	OAuth bool // the account signs in with a refresh token, not a password
}

// Snapshot says what Export copied.
type Snapshot struct {
	SchemaVersion int // the highest applied migration in the copy
	Accounts      []ExportedAccount
}

// KnownSchema is the schema version this build migrates a database to.
func KnownSchema() (int, error) {
	ms, err := loadMigrations()
	if err != nil {
		return 0, err
	}
	return len(ms), nil
}

// Export copies the database at src into the new file dst with VACUUM INTO,
// for moving it to another machine (mailctl export). It does not migrate
// src, as Open does, and it changes nothing in it: the copy keeps src's
// schema, which a newer maild migrates when it opens it. maild should be
// stopped, so that the copy is a still one. dst must not exist.
func Export(ctx context.Context, src, dst string) (Snapshot, error) {
	if strings.ContainsAny(src, "?#") {
		return Snapshot{}, fmt.Errorf("export database: path %q contains ? or #", src)
	}
	if _, err := os.Stat(src); err != nil {
		return Snapshot{}, fmt.Errorf("export database: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", "file:"+src+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return Snapshot{}, fmt.Errorf("export database: %w", err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)

	var snap Snapshot
	if err := sqlDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&snap.SchemaVersion); err != nil {
		return Snapshot{}, fmt.Errorf("export database: read schema version: %w", err)
	}
	rows, err := sqlDB.QueryContext(ctx, `SELECT id, email, auth FROM accounts ORDER BY id`)
	if err != nil {
		return Snapshot{}, fmt.Errorf("export database: list accounts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var a ExportedAccount
		var auth string
		if err := rows.Scan(&a.ID, &a.Email, &auth); err != nil {
			return Snapshot{}, fmt.Errorf("export database: list accounts: %w", err)
		}
		a.OAuth = auth == "oauth2"
		snap.Accounts = append(snap.Accounts, a)
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("export database: list accounts: %w", err)
	}
	if err := rows.Close(); err != nil {
		return Snapshot{}, fmt.Errorf("export database: list accounts: %w", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `VACUUM INTO ?`, dst); err != nil {
		return Snapshot{}, fmt.Errorf("export database: copy to %s: %w", dst, err)
	}
	return snap, nil
}
