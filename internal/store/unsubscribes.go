package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// UnsubscribeKey is how an unsubscribe is remembered: by List-Id, else by
// message (ADR-0027).
func UnsubscribeKey(listID string, messageID int64) string {
	if id := strings.ToLower(strings.TrimSpace(listID)); id != "" {
		return "list:" + id
	}
	return fmt.Sprintf("message:%d", messageID)
}

// Unsubscribed reports whether the user unsubscribed under key.
func (d *DB) Unsubscribed(ctx context.Context, key string) (bool, error) {
	var at string
	err := d.db.QueryRowContext(ctx, `SELECT at FROM unsubscribes WHERE key = ?`, key).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("unsubscribed: %w", err)
	}
	return true, nil
}

// RecordUnsubscribe remembers an unsubscribe under key.
func (t *Tx) RecordUnsubscribe(ctx context.Context, key string) error {
	if _, err := t.ExecContext(ctx, `INSERT INTO unsubscribes (key, at) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET at = excluded.at`, key, FormatTime(t.Now())); err != nil {
		return fmt.Errorf("record unsubscribe: %w", err)
	}
	return nil
}
