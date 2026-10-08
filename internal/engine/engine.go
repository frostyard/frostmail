// Package engine implements the maild API domains over the store; the sync
// engine joins it in M1 (docs/design/overview.md).
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/discover"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/render"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/view"
)

// Syncer is the sync engine as the API sees it (mailsync.Manager).
type Syncer interface {
	Reload(ctx context.Context) error
	Restart(ctx context.Context, accountID int64) error
	Status(accountID int64) []api.SyncStatus
	SyncNow(accountID int64) error
	FetchBody(ctx context.Context, messageID int64) (string, error)
	SetFlags(ctx context.Context, ids []int64, c store.FlagChange) error
	Move(ctx context.Context, ids []int64, mailboxID int64) error
	Delete(ctx context.Context, ids []int64) error
	// Kick, OutboxChanged and DraftsChanged wake an account's IMAP actor
	// (queued ops), sender (queued mail) and draft saver.
	Kick(accountID int64)
	OutboxChanged(accountID int64)
	DraftsChanged(accountID int64)
}

// Deps are what the engine's domains use. Sync, Views and Render may be nil
// in tests; their methods then report unavailable.
type Deps struct {
	DB      *store.DB
	Secrets secrets.Store
	Log     *slog.Logger
	Sync    Syncer
	Blobs   *blob.Store
	Views   *view.Manager
	Render  *render.Renderer
	// UndoDelay is how long draft.send holds a message [DefaultUndoDelay].
	UndoDelay time.Duration
	// Discovery is how account.discover reaches the network; zero means
	// the real HTTPS and DNS.
	Discovery discover.Deps
	// OAuth signs accounts in; nil makes account.authorize unavailable.
	OAuth *oauth.Manager
}

// Engine owns the domain services.
type Engine struct{ d Deps }

// New returns an Engine.
func New(d Deps) *Engine { return &Engine{d: d} }

// Accounts implements the account domain.
func (e *Engine) Accounts() api.AccountService { return accounts{e.d} }

// Mailboxes implements the mailbox domain.
func (e *Engine) Mailboxes() api.MailboxService { return mailboxes{e.d.DB} }

// Messages implements the message domain.
func (e *Engine) Messages() api.MessageService { return messages{e.d} }

// Drafts implements the draft domain.
func (e *Engine) Drafts() api.DraftService { return drafts{e.d} }

// Outbox implements the outbox domain.
func (e *Engine) Outbox() api.OutboxService { return outbox{e.d} }

// Addresses implements the address domain.
func (e *Engine) Addresses() api.AddressService { return addresses{e.d} }

// Identities implements the identity domain.
func (e *Engine) Identities() api.IdentityService { return identities{e.d} }

// Threads implements the thread domain.
func (e *Engine) Threads() api.ThreadService { return threads{e.d} }

// Sync implements the sync domain.
func (e *Engine) Sync() api.SyncService { return syncService{e.d} }

// Views implements the view domain.
func (e *Engine) Views() api.ViewService { return views{e.d} }

// afterAccountChange tells sync about an account change; failures are
// logged, since the change itself is committed.
func (d Deps) afterAccountChange(ctx context.Context, restart int64) {
	if d.Sync == nil {
		return
	}
	var err error
	if restart != 0 {
		err = d.Sync.Restart(ctx, restart)
	} else {
		err = d.Sync.Reload(ctx)
	}
	if err != nil {
		d.Log.Warn("sync did not pick up an account change", "err", err)
	}
}

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

type accounts struct{ Deps }

func (a accounts) List(ctx context.Context, _ *api.AccountListParams) ([]api.Account, error) {
	list, err := a.DB.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.Account, 0, len(list))
	for _, acct := range list {
		out = append(out, a.toAPI(ctx, acct))
	}
	return out, nil
}

func (a accounts) Get(ctx context.Context, p *api.AccountGetParams) (*api.Account, error) {
	acct, err := a.DB.GetAccount(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("account %d", p.ID))
	}
	r := a.toAPI(ctx, acct)
	return &r, nil
}

func (a accounts) Create(ctx context.Context, p *api.AccountCreateParams) (*api.Account, error) {
	imapCfg, smtpCfg, err := serversFor(p)
	if err != nil {
		return nil, err
	}
	in := store.Account{
		Kind: p.Kind, Email: p.Email, DisplayName: p.DisplayName, Auth: p.Auth,
		IMAP: toStoreServer(imapCfg), SMTP: toStoreServer(smtpCfg),
		ReadOnly: p.ReadOnly != nil && *p.ReadOnly, Notify: p.Notify == nil || *p.Notify,
	}
	var out store.Account
	err = a.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.InsertAccount(ctx, in)
		return err
	})
	if err != nil {
		return nil, apiError(err, "an account for "+p.Email)
	}
	a.afterAccountChange(ctx, 0)
	r := a.toAPI(ctx, out)
	return &r, nil
}

