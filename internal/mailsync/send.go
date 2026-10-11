package mailsync

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/smtpx"
	"github.com/frostyard/frostmail/internal/store"
)

// Sending (docs/design/send.md, Outbox). Each account has a sender next to
// its IMAP actor, so mail goes out while IMAP is down. A message the SMTP
// server accepted is marked accepted, its draft deleted and its Sent copy
// queued as an offline op, all in one transaction; the IMAP actor writes
// the copy and marks it sent. Rows left in sending by a stopped sender are
// settled by the next IMAP pass, which looks for them in Sent.

// The op kinds pending_ops allows for these (migration 0001).
const (
	opAppendSent = "append"
	opRemoveCopy = "delete"
)

// appendSentOp saves a sent message in the account's Sent mailbox.
type appendSentOp struct {
	Outbox    int64  `json:"outbox"`
	Blob      string `json:"blob"`
	MessageID string `json:"messageId"`
}

// removeCopyOp deletes one message from a mailbox by UID: a draft's
// replaced or discarded server copy.
type removeCopyOp struct {
	Mailbox int64  `json:"mailbox"`
	UID     uint32 `json:"uid"`
}

// QueueRemoveDraftCopy queues, in tx, the deletion of a draft's server copy
// at uid in the account's Drafts mailbox. It does nothing for uid 0 or an
// account without a Drafts mailbox. Kick the account afterwards.
func QueueRemoveDraftCopy(ctx context.Context, tx *store.Tx, accountID int64, uid uint32) error {
	if uid == 0 {
		return nil
	}
	mb, err := tx.MailboxByRole(ctx, accountID, api.MailboxRoleDrafts)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.QueueOp(ctx, accountID, opRemoveCopy, removeCopyOp{Mailbox: mb.ID, UID: uid}, nil)
	return err
}

// Kick tells an account's IMAP actor that offline ops are queued.
func (m *Manager) Kick(accountID int64) { m.kick(accountID) }

// OutboxChanged wakes an account's sender: a message was queued or retried.
func (m *Manager) OutboxChanged(accountID int64) {
	if a := m.actor(accountID); a != nil {
		select {
		case a.outbox <- struct{}{}:
		default:
		}
	}
}

// DraftsChanged tells an account's IMAP actor that a draft changed, so it
// saves the server copy once the draft is quiet.
func (m *Manager) DraftsChanged(accountID int64) {
	if a := m.actor(accountID); a != nil {
		select {
		case a.drafts <- struct{}{}:
		default:
		}
	}
}

// addInterrupted records outbox rows left in sending, for the account's
// next IMAP pass to settle.
func (m *Manager) addInterrupted(accountID int64, ids ...int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interrupted[accountID] = append(m.interrupted[accountID], ids...)
}

func (m *Manager) takeInterrupted(accountID int64) []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.interrupted[accountID]
	delete(m.interrupted, accountID)
	return ids
}

// collectInterrupted finds rows a previous maild left in sending.
func (m *Manager) collectInterrupted(ctx context.Context) error {
	rows, err := m.db.OutboxInState(ctx, 0, "sending")
	if err != nil {
		return fmt.Errorf("interrupted sends: %w", err)
	}
	for _, r := range rows {
		m.addInterrupted(r.AccountID, r.ID)
	}
	return nil
}

// sendLoop sends the account's due messages until ctx ends, waking when a
// message is queued and when the next one falls due.
func (a *actor) sendLoop(ctx context.Context) {
	if a.acct.ReadOnly {
		<-ctx.Done() // nothing is sent; making the account writable restarts the actor
		return
	}
	for {
		a.sendDue(ctx)
		var due <-chan time.Time
		at, ok, err := a.m.db.NextOutboxDue(ctx, a.acct.ID)
		switch {
		case err != nil && ctx.Err() == nil:
			a.m.log.Warn("outbox", "account", a.acct.ID, "err", err)
			due = time.After(a.m.cfg.MaxBackoff)
		case ok:
			// At least a moment, so a row the clock disagrees about cannot
			// spin, and at most a minute: timers do not run while the
			// computer sleeps, so a Send Later time is checked against the
			// wall clock (ADR-0025).
			due = time.After(min(max(time.Until(at), 10*time.Millisecond), time.Minute))
		}
		select {
		case <-ctx.Done():
			return
		case <-a.outbox:
		case <-due:
		}
	}
}

