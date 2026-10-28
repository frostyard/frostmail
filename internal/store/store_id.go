package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

const storeIDKey = "store_id"

// StoreID returns the id of this database: 32 hex characters, drawn at
// random the first time it is asked for and kept in the database after that
// (ADR-0046). The Flatpak's maild names its keyring items with it. No client
// sees it, so it emits no event.
func (d *DB) StoreID(ctx context.Context) (string, error) {
	id, err := d.readStoreID(ctx)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return id, err
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("store id: draw: %w", err)
	}
	err = d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO store_info (key, value) VALUES (?, ?)`, storeIDKey, hex.EncodeToString(raw))
		return err
	})
	if err != nil {
		return "", fmt.Errorf("store id: save: %w", err)
	}
	return d.readStoreID(ctx)
}

func (d *DB) readStoreID(ctx context.Context) (string, error) {
	var id string
	err := d.db.QueryRowContext(ctx, `SELECT value FROM store_info WHERE key = ?`, storeIDKey).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("store id: read: %w", err)
	}
	return id, nil
}
