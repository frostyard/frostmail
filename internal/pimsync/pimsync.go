// Package pimsync keeps each account's address books, calendars and task
// lists in step with its servers (docs/design/pim.md, ADR-0017): one loop
// per account with a service on, beside mail's actor and independent of
// it, polling every 5 minutes, at once on request and after a local change.
package pimsync

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
	"golang.org/x/net/publicsuffix"
)

// Config tunes sync. Zero fields take the defaults in brackets.
type Config struct {
	Interval time.Duration // between passes [5m]
	// MinGap is how recently a pass may have ended for Poll without force
	// to skip the account [1m].
	MinGap time.Duration
	Batch  int // hrefs per multiget [100]
	// HTTP sends DAV and Tasks requests; nil uses davx's default.
	HTTP *http.Client
	// AllowHTTP permits plain http:// servers, for local test servers only
	// (FROSTMAIL_INSECURE_TLS=1).
	AllowHTTP bool
	// Tokens gives OAuth accounts their access tokens (oauth.Manager); nil
	// leaves OAuth accounts unable to sign in.
	Tokens TokenSource
	// UserAgent is sent with every request.
	UserAgent string
	// Now is the clock that places the instances window; nil means
	// time.Now.
	Now func() time.Time
	// Local is the zone of floating event times; nil means time.Local.
	Local *time.Location
}

func (c Config) withDefaults() Config {
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Local == nil {
		c.Local = time.Local
	}
	if c.Interval == 0 {
		c.Interval = 5 * time.Minute
	}
	if c.MinGap == 0 {
		c.MinGap = time.Minute
	}
	if c.Batch == 0 {
		c.Batch = 100
	}
	return c
}

// TokenSource gives OAuth access tokens.
type TokenSource interface {
	AccessToken(ctx context.Context, accountID int64) (string, error)
	// Invalidate drops a token the server refused.
	Invalidate(accountID int64)
}

// Manager runs one loop per account with a service on.
type Manager struct {
	db      *store.DB
	secrets secrets.Store
	log     *slog.Logger
	cfg     Config

	windowMu sync.Mutex
	mu       sync.Mutex
	ctx      context.Context
	loops    map[int64]*loop
	wg       sync.WaitGroup
	// passes serializes passes per account: the loop's and Pass's.
	passes map[int64]*sync.Mutex
}

// New returns a Manager.
func New(db *store.DB, sec secrets.Store, log *slog.Logger, cfg Config) *Manager {
	return &Manager{db: db, secrets: sec, log: log, cfg: cfg.withDefaults(),
		loops: map[int64]*loop{}, passes: map[int64]*sync.Mutex{}}
}

// Start runs a loop for every account with a service on until ctx ends.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	return m.Reload(ctx)
}

// Wait blocks until every loop has stopped after the Start context ended.
func (m *Manager) Wait() { m.wg.Wait() }

