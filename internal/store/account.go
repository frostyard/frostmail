package store

import (
	"context"
	"errors"
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

// errNotImplemented marks the stubs task T-0002 replaces
// (docs/tasks/todo/0002-store-accounts.md).
var errNotImplemented = errors.New("store: not implemented")

// InsertAccount inserts a, ignoring a.ID and a.CreatedAt: the row gets a new
// ID and CreatedAt = tx.Now() truncated to the millisecond. It emits
// api.AccountChanged{ID: id} and returns the stored account. A duplicate
// email (case-insensitive) returns ErrConflict.
func (t *Tx) InsertAccount(ctx context.Context, a Account) (Account, error) {
	return Account{}, errNotImplemented
}

// GetAccount returns one account, or ErrNotFound.
func (d *DB) GetAccount(ctx context.Context, id int64) (Account, error) {
	return Account{}, errNotImplemented
}

// ListAccounts returns every account ordered by ID; none is an empty slice.
func (d *DB) ListAccounts(ctx context.Context) ([]Account, error) {
	return nil, errNotImplemented
}

// UpdateAccount applies u to account id, emits api.AccountChanged{ID: id}
// and returns the updated account. A missing account returns ErrNotFound.
func (t *Tx) UpdateAccount(ctx context.Context, id int64, u AccountUpdate) (Account, error) {
	return Account{}, errNotImplemented
}

// DeleteAccount deletes account id (its mailboxes and messages cascade) and
// emits api.AccountChanged{ID: id, Deleted: true}. A missing account returns
// ErrNotFound.
func (t *Tx) DeleteAccount(ctx context.Context, id int64) error {
	return errNotImplemented
}
