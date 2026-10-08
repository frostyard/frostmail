package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// OAuthClientID returns the client ID stored for a provider; a missing one
// wraps ErrNotFound.
func (d *DB) OAuthClientID(ctx context.Context, provider string) (string, error) {
	var id string
	err := d.db.QueryRowContext(ctx, `SELECT client_id FROM oauth_clients WHERE provider = ?`, provider).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("oauth client %s: %w", provider, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("oauth client %s: %w", provider, err)
	}
	return id, nil
}

// SetOAuthClientID stores a provider's client ID, replacing any previous one.
func (t *Tx) SetOAuthClientID(ctx context.Context, provider, clientID string) error {
	if _, err := t.ExecContext(ctx, `INSERT INTO oauth_clients (provider, client_id) VALUES (?, ?)
		ON CONFLICT (provider) DO UPDATE SET client_id = excluded.client_id`, provider, clientID); err != nil {
		return fmt.Errorf("set oauth client %s: %w", provider, err)
	}
	return nil
}