// sendDue sends every queued message whose time has come, oldest first.
func (a *actor) sendDue(ctx context.Context) {
	items, err := a.m.db.DueOutbox(ctx, a.acct.ID, time.Now())
	if err != nil {
		if ctx.Err() == nil {
			a.m.log.Warn("outbox", "account", a.acct.ID, "err", err)
		}
		return
	}
	for _, it := range items {
		if ctx.Err() != nil {
			return
		}
		if err := a.sendOne(ctx, it); err != nil && ctx.Err() == nil {
			a.m.log.Warn("send", "account", a.acct.ID, "outbox", it.ID, "err", err)
		}
	}
}

// sendOne claims a queued message, submits it, and records the outcome.
func (a *actor) sendOne(ctx context.Context, it store.OutboxItem) error {
	err := a.m.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.MoveOutbox(ctx, it.ID, "queued", "sending"); err != nil {
			return err
		}
		return emitOutbox(ctx, tx, it, api.OutboxStateSending)
	})
	if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
		return nil // cancelled, or claimed by someone else
	}
	if err != nil {
		return err
	}
	sendErr := a.submit(ctx, it)
	// From here on the outcome is recorded even if ctx ends: the server may
	// have the message.
	wctx := context.WithoutCancel(ctx)
	switch {
	case sendErr == nil:
		return a.accepted(wctx, it, true)
	case ctx.Err() != nil:
		// Stopped mid-send: whether the server took it is unknown.
		a.m.addInterrupted(a.acct.ID, it.ID)
		return nil
	case smtpx.Permanent(sendErr) || errors.Is(sendErr, errUnsendable):
		return a.m.db.Tx(wctx, func(tx *store.Tx) error {
			if err := tx.FailOutbox(wctx, it.ID, sendErr.Error()); err != nil {
				return err
			}
			return emitOutbox(wctx, tx, it, api.OutboxStateFailed)
		})
	default:
		next := time.Now().Add(retryDelay(sendErr, it.Attempts+1, a.m.cfg.SendRetry))
		return a.m.db.Tx(wctx, func(tx *store.Tx) error {
			if err := tx.RetryOutboxLater(wctx, it.ID, next, sendErr.Error()); err != nil {
				return err
			}
			return emitOutbox(wctx, tx, it, api.OutboxStateQueued)
		})
	}
}

// retryDelay is how long a message waits after a temporary failure, in
// units of Config.SendRetry (a minute): one unit while the server cannot be
// reached (so mail goes out soon after it returns), else 1, 2, 5 and 15
// units by attempt, then 60.
func retryDelay(err error, attempt int, unit time.Duration) time.Duration {
	if !smtpx.Replied(err) {
		return unit
	}
	steps := []time.Duration{1, 2, 5, 15}
	if attempt >= 1 && attempt <= len(steps) {
		return steps[attempt-1] * unit
	}
	return 60 * unit
}

// submit hands one message to the account's SMTP server. For OAuth
// accounts a refused token is refreshed once.
func (a *actor) submit(ctx context.Context, it store.OutboxItem) error {
	secret, isOAuth, err := a.credential(ctx)
	if errors.Is(err, errNoPassword) || errors.Is(err, errSignIn) {
		return fmt.Errorf("%w: %w", smtpx.ErrAuth, err)
	}
	if err != nil {
		return err
	}
	s := a.acct.SMTP
	opts := smtpx.Options{
		Host: s.Host, Port: s.Port, TLS: s.TLS, Username: s.Username, Password: secret, OAuth: isOAuth,
		InsecureSkipVerify: a.m.cfg.InsecureSkipVerify,
	}
	err = a.sendBlob(ctx, opts, it)
	if errors.Is(err, smtpx.ErrAuth) && isOAuth {
		if opts.Password, err = a.refreshed(ctx); err != nil {
			return fmt.Errorf("%w: %w", smtpx.ErrAuth, err)
		}
		err = a.sendBlob(ctx, opts, it)
	}
	return err
}