// Reload starts loops for accounts that gained a service and stops those
// of accounts that have none left; running loops pass again at once,
// since a service or its URL may have changed.
func (m *Manager) Reload(ctx context.Context) error {
	services, err := m.db.Services(ctx, 0)
	if err != nil {
		return err
	}
	want := map[int64]bool{}
	for _, s := range services {
		if s.Enabled {
			want[s.AccountID] = true
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx == nil {
		return errors.New("pimsync: Reload before Start")
	}
	for id, l := range m.loops {
		if !want[id] {
			l.cancel()
			delete(m.loops, id)
		}
	}
	for id := range want {
		if l, ok := m.loops[id]; ok {
			l.poke()
			continue
		}
		ctx, cancel := context.WithCancel(m.ctx)
		l := &loop{m: m, accountID: id, cancel: cancel, wake: make(chan struct{}, 1)}
		m.loops[id] = l
		m.wg.Go(func() { l.run(ctx) })
	}
	return nil
}

// Poll asks for a pass now: of one account, or of every account when
// accountID is 0. Without force it skips accounts whose last pass ended
// within Config.MinGap, as the app's focus polls do.
func (m *Manager) Poll(accountID int64, force bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, l := range m.loops {
		if accountID != 0 && id != accountID {
			continue
		}
		if !force && time.Since(l.lastPass()) < m.cfg.MinGap {
			continue
		}
		l.poke()
	}
}

// Kick tells an account's loop that changes are queued (pim_ops).
func (m *Manager) Kick(accountID int64) { m.Poll(accountID, true) }

// Pass runs one pass of an account's services now and returns the first
// error; passes of one account never overlap. The loops call it; tests
// call it directly.
func (m *Manager) Pass(ctx context.Context, accountID int64) error {
	m.mu.Lock()
	mu := m.passes[accountID]
	if mu == nil {
		mu = &sync.Mutex{}
		m.passes[accountID] = mu
	}
	m.mu.Unlock()
	mu.Lock()
	defer mu.Unlock()
	acct, err := m.db.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	services, err := m.db.Services(ctx, accountID)
	if err != nil {
		return err
	}
	p := &pass{m: m, acct: acct}
	if err := p.prepareCalendar(ctx); err != nil {
		return err
	}
	var first error
	if err := p.replay(ctx, services); err != nil {
		m.log.Warn("pim changes not written", "account", accountID, "err", err)
		first = err
	}
	for _, s := range services {
		if !s.Enabled || s.URL == "" {
			continue
		}
		err := p.service(ctx, s)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		text := ""
		if err != nil {
			text = err.Error()
			m.log.Warn("pim sync failed", "account", accountID, "service", s.Service, "err", err)
			first = cmp.Or(first, err)
		}
		if err := m.db.Tx(ctx, func(tx *store.Tx) error {
			return tx.ServiceSynced(ctx, accountID, s.Service, m.db.Now(), text)
		}); err != nil {
			return err
		}
	}
	return first
}

// loop runs an account's passes.
type loop struct {
	m         *Manager
	accountID int64
	cancel    context.CancelFunc
	wake      chan struct{}

	mu   sync.Mutex
	last time.Time
}

func (l *loop) poke() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *loop) lastPass() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last
}

func (l *loop) run(ctx context.Context) {
	for {
		_ = l.m.Pass(ctx, l.accountID) // Pass logs and records failures
		l.mu.Lock()
		l.last = time.Now()
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-l.wake:
		case <-time.After(l.m.cfg.Interval):
		}
	}
}

// pass is one pass over an account's services.
type pass struct {
	m          *Manager
	acct       store.Account
	userEmails []string
	from, to   time.Time
}

// service syncs one service, retrying once with a new access token when
// the server refuses an OAuth account's.
func (p *pass) service(ctx context.Context, s store.Service) error {
	err := p.serviceOnce(ctx, s)
	if errors.Is(err, davx.ErrUnauthorized) && p.acct.Auth == api.AuthKindOAuth2 && p.m.cfg.Tokens != nil {
		p.m.cfg.Tokens.Invalidate(p.acct.ID)
		err = p.serviceOnce(ctx, s)
	}
	return err
}

func (p *pass) serviceOnce(ctx context.Context, s store.Service) error {
	if err := p.scope(s.Service); err != nil {
		return err
	}
	switch s.Service {
	case api.ServiceKindContacts:
		c, err := p.home(ctx, s, davx.AddressBooks)
		if err != nil {
			return err
		}
		return p.syncDAV(ctx, c, davx.AddressBooks, func(davx.Collection) bool { return true })
	case api.ServiceKindCalendar:
		c, err := p.home(ctx, s, davx.Calendars)
		if err != nil {
			return err
		}
		return p.syncDAV(ctx, c, davx.Calendars, func(col davx.Collection) bool {
			return len(col.Components) == 0 || hasComponent(col, "VEVENT")
		})
	case api.ServiceKindTasks:
		return nil // Google Tasks and CalDAV task lists: M4.5 Phase 4
	}
	return fmt.Errorf("pimsync: unknown service %q", s.Service)
}

// errScope means a Google account's grant lacks a service's scope: the user
// signs in again (account.authorize asks for it).
var errScope = errors.New("sign in to Google again to let Frostmail reach this service")

// scope checks that an OAuth account's grant covers the service.
func (p *pass) scope(service api.ServiceKind) error {
	if p.acct.Auth != api.AuthKindOAuth2 {
		return nil
	}
	if want := oauth.GoogleScope(service); providers.ForKind(p.acct.Kind).OAuth == "google" && !p.acct.HasScope(want) {
		return errScope
	}
	return nil
}

