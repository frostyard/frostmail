package store

import (
	"context"
	"fmt"
)

// Identity is an address an account sends as, with its signature.
type Identity struct {
	ID            int64
	AccountID     int64
	Name          string
	Email         string
	ReplyTo       string
	SignatureHTML string
	IsDefault     bool
}

// IdentityUpdate changes the non-nil fields of an identity.
type IdentityUpdate struct {
	Name          *string
	ReplyTo       *string
	SignatureHTML *string
}

const identityCols = `id, account_id, name, email, reply_to, signature_html, is_default`

// ListIdentities lists an account's identities (every account's when
// accountID is 0), by account, the default first, then by ID.
func (d *DB) ListIdentities(ctx context.Context, accountID int64) ([]Identity, error) {
	query := `SELECT ` + identityCols + ` FROM identities`
	var args []any
	if accountID != 0 {
		query += ` WHERE account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY account_id, is_default DESC, id`
	return queryIdentities(ctx, d.db, query, args...)
}

// GetIdentity returns one identity; a missing one wraps ErrNotFound.
func (d *DB) GetIdentity(ctx context.Context, id int64) (Identity, error) {
	return getIdentity(ctx, d.db, id)
}

// DefaultIdentity returns an account's default identity, or its first one
// when none is marked default; an account without identities wraps
// ErrNotFound.
func (d *DB) DefaultIdentity(ctx context.Context, accountID int64) (Identity, error) {
	list, err := queryIdentities(ctx, d.db, `SELECT `+identityCols+` FROM identities
		WHERE account_id = ? ORDER BY is_default DESC, id LIMIT 1`, accountID)
	if err != nil {
		return Identity{}, err
	}
	if len(list) == 0 {
		return Identity{}, fmt.Errorf("identity of account %d: %w", accountID, ErrNotFound)
	}
	return list[0], nil
}

// UpdateIdentity changes an identity and returns it as stored; a missing one
// wraps ErrNotFound.
func (t *Tx) UpdateIdentity(ctx context.Context, id int64, u IdentityUpdate) (Identity, error) {
	cur, err := getIdentity(ctx, t, id)
	if err != nil {
		return Identity{}, err
	}
	if u.Name != nil {
		cur.Name = *u.Name
	}
	if u.ReplyTo != nil {
		cur.ReplyTo = *u.ReplyTo
	}
	if u.SignatureHTML != nil {
		cur.SignatureHTML = *u.SignatureHTML
	}
	if _, err := t.ExecContext(ctx, `UPDATE identities SET name = ?, reply_to = ?, signature_html = ? WHERE id = ?`,
		cur.Name, cur.ReplyTo, cur.SignatureHTML, id); err != nil {
		return Identity{}, fmt.Errorf("update identity %d: %w", id, err)
	}
	return cur, nil
}

func getIdentity(ctx context.Context, q querier, id int64) (Identity, error) {
	list, err := queryIdentities(ctx, q, `SELECT `+identityCols+` FROM identities WHERE id = ?`, id)
	if err != nil {
		return Identity{}, err
	}
	if len(list) == 0 {
		return Identity{}, fmt.Errorf("identity %d: %w", id, ErrNotFound)
	}
	return list[0], nil
}

func queryIdentities(ctx context.Context, q querier, query string, args ...any) ([]Identity, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("identities: %w", err)
	}
	defer rows.Close()
	var out []Identity
	for rows.Next() {
		var i Identity
		if err := rows.Scan(&i.ID, &i.AccountID, &i.Name, &i.Email, &i.ReplyTo, &i.SignatureHTML, &i.IsDefault); err != nil {
			return nil, fmt.Errorf("identities: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
