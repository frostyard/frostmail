package mailsync

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// reconcile brings one mailbox in step with the server
// (docs/design/sync.md, the reconcile pass). It is safe to interrupt at any
// point and to run again: inserts are idempotent on (mailbox, uid), and the
// sync state that arms the fast path is stored last.
func (a *actor) reconcile(ctx context.Context, cmd *imapx.Session, mb store.Mailbox) error {
	path := mb.Path
	a.setStatus(func(s *api.SyncStatus) {
		s.Phase, s.Mailbox, s.Done, s.Total, s.Error = api.SyncPhaseSyncing, &path, 0, 0, nil
	})
	sel, err := cmd.Select(ctx, mb.Path)
	if err != nil {
		return err
	}
	st, ok, err := a.m.db.MailboxSyncState(ctx, mb.ID)
	if err != nil {
		return err
	}
	if ok && st.UIDValidity != sel.UIDValidity {
		// Every UID is invalid: drop the local copies and resync. Bodies
		// already in the blob store are reused by content when refetched.
		a.m.log.Info("uidvalidity changed", "account", a.acct.ID, "mailbox", mb.Path)
		local, err := a.m.db.MailboxUIDs(ctx, mb.ID)
		if err != nil {
			return err
		}
		if err := a.remove(ctx, mb, local); err != nil {
			return err
		}
		ok = false
	}
	if ok && cmd.Caps.CondStore && st.UIDNext == sel.UIDNext && st.ServerCount == sel.Messages && st.HighestModSeq == sel.HighestModSeq {
		return nil // fast path: nothing moved
	}

	local, err := a.m.db.MailboxUIDs(ctx, mb.ID)
	if err != nil {
		return err
	}
	server := local
	if !ok || sel.UIDNext != st.UIDNext || sel.Messages != st.ServerCount || uint32(len(local)) != sel.Messages {
		if server, err = cmd.UIDs(ctx); err != nil {
			return err
		}
	}
	added, gone, kept := diffUIDs(local, server)

	if err := a.remove(ctx, mb, gone); err != nil {
		return err
	}
	// Newest first, so a fresh account shows recent mail early.
	slices.Reverse(added)
	total := len(added)
	for start := 0; start < len(added); start += a.m.cfg.Chunk {
		chunk := added[start:min(start+a.m.cfg.Chunk, len(added))]
		headers, err := cmd.FetchHeaders(ctx, chunk)
		if err != nil {
			return err
		}
		if err := a.insert(ctx, mb, headers); err != nil {
			return err
		}
		done := start + len(chunk)
		a.setStatus(func(s *api.SyncStatus) { s.Done, s.Total = int64(done), int64(total) })
	}

	if len(kept) > 0 && (!ok || !cmd.Caps.CondStore || sel.HighestModSeq != st.HighestModSeq) {
		var since uint64
		if ok && cmd.Caps.CondStore {
			since = st.HighestModSeq
		}
		for start := 0; start < len(kept); start += 5 * a.m.cfg.Chunk {
			chunk := kept[start:min(start+5*a.m.cfg.Chunk, len(kept))]
			ups, err := cmd.FetchFlags(ctx, chunk, since)
			if err != nil {
				return err
			}
			if err := a.applyFlags(ctx, mb, ups); err != nil {
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

// diffUIDs splits two ascending UID lists into server-only (added),
// local-only (gone) and common (kept) UIDs, each ascending.
func diffUIDs(local, server []uint32) (added, gone, kept []uint32) {
	i, j := 0, 0
	for i < len(local) || j < len(server) {
		switch {
		case j == len(server) || i < len(local) && local[i] < server[j]:
			gone = append(gone, local[i])
			i++
		case i == len(local) || server[j] < local[i]:
			added = append(added, server[j])
			j++
		default:
			kept = append(kept, local[i])
			i++
			j++
		}
	}
	return added, gone, kept
}

// insert stores new headers, threads and indexes them, and emits the batch.
func (a *actor) insert(ctx context.Context, mb store.Mailbox, headers []store.MessageHeader) error {
	if len(headers) == 0 {
		return nil
	}
	return a.m.db.Tx(ctx, func(tx *store.Tx) error {
		ids, err := tx.InsertHeaders(ctx, a.acct.ID, mb.ID, headers)
		if err != nil || len(ids) == 0 {
			return err
		}
		if _, err := tx.AssignThreads(ctx, a.acct.ID, ids); err != nil {
			return err
		}
		// Index the messages this insert created: resolve each header's UID
		// to its message and skip the ones that were already stored.
		created := map[int64]bool{}
		for _, id := range ids {
			created[id] = true
		}
		for _, h := range headers {
			var id int64
			err := tx.QueryRowContext(ctx, `SELECT message_id FROM message_mailbox WHERE mailbox_id = ? AND uid = ?`, mb.ID, int64(h.UID)).Scan(&id)
			if err != nil {
				return err
			}
			if created[id] {
				if err := tx.IndexMessage(ctx, id, store.SearchDocFor(h)); err != nil {
					return err
				}
			}
		}
		if err := tx.Emit(ctx, api.MessageChanged{AccountID: a.acct.ID, IDs: ids}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.MailboxChanged{ID: mb.ID, AccountID: a.acct.ID})
	})
}

// remove deletes local copies of UIDs the server no longer has.
func (a *actor) remove(ctx context.Context, mb store.Mailbox, uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}
	return a.m.db.Tx(ctx, func(tx *store.Tx) error {
		threads, err := tx.ThreadsOfUIDs(ctx, mb.ID, uids)
		if err != nil {
			return err
		}
		removed, err := tx.RemoveUIDs(ctx, mb.ID, uids)
		if err != nil {
			return err
		}
		if err := tx.RefreshThreads(ctx, threads); err != nil {
			return err
		}
		if len(removed) > 0 {
			if err := tx.Emit(ctx, api.MessageRemoved{AccountID: a.acct.ID, IDs: removed}); err != nil {
				return err
			}
		}
		return tx.Emit(ctx, api.MailboxChanged{ID: mb.ID, AccountID: a.acct.ID})
	})
}

// applyFlags stores flags the server reports and emits the messages whose
// flags changed.
func (a *actor) applyFlags(ctx context.Context, mb store.Mailbox, ups []store.FlagUpdate) error {
	if len(ups) == 0 {
		return nil
	}
	return a.m.db.Tx(ctx, func(tx *store.Tx) error {
		changed, err := tx.UpdateFlags(ctx, mb.ID, ups)
		if err != nil || len(changed) == 0 {
			return err
		}
		threads, err := tx.ThreadsOf(ctx, changed)
		if err != nil {
			return err
		}
		if err := tx.RefreshThreads(ctx, threads); err != nil {
			return err
		}
		if err := tx.Emit(ctx, api.MessageChanged{AccountID: a.acct.ID, IDs: changed}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.MailboxChanged{ID: mb.ID, AccountID: a.acct.ID})
	})
}

// fetchBody downloads a message into the blob store on C1.
func (a *actor) fetchBody(ctx context.Context, cmd *imapx.Session, id int64) (string, error) {
	loc, err := a.m.db.MessageLocation(ctx, id)
	if err != nil {
		return "", err
	}
	if _, err := cmd.Select(ctx, loc.MailboxPath); err != nil {
		return "", err
	}
	raw, err := cmd.FetchRaw(ctx, loc.UID)
	if err != nil {
		return "", err
	}
	blobID, err := a.m.blobs.Put(ctx, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	err = a.m.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetBody(ctx, id, blobID); err != nil {
			return err
		}
		return tx.Emit(ctx, api.MessageChanged{AccountID: a.acct.ID, IDs: []int64{id}})
	})
	return blobID, err
}