// home returns a client for the service's home set, discovering it from
// the service's URL and storing it the first time.
func (p *pass) home(ctx context.Context, s store.Service, kind davx.Kind) (*davx.Client, error) {
	home := s.Home
	if home == "" {
		var err error
		if home, err = p.m.discover(ctx, p, s.URL, kind); err != nil {
			return nil, fmt.Errorf("discover the %s home: %w", kind, err)
		}
		if err := p.m.db.Tx(ctx, func(tx *store.Tx) error {
			return tx.SetServiceHome(ctx, p.acct.ID, s.Service, home)
		}); err != nil {
			return nil, err
		}
	}
	return p.client(home)
}

func hasComponent(c davx.Collection, name string) bool {
	for _, comp := range c.Components {
		if comp == name {
			return true
		}
	}
	return false
}

// client returns a DAV client for a home set with the account's
// credentials.
func (p *pass) client(home string) (*davx.Client, error) {
	return davx.New(home, p.options(home))
}

func (p *pass) options(start string) davx.Options {
	return davx.Options{
		Authorization: p.authorization,
		HTTP:          p.m.cfg.HTTP,
		AllowHTTP:     p.m.cfg.AllowHTTP,
		UserAgent:     p.m.cfg.UserAgent,
		Trusted:       trusted(p.acct, start),
	}
}

// trusted is the credential rule of discovery (ADR-0017): the provider's
// own domains, or for other accounts the registrable domain discovery
// started at, such as caldav.example.com from example.com.
func trusted(acct store.Account, start string) func(*url.URL) bool {
	profile := providers.ForKind(acct.Kind)
	base := ""
	if u, err := url.Parse(start); err == nil {
		base, _ = publicsuffix.EffectiveTLDPlusOne(u.Hostname())
	}
	return func(u *url.URL) bool {
		if profile.TrustsHost(u.Hostname()) {
			return true
		}
		site, err := publicsuffix.EffectiveTLDPlusOne(u.Hostname())
		return err == nil && base != "" && site == base
	}
}

func (m *Manager) discover(ctx context.Context, p *pass, start string, kind davx.Kind) (string, error) {
	return davx.Discover(ctx, start, kind, p.options(start))
}

// Discover finds the home set of a service for an account from start (a
// provider's URL, the user's, or one SRV records gave), signing in as the
// account does. maild calls it when a service is turned on.
func (m *Manager) Discover(ctx context.Context, accountID int64, service api.ServiceKind, start string) (string, error) {
	acct, err := m.db.GetAccount(ctx, accountID)
	if err != nil {
		return "", err
	}
	p := &pass{m: m, acct: acct}
	if err := p.scope(service); err != nil {
		return "", err
	}
	kind := davx.AddressBooks
	if service != api.ServiceKindContacts {
		kind = davx.Calendars
	}
	return m.discover(ctx, p, start, kind)
}

// errSignIn means an OAuth account has no usable grant: sign in again.
var errSignIn = errors.New("sign in again")

// authorization is the account's Authorization header: Basic with the
// mail user name and password, or Bearer with an OAuth access token.
func (p *pass) authorization(ctx context.Context) (string, error) {
	if p.acct.Auth == api.AuthKindOAuth2 {
		if p.m.cfg.Tokens == nil {
			return "", fmt.Errorf("%w: maild has no OAuth sign-in", errSignIn)
		}
		tok, err := p.m.cfg.Tokens.AccessToken(ctx, p.acct.ID)
		if errors.Is(err, oauth.ErrReauth) || errors.Is(err, oauth.ErrNoClient) {
			return "", fmt.Errorf("%w: %w", errSignIn, err)
		}
		if err != nil {
			return "", err
		}
		return "Bearer " + tok, nil
	}
	password, err := p.m.secrets.Get(ctx, secrets.AccountPassword(p.acct.ID))
	if errors.Is(err, secrets.ErrNotFound) {
		return "", errors.New("no password is stored; use account.setPassword")
	}
	if err != nil {
		return "", err
	}
	user := p.acct.IMAP.Username
	if user == "" {
		user = p.acct.Email
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password)), nil
}
