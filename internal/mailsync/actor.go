package mailsync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// actor owns one account's connections. C1 runs sync and every change;
// C2, when the server has IDLE, idles on INBOX (All Mail on Gmail) and only
// signals changes; C3 fetches the bodies the user opens (bodies.go).
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

	// gmail is set per session when the account syncs the Gmail way
	// (ADR-0012); only the IMAP loop touches it.
	gmail bool
	// fresh collects messages new to a folder synced before, for the
	// next announcement; only the IMAP loop touches it.
	fresh []int64

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

// errSignIn means an OAuth account has no usable grant: sign in again.
var errSignIn = errors.New("sign in again")

// credential returns what the account signs in with: its password, or for
// OAuth accounts a current access token (oauth true).
func (a *actor) credential(ctx context.Context) (secret string, isOAuth bool, err error) {
	if a.acct.Auth == api.AuthKindOAuth2 {
		if a.m.cfg.Tokens == nil {
			return "", true, fmt.Errorf("%w: maild has no OAuth sign-in", errSignIn)
		}
		tok, err := a.m.cfg.Tokens.AccessToken(ctx, a.acct.ID)
		if errors.Is(err, oauth.ErrReauth) || errors.Is(err, oauth.ErrNoClient) {
			return "", true, fmt.Errorf("%w: %w", errSignIn, err)
		}
		return tok, true, err
	}
	password, err := a.m.secrets.Get(ctx, secrets.AccountPassword(a.acct.ID))
	if errors.Is(err, secrets.ErrNotFound) {
		return "", false, errNoPassword
	}
	return password, false, err
}

// refreshed returns a new access token after the server refused one, or
// the error that ends the attempt.
func (a *actor) refreshed(ctx context.Context) (string, error) {
	a.m.cfg.Tokens.Invalidate(a.acct.ID)
	tok, _, err := a.credential(ctx)
	return tok, err
}

// refused records that the server refused the account's credential, so the
// account shows "Sign in again".
func (a *actor) refused(ctx context.Context) {
	err := a.m.db.Tx(context.WithoutCancel(ctx), func(tx *store.Tx) error { return tx.SetNeedsReauth(ctx, a.acct.ID, true) })
	if err != nil {
		a.m.log.Warn("mark account for sign-in", "account", a.acct.ID, "err", err)
	}
}

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
		case errors.Is(err, imapx.ErrAuth), errors.Is(err, errNoPassword), errors.Is(err, errSignIn):
			a.refused(ctx)
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
		}
	}
}

func (a *actor) dialOptions(secret string, isOAuth bool) imapx.DialOptions {
	s := a.acct.IMAP
	opts := imapx.DialOptions{
		Host: s.Host, Port: s.Port, TLS: s.TLS, Username: s.Username, Password: secret, OAuth: isOAuth,
		ReadOnly: a.acct.ReadOnly, InsecureSkipVerify: a.m.cfg.InsecureSkipVerify,
		Log: a.m.log.With("account", a.acct.ID),
	}
	return opts
}

// traced is dialOptions for C1, the connection traces record.
func (a *actor) traced(opts imapx.DialOptions) imapx.DialOptions {
	if a.m.cfg.Trace != nil {
		opts.Trace = a.m.cfg.Trace(a.acct.ID)
	}
	return opts
}