// sendBlob submits the outbox row's built message.
func (a *actor) sendBlob(ctx context.Context, opts smtpx.Options, it store.OutboxItem) error {
	f, err := a.m.blobs.Open(it.BlobID)
	if err != nil {
		return fmt.Errorf("%w: %w", errUnsendable, err)
	}
	defer f.Close()
	size, err := blobSize(f)
	if err != nil {
		return err
	}
	return smtpx.Send(ctx, opts, smtpx.Envelope{From: it.From, To: it.Recipients}, size, f)
}

// errUnsendable marks a send that can never work: the built message is
// gone from the blob store.
var errUnsendable = errors.New("the built message is missing")

func blobSize(r io.Reader) (int64, error) {
	f, ok := r.(*os.File)
	if !ok {
		return 0, errors.New("blob is not a file")
	}
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// accepted records that the server took a message: the row moves to
// accepted, the draft and its server copy go, the recipients are counted
// for suggestions, and the Sent copy is queued (Gmail files its own, so its
// rows are sent at once). appendCopy false means the copy is known to be in
// Sent already.
func (a *actor) accepted(ctx context.Context, it store.OutboxItem, appendCopy bool) error {
	err := a.m.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.MoveOutbox(ctx, it.ID, "sending", "accepted"); err != nil {
			return err
		}
		if it.DraftID != 0 {
			d, err := tx.DeleteDraft(ctx, it.DraftID)
			switch {
			case errors.Is(err, store.ErrNotFound):
			case err != nil:
				return err
			default:
				if err := QueueRemoveDraftCopy(ctx, tx, a.acct.ID, d.ServerUID); err != nil {
					return err
				}
				if err := tx.Emit(ctx, api.DraftChanged{ID: d.ID, AccountID: d.AccountID, Deleted: true}); err != nil {
					return err
				}
			}
		}
		seen := append([]store.Address(nil), it.To...)
		for _, r := range it.Recipients {
			seen = append(seen, store.Address{Addr: r})
		}
		if err := tx.RecordAddresses(ctx, seen, tx.Now()); err != nil {
			return err
		}
		if !appendCopy || a.acct.Kind == api.AccountKindGmail {
			if err := tx.MoveOutbox(ctx, it.ID, "accepted", "sent"); err != nil {
				return err
			}
			return emitOutbox(ctx, tx, it, api.OutboxStateSent)
		}
		op := appendSentOp{Outbox: it.ID, Blob: it.BlobID, MessageID: it.MessageID}
		if _, err := tx.QueueOp(ctx, a.acct.ID, opAppendSent, op, nil); err != nil {
			return err
		}
		return emitOutbox(ctx, tx, it, api.OutboxStateAccepted)
	})
	if err != nil {
		return err
	}
	a.m.kick(a.acct.ID)
	return nil
}

func emitOutbox(ctx context.Context, tx *store.Tx, it store.OutboxItem, state api.OutboxState) error {
	return tx.Emit(ctx, api.OutboxChanged{ID: it.ID, AccountID: it.AccountID, State: state})
}

// settleInterrupted decides the rows a stopped sender left in sending, once
// a full pass has brought Sent up to date: a message found there was
// accepted; any other is queued again at once.
func (a *actor) settleInterrupted(ctx context.Context) error {
	ids := a.m.takeInterrupted(a.acct.ID)
	if len(ids) == 0 {
		return nil
	}
	sent, err := a.m.db.MailboxByRole(ctx, a.acct.ID, api.MailboxRoleSent)
	hasSent := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		a.m.addInterrupted(a.acct.ID, ids...)
		return err
	}
	requeued := false
	for i, id := range ids {
		it, err := a.m.db.GetOutbox(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			a.m.addInterrupted(a.acct.ID, ids[i:]...)
			return err
		}
		if it.State != "sending" {
			continue
		}
		found := false
		if hasSent {
			if found, err = a.m.db.HasMessageIn(ctx, sent.ID, it.MessageID); err != nil {
				a.m.addInterrupted(a.acct.ID, ids[i:]...)
				return err
			}
		}
		if found {
			if err := a.accepted(ctx, it, false); err != nil {
				return err
			}
			continue
		}
		err = a.m.db.Tx(ctx, func(tx *store.Tx) error {
			if err := tx.RetryOutboxLater(ctx, it.ID, time.Now(), "interrupted before the server answered"); err != nil {
				return err
			}
			return emitOutbox(ctx, tx, it, api.OutboxStateQueued)
		})
		if err != nil {
			return err
		}
		requeued = true
	}
	if requeued {
		a.m.OutboxChanged(a.acct.ID)
	}
	return nil
}

