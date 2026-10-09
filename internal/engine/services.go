package engine

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/discover"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/store"
)

// PIMSyncer is contacts, calendar and tasks sync as the API sees it
// (pimsync.Manager).
type PIMSyncer interface {
	// Reload starts and stops accounts' loops after a service changed.
	Reload(ctx context.Context) error
	// Poll asks for a pass of an account (0: every account); without force
	// it skips accounts polled within the last minute.
	Poll(accountID int64, force bool)
	// Kick tells an account's loop that changes are queued.
	Kick(accountID int64)
	// Discover finds a service's DAV home set from start, signing in as the
	// account.
	Discover(ctx context.Context, accountID int64, service api.ServiceKind, start string) (string, error)
	// Verify compares the account's synced collections with the servers.
	Verify(ctx context.Context, accountID int64) ([]api.CollectionCheck, error)
}

// serviceOrder is account.services' order.
func serviceOrder() []api.ServiceKind {
	return []api.ServiceKind{api.ServiceKindContacts, api.ServiceKindCalendar, api.ServiceKindTasks}
}

// serviceAvailable reports whether the account's provider offers a
// service Frostmail can reach: Microsoft has no DAV, iCloud no tasks.
func serviceAvailable(acct store.Account, s api.ServiceKind) bool {
	if acct.Kind == api.AccountKindMicrosoft {
		return false
	}
	return s != api.ServiceKindTasks || providers.ForKind(acct.Kind).DAV.Tasks != "none"
}

// googleTasks reports whether the account's tasks are Google Tasks.
func googleTasks(acct store.Account, s api.ServiceKind) bool {
	return s == api.ServiceKindTasks && providers.ForKind(acct.Kind).DAV.Tasks == "google"
}

// serviceSignedIn reports whether the account's sign-in covers a service.
func serviceSignedIn(acct store.Account, s api.ServiceKind) bool {
	if acct.Auth != api.AuthKindOAuth2 || providers.ForKind(acct.Kind).OAuth != "google" {
		return true
	}
	return acct.HasScope(oauth.GoogleScope(s))
}

func toAPIService(acct store.Account, kind api.ServiceKind, row *store.Service) api.ServiceSettings {
	out := api.ServiceSettings{Service: kind, Available: serviceAvailable(acct, kind), SignedIn: serviceSignedIn(acct, kind)}
	if row != nil {
		out.Enabled, out.URL, out.LastSyncAt = row.Enabled, row.URL, row.LastSyncAt
		if row.LastError != "" {
			out.Error = &row.LastError
		}
	}
	return out
}

func (a accounts) Services(ctx context.Context, p *api.AccountServicesParams) ([]api.ServiceSettings, error) {
	acct, err := a.DB.GetAccount(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("account %d", p.ID))
	}
	rows, err := a.DB.Services(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	out := make([]api.ServiceSettings, 0, 3)
	for _, kind := range serviceOrder() {
		var row *store.Service
		for i := range rows {
			if rows[i].Service == kind {
				row = &rows[i]
			}
		}
		out = append(out, toAPIService(acct, kind, row))
	}
	return out, nil
}

func (a accounts) service(ctx context.Context, acct store.Account, kind api.ServiceKind) (*api.ServiceSettings, error) {
	rows, err := a.DB.Services(ctx, acct.ID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Service == kind {
			s := toAPIService(acct, kind, &rows[i])
			return &s, nil
		}
	}
	s := toAPIService(acct, kind, nil)
	return &s, nil
}

func (a accounts) SetService(ctx context.Context, p *api.AccountSetServiceParams) (*api.ServiceSettings, error) {
	acct, err := a.DB.GetAccount(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("account %d", p.ID))
	}
	if !serviceAvailable(acct, p.Service) {
		return nil, api.InvalidParams("%s accounts have no %s Frostmail can reach", acct.Kind, p.Service)
	}
	current, err := a.service(ctx, acct, p.Service)
	if err != nil {
		return nil, err
	}
	if !p.Enabled {
		if current.URL != "" {
			if err := a.DB.Tx(ctx, func(tx *store.Tx) error {
				if err := tx.SetService(ctx, acct.ID, p.Service, false, current.URL); err != nil {
					return err
				}
				return relinkFor(ctx, tx, p.Service)
			}); err != nil {
				return nil, err
			}
		}
		a.afterServiceChange(ctx, acct.ID)
		return a.service(ctx, acct, p.Service)
	}
	start, home, err := a.locateService(ctx, acct, p, current.URL)
	if err != nil {
		return nil, err
	}
	err = a.DB.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetService(ctx, acct.ID, p.Service, true, start); err != nil {
			return err
		}
		if home != "" {
			if err := tx.SetServiceHome(ctx, acct.ID, p.Service, home); err != nil {
				return err
			}
		}
		return relinkFor(ctx, tx, p.Service)
	})
	if err != nil {
		return nil, err
	}
	a.afterServiceChange(ctx, acct.ID)
	return a.service(ctx, acct, p.Service)
}

