package mailsync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// actor owns one account's connections. C1 runs every command; C2, when the
// server has IDLE, idles on INBOX and only signals changes.
type actor struct {
	m      *Manager
	acct   store.Account
	cancel context.CancelFunc
	done   chan struct{}
	wake   chan struct{} // a full pass
	ops    chan struct{} // offline actions are queued
	outbox chan struct{} // a message was queued for sending
	drafts chan struct{} // a draft changed
	bodies chan bodyRequest

	// unsaved holds drafts whose copy the server refused, by the updated_at
	// that was refused; only the IMAP loop touches it.
	unsaved map[int64]time.Time

	mu     sync.Mutex
	status api.SyncStatus
}

type bodyRequest struct {
	id    int64
	reply chan bodyResult
}

type bodyResult struct {
	blobID string
	err    error
}

// errNoPassword means no password is stored for the account.
var errNoPassword = errors.New("no password is stored; use account.setPassword")

func (a *actor) stop() {
	a.cancel()
	<-a.done
}

func (a *actor) getStatus() api.SyncStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

// setStatus changes the status and publishes it.
func (a *actor) setStatus(f func(*api.SyncStatus)) {
	a.mu.Lock()
	f(&a.status)
	s := a.status
	a.mu.Unlock()
	a.m.progress(s)
}

func (a *actor) phase(p api.SyncPhase, errText string) {
	a.setStatus(func(s *api.SyncStatus) {
		s.Phase, s.Mailbox, s.Done, s.Total = p, nil, 0, 0
		s.Error = nil
		if errText != "" {
			s.Error = &errText
		}
	})
}

// run connects and syncs until ctx ends, retrying failures with backoff.
// A rejected login waits for a restart (a new password) or a sync request.
func (a *actor) run(ctx context.Context) {
	backoff := a.m.cfg.MinBackoff
	for {
		healthy, err := a.connected(ctx)
		if ctx.Err() != nil {
			return
		}
		if healthy {
			backoff = a.m.cfg.MinBackoff
		}
		a.m.log.Warn("sync stopped", "account", a.acct.ID, "err", err)
		var retry <-chan time.Time
		switch {
		case errors.Is(err, imapx.ErrAuth), errors.Is(err, errNoPassword):
			a.phase(api.SyncPhaseUnauthorized, err.Error())
		default:
			a.phase(api.SyncPhaseOffline, err.Error())
			retry = time.After(backoff)
			backoff = min(backoff*2, a.m.cfg.MaxBackoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-retry:
		case <-a.wake:
		case req := <-a.bodies:
			req.reply <- bodyResult{err: ErrOffline}
		}
	}
}

func (a *actor) dialOptions(password string) imapx.DialOptions {
	s := a.acct.IMAP
	opts := imapx.DialOptions{
		Host: s.Host, Port: s.Port, TLS: s.TLS, Username: s.Username, Password: password,
		InsecureSkipVerify: a.m.cfg.InsecureSkipVerify,
	}
	if a.m.cfg.Trace != nil {
		opts.Trace = a.m.cfg.Trace(a.acct.ID)
	}
	return opts
}

// connected runs one connected session. healthy reports whether it got as
// far as a completed pass, which resets the retry backoff.
func (a *actor) connected(ctx context.Context) (healthy bool, err error) {
	password, err := a.m.secrets.Get(ctx, secrets.AccountPassword(a.acct.ID))
	if errors.Is(err, secrets.ErrNotFound) {
		return false, errNoPassword
	}
	if err != nil {
		return false, err
	}
	opts := a.dialOptions(password)
	a.phase(api.SyncPhaseConnecting, "")
	cmd, err := imapx.Open(ctx, opts)
	if err != nil {
		return false, err
	}
	defer func() { _ = cmd.Close() }()

	if err := a.replay(ctx, cmd); err != nil {
		return false, err
	}
	mailboxes, err := a.fullPass(ctx, cmd)
	if err != nil {
		return false, err
	}
	if err := a.settleInterrupted(ctx); err != nil {
		return false, err
	}
	if err := a.saveDrafts(ctx, cmd); err != nil {
		return false, err
	}
	saveAt := a.nextDraftSave(ctx, true)
	a.idlePhase()

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	inboxDirty := make(chan struct{}, 1)
	idleErr := make(chan error, 1)
	watchingInbox := cmd.Caps.Idle && inbox(mailboxes) != nil
	if watchingInbox {
		go a.idleLoop(sctx, opts, inboxDirty, idleErr)
	}
	poll := time.NewTicker(a.m.cfg.PollInterval)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		case err := <-idleErr:
			return true, fmt.Errorf("idle connection: %w", err)
		case <-inboxDirty:
			if mb := inbox(mailboxes); mb != nil {
				if err := a.reconcile(ctx, cmd, *mb); err != nil {
					return true, err
				}
				a.idlePhase()
			}
		case <-poll.C:
			if err := a.pollPass(ctx, cmd, mailboxes, watchingInbox); err != nil {
				return true, err
			}
			a.idlePhase()
		case <-a.wake:
			if err := a.replay(ctx, cmd); err != nil {
				return true, err
			}
			if mailboxes, err = a.fullPass(ctx, cmd); err != nil {
				return true, err
			}
			a.idlePhase()
		case <-a.ops:
			if err := a.replay(ctx, cmd); err != nil {
				return true, err
			}
		case <-a.drafts:
			saveAt = a.nextDraftSave(ctx, false)
		case <-saveAt:
			if err := a.saveDrafts(ctx, cmd); err != nil {
				return true, err
			}
			saveAt = a.nextDraftSave(ctx, true)
			a.idlePhase()
		case req := <-a.bodies:
			id, err := a.fetchBody(ctx, cmd, req.id)
			req.reply <- bodyResult{blobID: id, err: err}
			if err != nil && !errors.Is(err, store.ErrNotFound) && !errors.Is(err, imapx.ErrNoMessage) {
				return true, err
			}
		}
	}
}