// replayAppendSent writes a sent message's copy into Sent, unless a copy
// with its Message-ID is already there (a replay after a crash), and marks
// the row sent. Without a Sent mailbox the row is marked sent directly.
func (a *actor) replayAppendSent(ctx context.Context, cmd conn, op store.Op) error {
	var p appendSentOp
	if err := json.Unmarshal(op.Payload, &p); err != nil {
		return err
	}
	mbs, err := a.m.db.ListMailboxes(ctx, a.acct.ID)
	if err != nil {
		return err
	}
	if len(mbs) == 0 {
		return errAfterListing // never listed: whether there is a Sent mailbox is unknown
	}
	sent, err := a.m.db.MailboxByRole(ctx, a.acct.ID, api.MailboxRoleSent)
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return err
	default:
		if _, err := cmd.Select(ctx, sent.Path); err != nil {
			return err
		}
		have, err := cmd.SearchMessageID(ctx, p.MessageID)
		if err != nil {
			return err
		}
		if len(have) == 0 {
			raw, err := a.readBlob(p.Blob)
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("sent copy: %w: %w", store.ErrNotFound, err) // refused: settle the row
			}
			if err != nil {
				return err
			}
			if _, err := cmd.Append(ctx, sent.Path, raw, []string{`\Seen`}); err != nil {
				return err
			}
		}
	}
	return a.markSent(ctx, p.Outbox)
}

// errAfterListing keeps an op queued until the account's mailboxes have
// been listed; the replay after the first full pass runs it.
var errAfterListing = errors.New("mailsync: the op waits for the mailbox list")

// markSent moves an accepted row to sent.
func (a *actor) markSent(ctx context.Context, id int64) error {
	return a.m.db.Tx(ctx, func(tx *store.Tx) error {
		err := tx.MoveOutbox(ctx, id, "accepted", "sent")
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) {
			return nil
		}
		if err != nil {
			return err
		}
		return tx.Emit(ctx, api.OutboxChanged{ID: id, AccountID: a.acct.ID, State: api.OutboxStateSent})
	})
}

// replayRemoveCopy deletes one UID from a mailbox (\Deleted, then UID
// EXPUNGE when the server has UIDPLUS) and forgets it locally. On Gmail it
// moves the copy to Trash and expunges it there: expunging from a label
// folder may only remove the label (ADR-0012), and the next pass forgets
// the local copy.
func (a *actor) replayRemoveCopy(ctx context.Context, cmd conn, op store.Op) error {
	var p removeCopyOp
	if err := json.Unmarshal(op.Payload, &p); err != nil {
		return err
	}
	path, err := a.mailboxPath(ctx, p.Mailbox)
	if err != nil {
		return err
	}
	if _, err := cmd.Select(ctx, path); err != nil {
		return err
	}
	uids := []uint32{p.UID}
	if a.gmail {
		trash, err := a.m.db.MailboxByRole(ctx, a.acct.ID, api.MailboxRoleTrash)
		if err != nil {
			return err
		}
		moved, err := cmd.Move(ctx, uids, trash.Path)
		if err != nil || moved[p.UID] == 0 {
			return err // already gone, or no COPYUID: it waits in Trash
		}
		uids = []uint32{moved[p.UID]}
		if _, err := cmd.Select(ctx, trash.Path); err != nil {
			return err
		}
		if err := cmd.StoreFlags(ctx, uids, []string{`\Deleted`}, nil); err != nil {
			return err
		}
		return cmd.Expunge(ctx, uids)
	}
	if err := cmd.StoreFlags(ctx, uids, []string{`\Deleted`}, nil); err != nil {
		return err
	}
	if cmd.Capabilities().UIDPlus {
		if err := cmd.Expunge(ctx, uids); err != nil {
			return err
		}
	}
	return a.remove(ctx, store.Mailbox{ID: p.Mailbox, AccountID: a.acct.ID, Path: path}, uids)
}

func (a *actor) readBlob(id string) ([]byte, error) {
	f, err := a.m.blobs.Open(id)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