// locateService decides where a service lives when it is turned on: the
// URL the user gave, the one it had, the provider's, or for other accounts
// the SRV record's server or the address's domain. It discovers the home
// set when it can sign in; a Google account without the service's scope
// waits for account.authorize, and the first pass discovers.
func (a accounts) locateService(ctx context.Context, acct store.Account, p *api.AccountSetServiceParams, had string) (start, home string, err error) {
	if googleTasks(acct, p.Service) {
		return providers.GoogleTasksURL, "", nil
	}
	var starts []string
	switch {
	case p.URL != nil && strings.TrimSpace(*p.URL) != "":
		u := strings.TrimSpace(*p.URL)
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}
		if parsed, perr := url.Parse(u); perr != nil || parsed.Host == "" {
			return "", "", api.InvalidParams("%q is not a server address", *p.URL)
		}
		starts = []string{u}
	case had != "":
		starts = []string{had}
	default:
		dav := providers.ForKind(acct.Kind).DAV
		if s := dav.Contacts; p.Service == api.ServiceKindContacts && s != "" {
			starts = []string{s}
		} else if s := dav.Calendar; p.Service != api.ServiceKindContacts && s != "" {
			starts = []string{s}
		} else if starts, err = discover.DAVStarts(ctx, a.Discovery.Resolver, acct.Email, p.Service); err != nil {
			return "", "", api.Unavailable("%v", err)
		}
	}
	if a.PIM == nil || !serviceSignedIn(acct, p.Service) {
		return starts[0], "", nil
	}
	var last error
	for _, s := range starts {
		dctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		home, err := a.PIM.Discover(dctx, acct.ID, p.Service, s)
		cancel()
		if err == nil {
			return s, home, nil
		}
		last = err
	}
	return "", "", api.Unavailable("found no %s server for %s (%v); enter its address", p.Service, acct.Email, errors.Unwrap(last))
}

// relinkFor rebuilds people when the contacts service changed: an
// account's contacts count only while it is on.
func relinkFor(ctx context.Context, tx *store.Tx, service api.ServiceKind) error {
	if service != api.ServiceKindContacts {
		return nil
	}
	return tx.RelinkPeople(ctx)
}

// afterServiceChange restarts sync for the account's services.
func (a accounts) afterServiceChange(ctx context.Context, accountID int64) {
	if a.PIM == nil {
		return
	}
	if err := a.PIM.Reload(ctx); err != nil {
		a.Log.Warn("pim sync did not pick up a service change", "err", err)
	}
	a.PIM.Poll(accountID, true)
}

func toAPICollection(c store.Collection, acctReadOnly bool) api.Collection {
	out := api.Collection{
		ID: c.ID, AccountID: c.AccountID, Kind: c.Kind, Name: c.Name, Color: c.Color,
		ReadOnly: c.ReadOnly || acctReadOnly, Enabled: c.Enabled, IsDefault: c.IsDefault,
		Events: c.Kind == api.CollectionKindCalendar && (len(c.Components) == 0 || slices.Contains(c.Components, "VEVENT")),
		Tasks: c.Kind == api.CollectionKindTasklist ||
			c.Kind == api.CollectionKindCalendar && (len(c.Components) == 0 || slices.Contains(c.Components, "VTODO")),
	}
	return out
}

func (a accounts) Collections(ctx context.Context, p *api.AccountCollectionsParams) ([]api.Collection, error) {
	f := store.CollectionFilter{}
	if p.AccountID != nil {
		f.AccountID = *p.AccountID
	}
	if p.Kind != nil {
		f.Kind = *p.Kind
	}
	list, err := a.DB.Collections(ctx, f)
	if err != nil {
		return nil, err
	}
	readOnly, err := a.readOnlyAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.Collection, 0, len(list))
	for _, c := range list {
		out = append(out, toAPICollection(c, readOnly[c.AccountID]))
	}
	return out, nil
}

func (a accounts) readOnlyAccounts(ctx context.Context) (map[int64]bool, error) {
	list, err := a.DB.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := map[int64]bool{}
	for _, acct := range list {
		out[acct.ID] = acct.ReadOnly
	}
	return out, nil
}

func (a accounts) SetCollection(ctx context.Context, p *api.AccountSetCollectionParams) (*api.Collection, error) {
	if p.IsDefault != nil && !*p.IsDefault {
		return nil, api.InvalidParams("isDefault can only be set, not cleared")
	}
	var c store.Collection
	err := a.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		c, err = tx.UpdateCollection(ctx, p.ID, p.Enabled, p.IsDefault != nil && *p.IsDefault)
		if err != nil || c.Kind != api.CollectionKindAddressbook {
			return err
		}
		return tx.RelinkPeople(ctx) // a hidden address book's people go
	})
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("collection %d", p.ID))
	}
	acct, err := a.DB.GetAccount(ctx, c.AccountID)
	if err != nil {
		return nil, err
	}
	if p.Enabled != nil && *p.Enabled && a.PIM != nil {
		a.PIM.Poll(c.AccountID, true)
	}
	out := toAPICollection(c, acct.ReadOnly)
	return &out, nil
}

func (s syncService) Pim(ctx context.Context, p *api.SyncPimParams) error {
	var id int64
	if p.AccountID != nil {
		if _, err := s.DB.GetAccount(ctx, *p.AccountID); err != nil {
			return apiError(err, "account")
		}
		id = *p.AccountID
	}
	if s.PIM != nil {
		s.PIM.Poll(id, false)
	}
	return nil
}