func (a accounts) Update(ctx context.Context, p *api.AccountUpdateParams) (*api.Account, error) {
	u := store.AccountUpdate{DisplayName: p.DisplayName, ReadOnly: p.ReadOnly, Notify: p.Notify}
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
	err := a.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.UpdateAccount(ctx, p.ID, u)
		return err
	})
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("account %d", p.ID))
	}
	a.afterAccountChange(ctx, p.ID)
	r := a.toAPI(ctx, out)
	return &r, nil
}

func (a accounts) Delete(ctx context.Context, p *api.AccountDeleteParams) error {
	err := a.DB.Tx(ctx, func(tx *store.Tx) error { return tx.DeleteAccount(ctx, p.ID) })
	if err != nil {
		return apiError(err, fmt.Sprintf("account %d", p.ID))
	}
	// Best effort: account IDs are never reused, so a leftover secret is inert.
	for _, key := range []string{secrets.AccountPassword(p.ID), secrets.RefreshToken(p.ID)} {
		if err := a.Secrets.Delete(ctx, key); err != nil {
			a.Log.Warn("account deleted but a secret was not", "account", p.ID, "key", key, "err", err)
		}
	}
	a.afterAccountChange(ctx, 0)
	return nil
}

func (a accounts) SetPassword(ctx context.Context, p *api.AccountSetPasswordParams) error {
	if p.Password == "" {
		return api.InvalidParams("password is empty")
	}
	if _, err := a.DB.GetAccount(ctx, p.ID); err != nil {
		return apiError(err, fmt.Sprintf("account %d", p.ID))
	}
	if err := a.Secrets.Set(ctx, secrets.AccountPassword(p.ID), p.Password); err != nil {
		return err
	}
	if err := a.DB.Tx(ctx, func(tx *store.Tx) error { return tx.SetNeedsReauth(ctx, p.ID, false) }); err != nil {
		return err
	}
	a.afterAccountChange(ctx, p.ID)
	return nil
}

// serversFor validates a create request and returns its servers: as given,
// or from the kind's provider profile when left out (with the address as
// the user name).
func serversFor(p *api.AccountCreateParams) (imapCfg, smtpCfg api.ServerConfig, err error) {
	if !p.Kind.Valid() {
		return imapCfg, smtpCfg, api.InvalidParams("unknown account kind %q", p.Kind)
	}
	if !p.Auth.Valid() {
		return imapCfg, smtpCfg, api.InvalidParams("unknown auth kind %q", p.Auth)
	}
	if addr, err := mail.ParseAddress(p.Email); err != nil || addr.Address != p.Email {
		return imapCfg, smtpCfg, api.InvalidParams("email %q is not a bare address", p.Email)
	}
	prof := providers.ForKind(p.Kind)
	if !slices.Contains(prof.Auth, p.Auth) {
		return imapCfg, smtpCfg, api.InvalidParams("%s accounts do not sign in with %s", p.Kind, p.Auth)
	}
	pick := func(key string, given *api.ServerConfig, def providers.Server) (api.ServerConfig, error) {
		if given != nil {
			return *given, validateServer(key, *given)
		}
		if def.Host == "" {
			return api.ServerConfig{}, api.InvalidParams("%s is required for %s accounts", key, p.Kind)
		}
		return api.ServerConfig{Host: def.Host, Port: int64(def.Port), TLS: def.TLS, Username: p.Email}, nil
	}
	if imapCfg, err = pick("imap", p.IMAP, prof.IMAP); err != nil {
		return imapCfg, smtpCfg, err
	}
	smtpCfg, err = pick("smtp", p.SMTP, prof.SMTP)
	return imapCfg, smtpCfg, err
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

// toAPI converts an account; signedIn needs a look in the secret store.
func (a accounts) toAPI(ctx context.Context, acct store.Account) api.Account {
	r := api.Account{
		ID: acct.ID, Kind: acct.Kind, Email: acct.Email, DisplayName: acct.DisplayName, Auth: acct.Auth,
		IMAP: toAPIServer(acct.IMAP), SMTP: toAPIServer(acct.SMTP), CreatedAt: acct.CreatedAt,
		ReadOnly: acct.ReadOnly, Notify: acct.Notify,
	}
	if !acct.NeedsReauth && a.Secrets != nil {
		_, err := a.Secrets.Get(ctx, secrets.Credential(acct.ID, acct.Auth == api.AuthKindOAuth2))
		r.SignedIn = err == nil
	}
	return r
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
