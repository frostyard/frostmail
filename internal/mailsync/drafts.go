package mailsync

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/compose"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// Draft copies (docs/design/send.md, Drafts): drafts live in the store; the
// IMAP actor writes each one's copy into the Drafts mailbox once it has
// been quiet for Config.DraftQuiet, then deletes the copy it replaces.

// nextDraftSave returns when to look for quiet drafts again, or nil when
// every copy is current. It waits at least the quiet period after a save
// attempt so a draft that cannot be saved does not spin the loop.
func (a *actor) nextDraftSave(ctx context.Context, attempted bool) <-chan time.Time {
	at, ok, err := a.m.db.NextDraftSave(ctx, a.acct.ID)
	if err != nil || !ok {
		return nil
	}
	wait := time.Until(at.Add(a.m.cfg.DraftQuiet))
	if attempted {
		wait = max(wait, a.m.cfg.DraftQuiet)
	}
	return time.After(max(wait, 0))
}

// saveDrafts writes the server copy of every quiet draft whose copy is
// missing or out of date. Drafts stay local on an account without a Drafts
// mailbox. A draft the server refuses is skipped until it changes again.
func (a *actor) saveDrafts(ctx context.Context, cmd *imapx.Session) error {
	list, err := a.m.db.DraftsToSave(ctx, a.acct.ID, time.Now().Add(-a.m.cfg.DraftQuiet))
	if err != nil || len(list) == 0 {
		return err
	}
	mb, err := a.m.db.MailboxByRole(ctx, a.acct.ID, api.MailboxRoleDrafts)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	saved := false
	for _, d := range list {
		if a.unsaved[d.ID].Equal(d.UpdatedAt) {
			continue // refused before, unchanged since
		}
		raw, err := a.draftCopy(ctx, d)
		if err != nil {
			a.m.log.Warn("draft copy", "account", a.acct.ID, "draft", d.ID, "err", err)
			a.unsaved[d.ID] = d.UpdatedAt
			continue
		}
		uid, err := cmd.Append(ctx, mb.Path, raw, []string{`\Draft`, `\Seen`})
		var imapErr *imap.Error
		if errors.As(err, &imapErr) {
			a.m.log.Warn("server refused a draft copy", "account", a.acct.ID, "draft", d.ID, "err", err)
			a.unsaved[d.ID] = d.UpdatedAt
			continue
		}
		if err != nil {
			return err
		}
		if uid == 0 { // no UIDPLUS: find the copy by its Message-ID
			if _, err := cmd.Select(ctx, mb.Path); err != nil {
				return err
			}
			uids, err := cmd.SearchMessageID(ctx, d.MessageID)
			if err != nil {
				return err
			}
			for _, u := range uids {
				if u != d.ServerUID {
					uid = max(uid, u)
				}
			}
		}
		delete(a.unsaved, d.ID)
		saved = true
		err = a.m.db.Tx(ctx, func(tx *store.Tx) error {
			err := tx.SetDraftServerCopy(ctx, d.ID, uid, d.UpdatedAt)
			if errors.Is(err, store.ErrNotFound) { // discarded while saving
				return QueueRemoveDraftCopy(ctx, tx, a.acct.ID, uid)
			}
			if err != nil {
				return err
			}
			if d.ServerUID != 0 && d.ServerUID != uid {
				return QueueRemoveDraftCopy(ctx, tx, a.acct.ID, d.ServerUID)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if !saved {
		return nil
	}
	if err := a.replay(ctx, cmd); err != nil { // the replaced copies
		return err
	}
	return a.reconcile(ctx, cmd, mb)
}

// draftCopy builds a draft's server copy: Bcc kept, and recipients the
// builder would refuse (half-typed addresses) left out.
func (a *actor) draftCopy(ctx context.Context, d store.Draft) ([]byte, error) {
	ident, err := a.m.db.GetIdentity(ctx, d.Content.IdentityID)
	if err != nil || ident.AccountID != d.AccountID {
		if ident, err = a.m.db.DefaultIdentity(ctx, d.AccountID); err != nil {
			return nil, err
		}
	}
	m := DraftMessage(d, ident, a.m.blobs, a.m.cfg.Parts, d.UpdatedAt)
	m.WriteBcc = true
	m.To, m.Cc, m.Bcc = validOnly(m.To), validOnly(m.Cc), validOnly(m.Bcc)
	var buf bytes.Buffer
	if err := compose.Build(&buf, m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func validOnly(list []compose.Address) []compose.Address {
	var out []compose.Address
	for _, a := range list {
		if compose.ValidAddress(a) {
			out = append(out, a)
		}
	}
	return out
}