// connected runs one connected session. healthy reports whether it got as
// far as a completed pass, which resets the retry backoff.
func (a *actor) connected(ctx context.Context) (healthy bool, err error) {
	secret, isOAuth, err := a.credential(ctx)
	if err != nil {
		return false, err
	}
	opts := a.dialOptions(secret, isOAuth)
	a.phase(api.SyncPhaseConnecting, "")
	cmd, err := imapx.Open(ctx, a.traced(opts))
	if errors.Is(err, imapx.ErrAuth) && isOAuth {
		// Usually an expired access token: refresh once and try again.
		if opts.Password, err = a.refreshed(ctx); err != nil {
			return false, err
		}
		cmd, err = imapx.Open(ctx, a.traced(opts))
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = cmd.Close() }()
	a.gmail = providers.ForKind(a.acct.Kind).Quirks.Gmail && cmd.Caps.Gmail

	if err := a.replay(ctx, cmd); err != nil {
		return false, err
	}
	mailboxes, err := a.fullPass(ctx, cmd)
	if err != nil {
		return false, err
	}
	// Ops that needed the mailbox list (a Sent copy on a new account).
	if err := a.replay(ctx, cmd); err != nil {
		return false, err
	}
	if err := a.settleInterrupted(ctx); err != nil {
		return false, err
	}
	if err := a.saveDrafts(ctx, cmd); err != nil {
		return false, err
	}
	saveAt := a.nextDraftSave(ctx, true)
	a.settled(ctx)

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dirty := make(chan struct{}, 1)
	idleErr := make(chan error, 1)
	watched := a.watched(mailboxes)
	if !cmd.Caps.Idle {
		watched = nil
	}
	if watched != nil {
		go a.idleLoop(sctx, opts, watched.Path, dirty, idleErr)
	}
	every := a.m.cfg.PollInterval
	if a.gmail {
		every = min(every, a.m.cfg.GmailPoll)
	}
	poll := time.NewTicker(every)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		case err := <-idleErr:
			return true, fmt.Errorf("idle connection: %w", err)
		case <-dirty:
			if err := a.refresh(ctx, cmd, *watched); err != nil {
				return true, err
			}
			a.settled(ctx)
		case <-poll.C:
			if err := a.pollPass(ctx, cmd, mailboxes, watched); err != nil {
				return true, err
			}
			a.settled(ctx)
		case <-a.wake:
			if err := a.replay(ctx, cmd); err != nil {
				return true, err
			}
			if mailboxes, err = a.fullPass(ctx, cmd); err != nil {
				return true, err
			}
			a.settled(ctx)
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
			a.settled(ctx)
		}
	}
}

// settled announces the new mail of the work just done and goes idle.
func (a *actor) settled(ctx context.Context) {
	a.announceNew(ctx)
	a.idlePhase()
}

func (a *actor) idlePhase() {
	now := time.Now().UTC()
	a.setStatus(func(s *api.SyncStatus) {
		s.Phase, s.Mailbox, s.Done, s.Total, s.Error = api.SyncPhaseIdle, nil, 0, 0, nil
		s.LastSyncAt = &now
	})
}

// watched is the mailbox C2 idles on: All Mail for Gmail, else INBOX.
func (a *actor) watched(mbs []store.Mailbox) *store.Mailbox {
	if a.gmail {
		return byRole(mbs, api.MailboxRoleAll)
	}
	return byRole(mbs, api.MailboxRoleInbox)
}

// refresh brings a mailbox up to date; on Gmail that is a Gmail pass, which
// any change to a label or synced folder needs.
func (a *actor) refresh(ctx context.Context, cmd *imapx.Session, mb store.Mailbox) error {
	if a.gmail {
		return a.gmailPass(ctx, cmd)
	}
	return a.reconcile(ctx, cmd, mb)
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
		if mailboxes, err = tx.ReplaceMailboxes(ctx, a.acct.ID, list); err != nil || !a.gmail {
			return err
		}
		return tx.MarkGmailLabels(ctx, a.acct.ID)
	})
	if err != nil {
		return nil, err
	}
	if a.gmail {
		return mailboxes, a.gmailPass(ctx, cmd)
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
// whose state moved, skipping the one C2 watches. Gmail polls differently
// (gmailPoll).
func (a *actor) pollPass(ctx context.Context, cmd *imapx.Session, mailboxes []store.Mailbox, watched *store.Mailbox) error {
	if a.gmail {
		return a.gmailPoll(ctx, cmd, mailboxes)
	}
	for _, mb := range mailboxes {
		if watched != nil && mb.ID == watched.ID {
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

// idleLoop keeps C2 in IDLE on path and signals dirty after every wake-up,
// whether from an unsolicited response or the IdleMax restart (a spare
// reconcile pass then takes the fast path).
func (a *actor) idleLoop(ctx context.Context, opts imapx.DialOptions, path string, dirty chan<- struct{}, errc chan<- error) {
	wake := make(chan struct{}, 1)
	opts.OnUpdate = func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	opts = a.traced(opts)
	s, err := imapx.Open(ctx, opts)
	if err != nil {
		errc <- err
		return
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Select(ctx, path); err != nil {
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
