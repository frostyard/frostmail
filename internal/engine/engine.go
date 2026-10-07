// Package engine implements the maild API domains over the store; the sync
// engine joins it in M1 (docs/design/overview.md).
package engine

import (
	"context"
	"errors"
	"fmt"
	"net/mail"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// Engine owns the domain services.
type Engine struct{ db *store.DB }

// New returns an Engine over db.
func New(db *store.DB) *Engine { return &Engine{db: db} }

// Accounts implements the account domain.
func (e *Engine) Accounts() api.AccountService { return accounts{e.db} }

// Mailboxes implements the mailbox domain.
func (e *Engine) Mailboxes() api.MailboxService { return mailboxes{e.db} }

// apiError maps store errors to API errors.
func apiError(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return api.NotFound("%s does not exist", what)
	case errors.Is(err, store.ErrConflict):
		return api.Conflict("%s already exists", what)
	}
	return err
}

type accounts struct{ db *store.DB }

func (a accounts) List(ctx context.Context, _ *api.AccountListParams) ([]api.Account, error) {
	list, err := a.db.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.Account, 0, len(list))
	for _, acct := range list {
		out = append(out, toAPIAccount(acct))
	}
	return out, nil
}

func (a accounts) Get(ctx context.Context, p *api.AccountGetParams) (*api.Account, error) {
	acct, err := a.db.GetAccount(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("account %d", p.ID))
	}
	r := toAPIAccount(acct)
	return &r, nil
}

func (a accounts) Create(ctx context.Context, p *api.AccountCreateParams) (*api.Account, error) {
	if err := validateAccount(p); err != nil {
		return nil, err
	}
	in := store.Account{
		Kind: p.Kind, Email: p.Email, DisplayName: p.DisplayName, Auth: p.Auth,
		IMAP: toStoreServer(p.IMAP), SMTP: toStoreServer(p.SMTP),
	}
	var out store.Account
	err := a.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.InsertAccount(ctx, in)
		return err
	})
	if err != nil {
		return nil, apiError(err, "an account for "+p.Email)
	}
	r := toAPIAccount(out)
	return &r, nil
}

func (a accounts) Update(ctx context.Context, p *api.AccountUpdateParams) (*api.Account, error) {
	u := store.AccountUpdate{DisplayName: p.DisplayName}
	for _, s := range []struct {
		in  *api.ServerConfig
		out **store.ServerConfig
		key string
	}{{p.IMAP, &u.IMAP, "imap"}, {p.SMTP, &u.SMTP, "smtp"}} {
		if s.in == nil {
			continue
		}
		if err := validateServer(s.key, *s.in); err != nil {
			return nil, err
		}
		sc := toStoreServer(*s.in)
		*s.out = &sc
	}
	var out store.Account
	err := a.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.UpdateAccount(ctx, p.ID, u)
		return err
	})
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("account %d", p.ID))
	}
	r := toAPIAccount(out)
	return &r, nil
}

func (a accounts) Delete(ctx context.Context, p *api.AccountDeleteParams) error {
	err := a.db.Tx(ctx, func(tx *store.Tx) error { return tx.DeleteAccount(ctx, p.ID) })
	return apiError(err, fmt.Sprintf("account %d", p.ID))
}

func validateAccount(p *api.AccountCreateParams) error {
	if !p.Kind.Valid() {
		return api.InvalidParams("unknown account kind %q", p.Kind)
	}
	if !p.Auth.Valid() {
		return api.InvalidParams("unknown auth kind %q", p.Auth)
	}
	if addr, err := mail.ParseAddress(p.Email); err != nil || addr.Address != p.Email {
		return api.InvalidParams("email %q is not a bare address", p.Email)
	}
	if err := validateServer("imap", p.IMAP); err != nil {
		return err
	}
	return validateServer("smtp", p.SMTP)
}

func validateServer(key string, s api.ServerConfig) error {
	switch {
	case s.Host == "":
		return api.InvalidParams("%s.host is empty", key)
	case s.Port < 1 || s.Port > 65535:
		return api.InvalidParams("%s.port %d is out of range", key, s.Port)
	case !s.TLS.Valid():
		return api.InvalidParams("%s.tls %q is unknown", key, s.TLS)
	case s.Username == "":
		return api.InvalidParams("%s.username is empty", key)
	}
	return nil
}

func toAPIAccount(a store.Account) api.Account {
	return api.Account{
		ID: a.ID, Kind: a.Kind, Email: a.Email, DisplayName: a.DisplayName, Auth: a.Auth,
		IMAP: toAPIServer(a.IMAP), SMTP: toAPIServer(a.SMTP), CreatedAt: a.CreatedAt,
	}
}

func toAPIServer(s store.ServerConfig) api.ServerConfig {
	return api.ServerConfig{Host: s.Host, Port: int64(s.Port), TLS: s.TLS, Username: s.Username}
}

func toStoreServer(s api.ServerConfig) store.ServerConfig {
	return store.ServerConfig{Host: s.Host, Port: int(s.Port), TLS: s.TLS, Username: s.Username}
}

type mailboxes struct{ db *store.DB }

func (m mailboxes) List(ctx context.Context, p *api.MailboxListParams) ([]api.Mailbox, error) {
	var accountID int64
	if p.AccountID != nil {
		accountID = *p.AccountID
	}
	list, err := m.db.ListMailboxes(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]api.Mailbox, 0, len(list))
	for _, mb := range list {
		out = append(out, api.Mailbox{
			ID: mb.ID, AccountID: mb.AccountID, Path: mb.Path, Name: mb.Name,
			Delimiter: mb.Delimiter, Role: mb.Role, Total: mb.Total, Unread: mb.Unread,
		})
	}
	return out, nil
}
