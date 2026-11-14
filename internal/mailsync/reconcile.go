package mailsync

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/store"
)

// reconcile brings one mailbox in step with the server
// (docs/design/sync.md, the reconcile pass). It is safe to interrupt at any
// point and to run again: inserts are idempotent on (mailbox, uid), and the
// sync state that arms the fast path is stored last.
func (a *actor) reconcile(ctx context.Context, cmd *imapx.Session, mb store.Mailbox) error {
	began := time.Now()
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
	if now := a.m.windowNow(); a.mustSearch(ok, st, sel, len(local), now) {
		if server, err = cmd.UIDsSince(ctx, windowAt(a.acct.SyncDays, now)); err != nil {
			return err
		}
	}
	added, gone, kept := diffUIDs(local, server)
	if added, err = a.notLeaving(ctx, mb.ID, added); err != nil {
		return err
	}

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
		ids, err := a.insert(ctx, mb, headers, ok && !a.acct.ReadOnly)
		if err != nil {
			return err
		}
		if ok {
			a.fresh = append(a.fresh, ids...) // new mail, not the first sync; the scope decides (announceNew)
		}
		done := start + len(chunk)
		a.setStatus(func(s *api.SyncStatus) { s.Done, s.Total = int64(done), int64(total) })
	}

	changed := 0
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
			n, err := a.applyFlags(ctx, mb, ups)
			if err != nil {
				return err
			}
			changed += n
		}
	}

	if err := a.storeSyncState(ctx, mb.ID, store.SyncState{
		UIDValidity: sel.UIDValidity, UIDNext: sel.UIDNext, HighestModSeq: sel.HighestModSeq,
		ServerCount: sel.Messages, LastSyncAt: a.m.windowNow(),
	}); err != nil {
		return err
	}
	a.record(activity.MailboxSynced(a.acct.ID, mb.ID, len(added), changed, len(gone), time.Since(began)))
	return nil
}

// notLeaving is uids without those that queued moves take out of the
// mailbox: the store has moved them already, the server not until the
// moves replay, so they are not new mail.
func (a *actor) notLeaving(ctx context.Context, mailboxID int64, uids []uint32) ([]uint32, error) {
	if len(uids) == 0 {
		return uids, nil
	}
	ops, err := a.m.db.QueuedOps(ctx, a.acct.ID, opMove)
	if err != nil {
		return nil, err
	}
	leaving := map[uint32]bool{}
	for _, op := range ops {
		var p moveOp
		if err := json.Unmarshal(op.Payload, &p); err != nil {
			return nil, fmt.Errorf("move op %d: %w", op.ID, err)
		}
		if p.From == mailboxID {
			for _, it := range p.Items {
				leaving[it.UID] = true
			}
		}
	}
	return slices.DeleteFunc(uids, func(u uint32) bool { return leaving[u] }), nil
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

// insert stores new headers, threads and indexes them, emits the batch,
// and returns the messages it created.
func (a *actor) insert(ctx context.Context, mb store.Mailbox, headers []store.MessageHeader, waiting bool) ([]int64, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	var created []int64
	err := a.m.db.Tx(ctx, func(tx *store.Tx) error {
		ids, err := tx.InsertHeaders(ctx, a.acct.ID, mb.ID, headers)
		if err != nil || len(ids) == 0 {
			return err
		}
		created = ids
		if _, err := tx.AssignThreads(ctx, a.acct.ID, ids); err != nil {
			return err
		}
		// Index the messages this insert created, and skip the headers that
		// were already stored.
		if err := indexCreated(ctx, tx, mb.ID, headers, ids); err != nil {
			return err
		}
		if waiting {
			// New inbox mail waits for the rules (ADR-0024).
			if err := tx.MarkRulesWaiting(ctx, ids); err != nil {
				return err
			}
		}
		if err := tx.Emit(ctx, api.MessageChanged{AccountID: a.acct.ID, IDs: ids}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.MailboxChanged{ID: mb.ID, AccountID: a.acct.ID})
	})
	return created, applied(ctx, err)
}

// indexCreated adds to the search index the headers whose message is one of
// created, the messages an insert of headers into a mailbox made. It finds
// the messages of the whole batch at once: a query for each header would
// hold the writer for a first sync's every message (issue 290).
func indexCreated(ctx context.Context, tx *store.Tx, mailboxID int64, headers []store.MessageHeader, created []int64) error {
	if len(created) == 0 {
		return nil
	}
	isNew := make(map[int64]bool, len(created))
	for _, id := range created {
		isNew[id] = true
	}
	uids := make([]uint32, len(headers))
	for i, h := range headers {
		uids[i] = h.UID
	}
	at, err := tx.MessagesAtUIDs(ctx, mailboxID, uids)
	if err != nil {
		return err
	}
	for _, h := range headers {
		id, ok := at[h.UID]
		if !ok {
			return fmt.Errorf("index uid %d of mailbox %d: %w", h.UID, mailboxID, store.ErrNotFound)
		}
		if isNew[id] {
			if err := tx.IndexMessage(ctx, id, store.SearchDocFor(h)); err != nil {
				return err
			}
		}
	}
	return nil
}

// remove deletes local copies of UIDs the server no longer has.
func (a *actor) remove(ctx context.Context, mb store.Mailbox, uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}
	return applied(ctx, a.m.db.Tx(ctx, func(tx *store.Tx) error {
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
	}))
}

// applyFlags stores flags the server reports, emits the messages whose
// flags changed, and returns how many did. On a server that keeps no
// keywords, a message's keywords and color stay as they are here.
func (a *actor) applyFlags(ctx context.Context, mb store.Mailbox, ups []store.FlagUpdate) (int, error) {
	if len(ups) == 0 {
		return 0, nil
	}
	update := (*store.Tx).UpdateFlags
	if providers.ForKind(a.acct.Kind).Quirks.NoKeywords {
		update = (*store.Tx).UpdateSystemFlags
	}
	n := 0
	err := applied(ctx, a.m.db.Tx(ctx, func(tx *store.Tx) error {
		changed, err := update(tx, ctx, mb.ID, ups)
		n = len(changed)
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
	}))
	return n, err
}
