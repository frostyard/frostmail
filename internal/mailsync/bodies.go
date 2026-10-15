package mailsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// Bodies come over a connection of their own (C3), so reading a message
// never waits behind a sync pass on C1: a first sync can take minutes,
// and what the user opens comes first (docs/design/sync.md). C3 opens when
// a body is wanted, examines mailboxes read-only (bodies are fetched with
// BODY.PEEK and change nothing), and closes after BodyIdle without
// requests or prefetch.
//
// A reader gets what it shows: the display copy (ADR-0037), or the whole
// message when it is small. When a pass settles, C3 prefetches the bodies
// of recent INBOX and VIP mail, one at a time, the user's requests first.

// wholeLimit is the size up to which a message is fetched whole even for
// a reader: one round trip takes less time than the display copy's two.
const wholeLimit = 256 << 10

// Prefetch reaches back prefetchWindow and loads prefetchBatch candidates
// at a time.
const (
	prefetchWindow = 90 * 24 * time.Hour
	prefetchBatch  = 100
)

// bodyLoop answers body requests and prefetches until ctx ends.
func (a *actor) bodyLoop(ctx context.Context) {
	var s *imapx.Session
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()
	ready := make(chan struct{})
	close(ready)
	var queue []int64
	// failed are the messages a prefetch could not fetch, which it does
	// not try again.
	failed := map[int64]bool{}
	for {
		// A request from the user goes before the next prefetch.
		select {
		case req := <-a.bodies:
			s = a.answer(ctx, s, req)
			continue
		default:
		}
		var expire <-chan time.Time
		var more <-chan struct{}
		switch {
		case len(queue) > 0:
			more = ready
		case s != nil:
			expire = time.After(a.m.cfg.BodyIdle)
		}
		select {
		case <-ctx.Done():
			return
		case <-expire:
			_ = s.Close()
			s = nil
		case req := <-a.bodies:
			s = a.answer(ctx, s, req)
		case <-a.prefetch:
			queue = a.prefetchQueue(ctx, failed)
		case <-more:
			id := queue[0]
			queue = queue[1:]
			var err error
			if s, _, err = a.serveBody(ctx, s, id, false); err != nil {
				if s == nil {
					// C3 cannot connect: try again after the next pass.
					a.m.log.Warn("prefetch stopped", "account", a.acct.ID, "err", err)
					queue = nil
					continue
				}
				a.m.log.Debug("prefetch", "account", a.acct.ID, "message", id, "err", err)
				failed[id] = true
			}
			if len(queue) == 0 {
				queue = a.prefetchQueue(ctx, failed)
			}
		}
	}
}

// answer serves one request and returns the session to keep.
func (a *actor) answer(ctx context.Context, s *imapx.Session, req bodyRequest) *imapx.Session {
	s, id, err := a.serveBody(ctx, s, req.id, req.whole)
	req.reply <- bodyResult{blobID: id, err: err}
	return s
}

// prefetchQueue is the next messages to prefetch, without those that
// failed.
func (a *actor) prefetchQueue(ctx context.Context, failed map[int64]bool) []int64 {
	since := a.m.db.Now().Add(-prefetchWindow)
	ids, err := a.m.db.PrefetchCandidates(ctx, a.acct.ID, since, prefetchBatch+len(failed))
	if err != nil {
		if ctx.Err() == nil {
			a.m.log.Warn("prefetch", "account", a.acct.ID, "err", err)
		}
		return nil
	}
	return slices.DeleteFunc(ids, func(id int64) bool { return failed[id] })
}

// serveBody makes sure a message's body is stored, the whole message or
// what a reader shows, fetching it on s. It opens C3 when needed and once
// more when the open connection turns out to be dead, and returns the
// session to keep, nil when there is none.
func (a *actor) serveBody(ctx context.Context, s *imapx.Session, msgID int64, whole bool) (*imapx.Session, string, error) {
	// An earlier request may have fetched it while this one waited.
	detail, err := a.m.db.GetMessage(ctx, msgID)
	if err != nil {
		return s, "", err
	}
	if id := a.m.stored(detail, whole); id != "" {
		return s, id, nil
	}
	whole = whole || (detail.Size > 0 && detail.Size <= wholeLimit)
	for attempt := 0; ; attempt++ {
		if s == nil {
			var err error
			if s, err = a.openBodies(ctx); err != nil {
				return nil, "", fmt.Errorf("%w: %w", ErrOffline, err)
			}
		}
		id, err := a.fetchBody(ctx, s, msgID, whole)
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

// fetchBody downloads a message's body into the blob store on C3: the
// whole message, or its display copy.
func (a *actor) fetchBody(ctx context.Context, s *imapx.Session, id int64, whole bool) (string, error) {
	loc, err := a.m.db.MessageLocation(ctx, id)
	if err != nil {
		return "", err
	}
	// C3 keeps its mailbox between requests: examining Gmail's All Mail
	// takes a second.
	kept := s.Mailbox() == loc.MailboxPath
	if !kept {
		if _, err := s.Select(ctx, loc.MailboxPath); err != nil {
			return "", err
		}
	}
	raw, got, err := fetchOn(ctx, s, loc.UID, whole)
	if errors.Is(err, imapx.ErrNoMessage) && kept {
		// Perhaps newer than the selection: examine the mailbox again.
		if _, err := s.Select(ctx, loc.MailboxPath); err != nil {
			return "", err
		}
		raw, got, err = fetchOn(ctx, s, loc.UID, whole)
	}
	if err != nil {
		return "", err
	}
	blobID, err := a.m.blobs.Put(ctx, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	err = a.m.db.Tx(ctx, func(tx *store.Tx) error {
		if !got {
			// Nothing the API shows changes: bodyFetched is the whole body.
			return tx.SetDisplay(ctx, id, blobID)
		}
		if err := tx.SetBody(ctx, id, blobID); err != nil {
			return err
		}
		return tx.Emit(ctx, api.MessageChanged{AccountID: a.acct.ID, IDs: []int64{id}})
	})
	return blobID, err
}

// fetchOn fetches message uid in the selected mailbox, whole or its
// display copy, and the whole message when the display copy cannot be
// made. got reports whether it is the whole message.
func fetchOn(ctx context.Context, s *imapx.Session, uid uint32, whole bool) (raw []byte, got bool, err error) {
	if !whole {
		raw, err := s.FetchDisplay(ctx, uid)
		if !errors.Is(err, imapx.ErrNoDisplay) {
			return raw, false, err
		}
	}
	raw, err = s.FetchRaw(ctx, uid)
	return raw, true, err
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
