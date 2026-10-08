package mailsync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// Bodies come over a connection of their own (C3), so reading a message
// never waits behind a sync pass on C1: a first sync can take minutes,
// and what the user opens comes first (docs/design/sync.md). C3 opens when
// a body is wanted, examines mailboxes read-only (bodies are fetched with
// BODY.PEEK and change nothing), and closes after BodyIdle without
// requests.

// bodyLoop answers body requests until ctx ends.
func (a *actor) bodyLoop(ctx context.Context) {
	var s *imapx.Session
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()
	for {
		var expire <-chan time.Time
		if s != nil {
			expire = time.After(a.m.cfg.BodyIdle)
		}
		select {
		case <-ctx.Done():
			return
		case <-expire:
			_ = s.Close()
			s = nil
		case req := <-a.bodies:
			var id string
			var err error
			s, id, err = a.serveBody(ctx, s, req.id)
			req.reply <- bodyResult{blobID: id, err: err}
		}
	}
}

// serveBody fetches one body on s, opening C3 when needed and once more
// when the open connection turns out to be dead. It returns the session
// to keep, nil when there is none.
func (a *actor) serveBody(ctx context.Context, s *imapx.Session, msgID int64) (*imapx.Session, string, error) {
	for attempt := 0; ; attempt++ {
		if s == nil {
			var err error
			if s, err = a.openBodies(ctx); err != nil {
				return nil, "", fmt.Errorf("%w: %w", ErrOffline, err)
			}
		}
		id, err := a.fetchBody(ctx, s, msgID)
		var imapErr *imap.Error
		if err == nil || errors.Is(err, store.ErrNotFound) || errors.Is(err, imapx.ErrNoMessage) || errors.As(err, &imapErr) {
			return s, id, err
		}
		// The connection failed (it may have timed out while idle).
		_ = s.Close()
		s = nil
		if attempt > 0 || ctx.Err() != nil {
			return nil, "", err
		}
	}
}

// openBodies opens C3: read-only, refreshing an OAuth token once if the
// server refuses it.
func (a *actor) openBodies(ctx context.Context) (*imapx.Session, error) {
	secret, isOAuth, err := a.credential(ctx)
	if err != nil {
		return nil, err
	}
	opts := a.dialOptions(secret, isOAuth)
	opts.ReadOnly = true
	s, err := imapx.Open(ctx, opts)
	if errors.Is(err, imapx.ErrAuth) && isOAuth {
		if opts.Password, err = a.refreshed(ctx); err != nil {
			return nil, err
		}
		s, err = imapx.Open(ctx, opts)
	}
	return s, err
}
