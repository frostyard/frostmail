// Package mailsync keeps each account's local store in step with its IMAP
// server: one actor per account runs the reconcile pass, watches INBOX with
// IDLE, polls the other mailboxes and fetches bodies on demand
// (docs/design/sync.md).
package mailsync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// Config tunes sync. Zero fields take the defaults in brackets.
type Config struct {
	PollInterval time.Duration // STATUS polling of mailboxes IDLE does not watch [5m]
	IdleMax      time.Duration // re-issue IDLE before servers time it out [25m]
	Chunk        int           // UIDs per header fetch [500]
	MinBackoff   time.Duration // first retry after a failure [2s]
	MaxBackoff   time.Duration // longest retry interval [5m]
	// InsecureSkipVerify accepts any TLS certificate; for test servers only
	// (FROSTMAIL_INSECURE_TLS=1).
	InsecureSkipVerify bool
	// Trace, if set, receives each account's raw IMAP exchange.
	Trace func(accountID int64) io.Writer
}

func (c Config) withDefaults() Config {
	if c.PollInterval == 0 {
		c.PollInterval = 5 * time.Minute
	}
	if c.IdleMax == 0 {
		c.IdleMax = 25 * time.Minute
	}
	if c.Chunk == 0 {
		c.Chunk = 500
	}
	if c.MinBackoff == 0 {
		c.MinBackoff = 2 * time.Second
	}
	if c.MaxBackoff == 0 {
		c.MaxBackoff = 5 * time.Minute
	}
	return c
}

// Manager runs one actor per account.
type Manager struct {
	db      *store.DB
	secrets secrets.Store
	blobs   *blob.Store
	log     *slog.Logger
	cfg     Config
	publish func([]api.EventEnvelope)

	mu     sync.Mutex
	ctx    context.Context
	actors map[int64]*actor
	wg     sync.WaitGroup
}

// New returns a Manager. publish delivers transient events (sync.progress);
// durable events go through the store as usual.
func New(db *store.DB, sec secrets.Store, blobs *blob.Store, log *slog.Logger, cfg Config, publish func([]api.EventEnvelope)) *Manager {
	return &Manager{
		db: db, secrets: sec, blobs: blobs, log: log, cfg: cfg.withDefaults(), publish: publish,
		actors: map[int64]*actor{},
	}
}

// Start runs an actor for every account until ctx ends.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	return m.Reload(ctx)
}

// Wait blocks until every actor has stopped after the Start context ended.
func (m *Manager) Wait() { m.wg.Wait() }

// Reload starts actors for accounts without one and stops actors whose
// account is gone.
func (m *Manager) Reload(ctx context.Context) error {
	accounts, err := m.db.ListAccounts(ctx)
	if err != nil {
		return fmt.Errorf("reload accounts: %w", err)
	}
	var stopping []*actor
	m.mu.Lock()
	if m.ctx == nil {
		m.mu.Unlock()
		return errors.New("mailsync: Reload before Start")
	}
	live := map[int64]bool{}
	for _, a := range accounts {
		live[a.ID] = true
		if _, ok := m.actors[a.ID]; !ok {
			m.startLocked(a)
		}
	}
	for id, a := range m.actors {
		if !live[id] {
			stopping = append(stopping, a)
			delete(m.actors, id)
		}
	}
	m.mu.Unlock()
	for _, a := range stopping { // outside the lock: logout can take seconds
		a.stop()
	}
	return nil
}

// Restart stops an account's actor and starts a fresh one with the current
// settings and password.
func (m *Manager) Restart(ctx context.Context, accountID int64) error {
	acct, err := m.db.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	m.mu.Lock()
	old := m.actors[accountID]
	delete(m.actors, accountID)
	m.mu.Unlock()
	if old != nil {
		old.stop() // before the new actor connects
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx == nil {
		return errors.New("mailsync: Restart before Start")
	}
	if _, ok := m.actors[accountID]; !ok {
		m.startLocked(acct)
	}
	return nil
}

func (m *Manager) startLocked(acct store.Account) {
	ctx, cancel := context.WithCancel(m.ctx)
	a := &actor{
		m: m, acct: acct, cancel: cancel, done: make(chan struct{}),
		wake: make(chan struct{}, 1), ops: make(chan struct{}, 1), bodies: make(chan bodyRequest),
		status: api.SyncStatus{AccountID: acct.ID, Phase: api.SyncPhaseConnecting},
	}
	m.actors[acct.ID] = a
	m.wg.Go(func() {
		defer close(a.done)
		a.run(ctx)
	})
}

// Status returns the sync state of one account, or of all when accountID is 0.
func (m *Manager) Status(accountID int64) []api.SyncStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []api.SyncStatus{}
	for id, a := range m.actors {
		if accountID == 0 || id == accountID {
			out = append(out, a.getStatus())
		}
	}
	slices.SortFunc(out, func(a, b api.SyncStatus) int { return int(a.AccountID - b.AccountID) })
	return out
}

// SyncNow asks an account's actor for a full pass.
func (m *Manager) SyncNow(accountID int64) error {
	a := m.actor(accountID)
	if a == nil {
		return store.ErrNotFound
	}
	select {
	case a.wake <- struct{}{}:
	default:
	}
	return nil
}

// FetchBody makes sure a message's raw body is in the blob store, asking its
// account's actor to fetch it if needed, and returns the blob ID.
func (m *Manager) FetchBody(ctx context.Context, messageID int64) (string, error) {
	detail, err := m.db.GetMessage(ctx, messageID)
	if err != nil {
		return "", err
	}
	if detail.BlobID != "" {
		if ok, _ := m.blobs.Has(detail.BlobID); ok {
			return detail.BlobID, nil
		}
	}
	a := m.actor(detail.AccountID)
	if a == nil {
		return "", fmt.Errorf("mailsync: account %d is not running", detail.AccountID)
	}
	reply := make(chan bodyResult, 1)
	select {
	case a.bodies <- bodyRequest{id: messageID, reply: reply}:
	case <-ctx.Done():
		return "", ctx.Err()
	case <-a.done:
		return "", ErrOffline
	}
	select {
	case r := <-reply:
		return r.blobID, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// ErrOffline means the account is not connected, so a body that is not
// stored locally cannot be fetched now.
var ErrOffline = errors.New("mailsync: account is offline")

func (m *Manager) actor(id int64) *actor {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.actors[id]
}

// progress publishes a transient sync.progress event.
func (m *Manager) progress(s api.SyncStatus) {
	if m.publish == nil {
		return
	}
	env, err := api.NewEnvelope(0, api.SyncProgress{Status: s})
	if err != nil {
		m.log.Warn("encode sync progress", "err", err)
		return
	}
	m.publish([]api.EventEnvelope{env})
}
