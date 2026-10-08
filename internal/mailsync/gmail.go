package mailsync

import (
	"context"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// The Gmail sync path (ADR-0012): All Mail, Spam and Trash are reconciled
// with X-GM-MSGID, X-GM-THRID and X-GM-LABELS; every other mailbox is a
// label maintained from those labels. The account uses it when its profile
// says Gmail and the server offers X-GM-EXT-1.

// conn is what the account loop sends on C1 beyond the generic reconcile
// pass: the Gmail path and offline-action replay. *imapx.Session has it,
// and tests use models of a server.
type conn interface {
	Capabilities() imapx.Capabilities
	Select(ctx context.Context, path string) (imapx.Selected, error)
	UIDs(ctx context.Context) ([]uint32, error)
	SearchMessageID(ctx context.Context, msgid string) ([]uint32, error)
	FetchGmailHeaders(ctx context.Context, uids []uint32) ([]store.MessageHeader, error)
	FetchGmailChanges(ctx context.Context, uids []uint32, changedSince uint64) ([]store.FlagUpdate, error)
	StoreLabels(ctx context.Context, uids []uint32, add, remove []string) error
	StoreFlags(ctx context.Context, uids []uint32, add, remove []string) error
	Move(ctx context.Context, uids []uint32, dest string) (map[uint32]uint32, error)
	Expunge(ctx context.Context, uids []uint32) error
	Append(ctx context.Context, mailbox string, raw []byte, flags []string) (uint32, error)
}

var _ conn = (*imapx.Session)(nil)

// syncedRoles are the Gmail folders that hold messages, in pass order.
var syncedRoles = []api.MailboxRole{api.MailboxRoleAll, api.MailboxRoleJunk, api.MailboxRoleTrash}

func isSynced(role api.MailboxRole) bool { return slices.Contains(syncedRoles, role) }

func byRole(mbs []store.Mailbox, role api.MailboxRole) *store.Mailbox {
	for i := range mbs {
		if mbs[i].Role == role {
			return &mbs[i]
		}
	}
	return nil
}

// gmailPass reconciles All Mail, Spam and Trash, in that order, and then
// prunes the messages that left all three. Any change on a Gmail account
// runs the whole pass: a message leaving one folder may have arrived in
// another, and only after all three is a missing message gone.
func (a *actor) gmailPass(ctx context.Context, conn conn) error {
	mailboxes, err := a.m.db.ListMailboxes(ctx, a.acct.ID)
	if err != nil {
		return err
	}
	for _, role := range syncedRoles {
		if mb := byRole(mailboxes, role); mb != nil {
			if err := a.reconcileGmail(ctx, conn, *mb); err != nil {
				return err
			}
		}
	}
	return a.pruneGmail(ctx)
}

// pruneGmail deletes messages no synced folder holds any more.
func (a *actor) pruneGmail(ctx context.Context) error {
	return a.m.db.Tx(ctx, func(tx *store.Tx) error {
		ids, err := tx.PruneGmail(ctx, a.acct.ID)
		if err != nil || len(ids) == 0 {
			return err
		}
		return tx.Emit(ctx, api.MessageRemoved{AccountID: a.acct.ID, IDs: ids})
	})
}

// reconcileGmail is reconcile for a Gmail synced folder.
func (a *actor) reconcileGmail(ctx context.Context, conn conn, mb store.Mailbox) error {
	path := mb.Path
	a.setStatus(func(s *api.SyncStatus) {
		s.Phase, s.Mailbox, s.Done, s.Total, s.Error = api.SyncPhaseSyncing, &path, 0, 0, nil
	})
	caps := conn.Capabilities()
	allMail := mb.Role == api.MailboxRoleAll
	sel, err := conn.Select(ctx, mb.Path)
	if err != nil {
		return err
	}
	st, ok, err := a.m.db.MailboxSyncState(ctx, mb.ID)
	if err != nil {
		return err
	}
	if ok && st.UIDValidity != sel.UIDValidity {
		a.m.log.Info("uidvalidity changed", "account", a.acct.ID, "mailbox", mb.Path)
		local, err := a.m.db.MailboxUIDs(ctx, mb.ID)
		if err != nil {
			return err
		}
		if err := a.removeGmail(ctx, mb, local); err != nil {
			return err
		}
		ok = false
	}
	if ok && caps.CondStore && st.UIDNext == sel.UIDNext && st.ServerCount == sel.Messages && st.HighestModSeq == sel.HighestModSeq {
		return nil
	}
	local, err := a.m.db.MailboxUIDs(ctx, mb.ID)
	if err != nil {
		return err
	}
	server := local
	if !ok || sel.UIDNext != st.UIDNext || sel.Messages != st.ServerCount || uint32(len(local)) != sel.Messages {
		if server, err = conn.UIDs(ctx); err != nil {
			return err
		}
	}
	added, gone, kept := diffUIDs(local, server)
	if err := a.removeGmail(ctx, mb, gone); err != nil {
		return err
	}
	slices.Reverse(added)
	total := len(added)
	for start := 0; start < len(added); start += a.m.cfg.Chunk {
		chunk := added[start:min(start+a.m.cfg.Chunk, len(added))]
		headers, err := conn.FetchGmailHeaders(ctx, chunk)
		if err != nil {
			return err
		}
		if err := a.insertGmail(ctx, mb, allMail, headers); err != nil {
			return err
		}
		done := start + len(chunk)
		a.setStatus(func(s *api.SyncStatus) { s.Done, s.Total = int64(done), int64(total) })
	}
	if len(kept) > 0 && (!ok || !caps.CondStore || sel.HighestModSeq != st.HighestModSeq) {
		var since uint64
		if ok && caps.CondStore {
			since = st.HighestModSeq
		}
		for start := 0; start < len(kept); start += 5 * a.m.cfg.Chunk {
			chunk := kept[start:min(start+5*a.m.cfg.Chunk, len(kept))]
			ups, err := conn.FetchGmailChanges(ctx, chunk, since)
			if err != nil {
				return err
			}
			if err := a.applyGmail(ctx, mb, allMail, ups); err != nil {
				return err
			}
		}
	}
	return a.m.db.Tx(ctx, func(tx *store.Tx) error {
		return tx.SetMailboxSyncState(ctx, mb.ID, store.SyncState{
			UIDValidity: sel.UIDValidity, UIDNext: sel.UIDNext, HighestModSeq: sel.HighestModSeq,
			ServerCount: sel.Messages, LastSyncAt: time.Now(),
		})
	})
}

// insertGmail stores new headers of a synced folder, threads and indexes
// the new messages, and announces what changed.
func (a *actor) insertGmail(ctx context.Context, mb store.Mailbox, allMail bool, headers []store.MessageHeader) error {
	if len(headers) == 0 {
		return nil
	}
	return a.m.db.Tx(ctx, func(tx *store.Tx) error {
		labels, err := tx.GmailLabels(ctx, a.acct.ID)
		if err != nil {
			return err
		}
		r, err := tx.InsertGmail(ctx, a.acct.ID, mb.ID, allMail, labels, headers)
		if err != nil {
			return err
		}
		ids := append(slices.Clone(r.Added), r.Changed...)
		if len(ids) == 0 {
			return nil
		}
		if _, err := tx.AssignGmailThreads(ctx, a.acct.ID, ids); err != nil {
			return err
		}
		added := map[int64]bool{}
		for _, id := range r.Added {
			added[id] = true
		}
		for _, h := range headers {
			var id int64
			if err := tx.QueryRowContext(ctx, `SELECT message_id FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`,
				mb.ID, int64(h.UID)).Scan(&id); err != nil {
				return err
			}
			if added[id] {
				if err := tx.IndexMessage(ctx, id, store.SearchDocFor(h)); err != nil {
					return err
				}
			}
		}
		return a.announce(ctx, tx, ids, append([]int64{mb.ID}, r.Mailboxes...))
	})
}

// applyGmail stores flag and label changes reported for kept UIDs.
func (a *actor) applyGmail(ctx context.Context, mb store.Mailbox, allMail bool, ups []store.FlagUpdate) error {
	if len(ups) == 0 {
		return nil
	}
	return a.m.db.Tx(ctx, func(tx *store.Tx) error {
		changed, err := tx.UpdateFlags(ctx, mb.ID, ups)
		if err != nil {
			return err
		}
		touched := []int64{mb.ID}
		if allMail {
			labels, err := tx.GmailLabels(ctx, a.acct.ID)
			if err != nil {
				return err
			}
			for _, u := range ups {
				if u.Labels == nil {
					continue
				}
				id, mbs, found, err := tx.SetGmailLabels(ctx, mb.ID, u.UID, u.Labels, labels)
				if err != nil {
					return err
				}
				if found && len(mbs) > 0 {
					touched = append(touched, mbs...)
					if !slices.Contains(changed, id) {
						changed = append(changed, id)
					}
				}
			}
		}
		if len(changed) == 0 {
			return nil
		}
		if err := refreshThreadsOf(ctx, tx, changed); err != nil {
			return err
		}
		return a.announce(ctx, tx, changed, touched)
	})
}

// removeGmail drops UIDs a synced folder no longer has; the messages wait
// for pruneGmail in case they moved to another synced folder.
func (a *actor) removeGmail(ctx context.Context, mb store.Mailbox, uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}
	return a.m.db.Tx(ctx, func(tx *store.Tx) error {
		msgs, mbs, err := tx.RemoveGmailUIDs(ctx, mb.ID, uids)
		if err != nil || len(msgs) == 0 {
			return err
		}
		return a.announce(ctx, tx, msgs, append([]int64{mb.ID}, mbs...))
	})
}

// announce emits message.changed and mailbox.changed for each mailbox once.
func (a *actor) announce(ctx context.Context, tx *store.Tx, msgs, mailboxes []int64) error {
	if len(msgs) > 0 {
		if err := tx.Emit(ctx, api.MessageChanged{AccountID: a.acct.ID, IDs: msgs}); err != nil {
			return err
		}
	}
	slices.Sort(mailboxes)
	for _, id := range slices.Compact(mailboxes) {
		if err := tx.Emit(ctx, api.MailboxChanged{ID: id, AccountID: a.acct.ID}); err != nil {
			return err
		}
	}
	return nil
}