func (a *actor) idlePhase() {
	now := time.Now().UTC()
	a.setStatus(func(s *api.SyncStatus) {
		s.Phase, s.Mailbox, s.Done, s.Total, s.Error = api.SyncPhaseIdle, nil, 0, 0, nil
		s.LastSyncAt = &now
	})
}

func inbox(mbs []store.Mailbox) *store.Mailbox {
	for i := range mbs {
		if mbs[i].Role == api.MailboxRoleInbox {
			return &mbs[i]
		}
	}
	return nil
}

// fullPass lists mailboxes, mirrors them into the store, and reconciles each
// selectable one, INBOX first.
func (a *actor) fullPass(ctx context.Context, cmd *imapx.Session) ([]store.Mailbox, error) {
	a.phase(api.SyncPhaseListing, "")
	list, err := cmd.List(ctx)
	if err != nil {
		return nil, err
	}
	selectable := map[string]bool{}
	for _, mb := range list {
		selectable[mb.Path] = mb.Selectable
	}
	var mailboxes []store.Mailbox
	err = a.m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		mailboxes, err = tx.ReplaceMailboxes(ctx, a.acct.ID, list)
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, mb := range mailboxes { // ListMailboxes order: INBOX, drafts, sent, …
		if !selectable[mb.Path] {
			continue
		}
		if err := a.reconcile(ctx, cmd, mb); err != nil {
			return nil, err
		}
	}
	return mailboxes, nil
}

// pollPass checks every selectable mailbox with STATUS and reconciles those
// whose state moved. INBOX is skipped while C2 watches it.
func (a *actor) pollPass(ctx context.Context, cmd *imapx.Session, mailboxes []store.Mailbox, skipInbox bool) error {
	for _, mb := range mailboxes {
		if skipInbox && mb.Role == api.MailboxRoleInbox {
			continue
		}
		st, ok, err := a.m.db.MailboxSyncState(ctx, mb.ID)
		if err != nil {
			return err
		}
		status, err := cmd.Status(ctx, mb.Path)
		if err != nil {
			continue // \Noselect and vanished mailboxes; the next full pass relists
		}
		if ok && status.UIDValidity == st.UIDValidity && status.UIDNext == st.UIDNext &&
			status.Messages == st.ServerCount && status.HighestModSeq == st.HighestModSeq && cmd.Caps.CondStore {
			continue
		}
		if err := a.reconcile(ctx, cmd, mb); err != nil {
			return err
		}
	}
	return nil
}

// idleLoop keeps C2 in IDLE on INBOX and signals dirty after every wake-up,
// whether from an unsolicited response or the IdleMax restart (a spare
// reconcile pass then takes the fast path).
func (a *actor) idleLoop(ctx context.Context, opts imapx.DialOptions, dirty chan<- struct{}, errc chan<- error) {
	wake := make(chan struct{}, 1)
	opts.OnUpdate = func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	opts.Trace = nil
	s, err := imapx.Open(ctx, opts)
	if err != nil {
		errc <- err
		return
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Select(ctx, "INBOX"); err != nil {
		errc <- err
		return
	}
	// Mail that arrived between C1's pass and this SELECT is already in
	// the SELECT result, so no unsolicited EXISTS will report it: ask for
	// one reconcile now (the fast path makes it cheap when nothing came).
	select {
	case dirty <- struct{}{}:
	default:
	}
	for {
		if err := s.Idle(ctx, wake, a.m.cfg.IdleMax, nil); err != nil {
			if ctx.Err() == nil {
				errc <- err
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		select {
		case dirty <- struct{}{}:
		default:
		}
	}
}
