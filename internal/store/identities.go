package store

import (
	"context"
	"fmt"

	"github.com/frostyard/frostmail/api"
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

// AddIdentity adds an address an account also sends and receives as (an
// alias), not its default, and returns it as stored. It wraps ErrNotFound
// for an unknown account and ErrConflict for an address the account
// already has, compared without case. account.changed follows.
func (t *Tx) AddIdentity(ctx context.Context, accountID int64, name, email string) (Identity, error) {
	var n int
	if err := t.QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE id = ?`, accountID).Scan(&n); err != nil {
		return Identity{}, fmt.Errorf("add identity: %w", err)
	}
	if n == 0 {
		return Identity{}, fmt.Errorf("account %d: %w", accountID, ErrNotFound)
	}
	if err := t.QueryRowContext(ctx, `SELECT count(*) FROM identities WHERE account_id = ? AND lower(email) = lower(?)`,
		accountID, email).Scan(&n); err != nil {
		return Identity{}, fmt.Errorf("add identity: %w", err)
	}
	if n > 0 {
		return Identity{}, fmt.Errorf("identity %s of account %d: %w", email, accountID, ErrConflict)
	}
	res, err := t.ExecContext(ctx, `INSERT INTO identities (account_id, name, email) VALUES (?, ?, ?)`, accountID, name, email)
	if err != nil {
		return Identity{}, fmt.Errorf("add identity: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Identity{}, fmt.Errorf("add identity: %w", err)
	}
	if err := t.Emit(ctx, api.AccountChanged{ID: accountID}); err != nil {
		return Identity{}, err
	}
	return getIdentity(ctx, t, id)
}

// DeleteIdentity removes an identity that is not its account's default;
// the drafts that used it move to the default. It wraps ErrNotFound, and
// ErrConflict for a default identity. account.changed follows.
func (t *Tx) DeleteIdentity(ctx context.Context, id int64) error {
	cur, err := getIdentity(ctx, t, id)
	if err != nil {
		return err
	}
	if cur.IsDefault {
		return fmt.Errorf("identity %d is its account's default: %w", id, ErrConflict)
	}
	if _, err := t.ExecContext(ctx, `UPDATE drafts SET content_json = json_set(content_json, '$.identityId',
 (SELECT id FROM identities WHERE account_id = ? AND is_default = 1))
 WHERE account_id = ? AND json_extract(content_json, '$.identityId') = ?`, cur.AccountID, cur.AccountID, id); err != nil {
		return fmt.Errorf("move drafts from identity %d: %w", id, err)
	}
	if _, err := t.ExecContext(ctx, `DELETE FROM identities WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete identity %d: %w", id, err)
	}
	return t.Emit(ctx, api.AccountChanged{ID: cur.AccountID})
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
