package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/frostyard/frostmail/api"
)

// Account is an accounts row.
type Account struct {
	ID          int64
	Kind        api.AccountKind
	Email       string
	DisplayName string
	Auth        api.AuthKind
	IMAP        ServerConfig
	SMTP        ServerConfig
	CreatedAt   time.Time
}

// ServerConfig is one server's connection settings.
type ServerConfig struct {
	Host     string
	Port     int
	TLS      api.TLSMode
	Username string
}

// AccountUpdate changes the non-nil fields of an account.
type AccountUpdate struct {
	DisplayName *string
	IMAP        *ServerConfig
	SMTP        *ServerConfig
}

const accountColumns = `SELECT id, kind, email, display_name, auth,
	imap_host, imap_port, imap_tls, imap_username,
	smtp_host, smtp_port, smtp_tls, smtp_username, created_at
	FROM accounts`

func scanAccount(row interface{ Scan(...any) error }) (Account, error) {
	var a Account
	var createdAt string
	err := row.Scan(&a.ID, &a.Kind, &a.Email, &a.DisplayName, &a.Auth,
		&a.IMAP.Host, &a.IMAP.Port, &a.IMAP.TLS, &a.IMAP.Username,
		&a.SMTP.Host, &a.SMTP.Port, &a.SMTP.TLS, &a.SMTP.Username,
		&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("scan account: %w", err)
	}
	a.CreatedAt, err = ParseTime(createdAt)
	if err != nil {
		return Account{}, err
	}
	return a, nil
}

// InsertAccount inserts a, ignoring a.ID and a.CreatedAt: the row gets a new
// ID and CreatedAt = tx.Now() truncated to the millisecond. It emits
// api.AccountChanged{ID: id} and returns the stored account. A duplicate
// email (case-insensitive) returns ErrConflict.
func (t *Tx) InsertAccount(ctx context.Context, a Account) (Account, error) {
	createdAt := t.Now().UTC().Truncate(time.Millisecond)
	res, err := t.ExecContext(ctx, `INSERT INTO accounts (
		kind, email, display_name, auth,
		imap_host, imap_port, imap_tls, imap_username,
		smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Kind, a.Email, a.DisplayName, a.Auth,
		a.IMAP.Host, a.IMAP.Port, a.IMAP.TLS, a.IMAP.Username,
		a.SMTP.Host, a.SMTP.Port, a.SMTP.TLS, a.SMTP.Username,
		FormatTime(createdAt))
	if err != nil {
		if IsUniqueViolation(err) {
			return Account{}, ErrConflict
		}
		return Account{}, fmt.Errorf("insert account: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Account{}, fmt.Errorf("insert account: %w", err)
	}
	a.ID = id
	a.CreatedAt = createdAt
	if err := t.Emit(ctx, api.AccountChanged{ID: id}); err != nil {
		return Account{}, err
	}
	return a, nil
}

// GetAccount returns one account, or ErrNotFound.
func (d *DB) GetAccount(ctx context.Context, id int64) (Account, error) {
	return scanAccount(d.db.QueryRowContext(ctx, accountColumns+` WHERE id = ?`, id))
}

// ListAccounts returns every account ordered by ID; none is an empty slice.
func (d *DB) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := d.db.QueryContext(ctx, accountColumns+` ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateAccount applies u to account id, emits api.AccountChanged{ID: id}
// and returns the updated account. A missing account returns ErrNotFound.
func (t *Tx) UpdateAccount(ctx context.Context, id int64, u AccountUpdate) (Account, error) {
	a, err := scanAccount(t.QueryRowContext(ctx, accountColumns+` WHERE id = ?`, id))
	if err != nil {
		return Account{}, err
	}
	if u.DisplayName != nil {
		a.DisplayName = *u.DisplayName
	}
	if u.IMAP != nil {
		a.IMAP = *u.IMAP
	}
	if u.SMTP != nil {
		a.SMTP = *u.SMTP
	}
	_, err = t.ExecContext(ctx, `UPDATE accounts SET
		display_name = ?,
		imap_host = ?, imap_port = ?, imap_tls = ?, imap_username = ?,
		smtp_host = ?, smtp_port = ?, smtp_tls = ?, smtp_username = ?
		WHERE id = ?`,
		a.DisplayName,
		a.IMAP.Host, a.IMAP.Port, a.IMAP.TLS, a.IMAP.Username,
		a.SMTP.Host, a.SMTP.Port, a.SMTP.TLS, a.SMTP.Username,
		id)
	if err != nil {
		if IsUniqueViolation(err) {
			return Account{}, ErrConflict
		}
		return Account{}, fmt.Errorf("update account: %w", err)
	}
	if err := t.Emit(ctx, api.AccountChanged{ID: id}); err != nil {
		return Account{}, err
	}
	return a, nil
}

// DeleteAccount deletes account id (its mailboxes and messages cascade) and
// emits api.AccountChanged{ID: id, Deleted: true}. A missing account returns
// ErrNotFound.
func (t *Tx) DeleteAccount(ctx context.Context, id int64) error {
	res, err := t.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return t.Emit(ctx, api.AccountChanged{ID: id, Deleted: true})
}
