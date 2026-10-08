package mailsync

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// The safety check (docs/design/accounts.md): compare the store with the
// server over a read-only connection of its own, changing nothing. Mail
// that arrived or changed since the folder's last pass is not a
// difference: server UIDs at or above the stored UIDNEXT are new, and
// with CONDSTORE a message whose MODSEQ is above the stored HIGHESTMODSEQ
// changed after the pass. Messages a queued action covers are skipped.

// maxListed caps the UIDs a Check lists on each side.
const maxListed = 20

// Check is how one synced folder compares with the server.
type Check struct {
	MailboxID       int64
	Path            string
	Server, Local   int
	MissingLocally  []uint32 // server UIDs with no local message
	MissingOnServer []uint32 // local UIDs the server no longer has
	FlagDiffs       int
	LabelDiffs      int // Gmail
}

// OK reports whether the folder matches the server.
func (c Check) OK() bool {
	return len(c.MissingLocally) == 0 && len(c.MissingOnServer) == 0 && c.FlagDiffs == 0 && c.LabelDiffs == 0
}

// ErrNotSynced means the account has no synced folder to compare yet.
var ErrNotSynced = errors.New("mailsync: the account has not synced yet")

// Verify compares an account's synced folders with its server.
func (m *Manager) Verify(ctx context.Context, accountID int64) ([]Check, error) {
	acct, err := m.db.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	a := &actor{m: m, acct: acct}
	secret, isOAuth, err := a.credential(ctx)
	if err != nil {
		return nil, err
	}
	opts := a.dialOptions(secret, isOAuth)
	opts.ReadOnly = true
	s, err := imapx.Open(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	return m.verify(ctx, s, accountID)
}

// verify compares every synced folder over cmd.
func (m *Manager) verify(ctx context.Context, cmd conn, accountID int64) ([]Check, error) {
	var gmail bool
	var labels map[string]int64
	err := m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		if gmail, err = tx.GmailAccount(ctx, accountID); err != nil || !gmail {
			return err
		}
		labels, err = tx.GmailLabels(ctx, accountID)
		return err
	})
	if err != nil {
		return nil, err
	}
	acct, err := m.db.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	mailboxes, err := m.db.ListMailboxes(ctx, accountID)
	if err != nil {
		return nil, err
	}
	var out []Check
	for _, mb := range mailboxes {
		if gmail && !isSynced(mb.Role) {
			continue
		}
		st, ok, err := m.db.MailboxSyncState(ctx, mb.ID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue // never reconciled: \Noselect, or not reached yet
		}
		c, err := m.verifyFolder(ctx, cmd, mb, st, windowAt(acct.SyncDays, st.LastSyncAt), gmail && mb.Role == api.MailboxRoleAll, labels)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, ErrNotSynced
	}
	return out, nil
}

// verifyFolder compares one folder over the window its last pass kept
// (since; zero for every message): mail that aged out after that pass is
// not a difference.
func (m *Manager) verifyFolder(ctx context.Context, cmd conn, mb store.Mailbox, st store.SyncState, since time.Time, allMail bool, labels map[string]int64) (Check, error) {
	c := Check{MailboxID: mb.ID, Path: mb.Path}
	if _, err := cmd.Select(ctx, mb.Path); err != nil {
		return c, err
	}
	server, err := cmd.UIDsSince(ctx, since)
	if err != nil {
		return c, err
	}
	local, err := m.db.LocalCopies(ctx, mb.ID)
	if err != nil {
		return c, err
	}
	c.Server, c.Local = len(server), len(local)
	byUID := map[uint32]store.LocalCopy{}
	localUIDs := make([]uint32, len(local))
	for i, l := range local {
		byUID[l.UID], localUIDs[i] = l, l.UID
	}
	added, gone, kept := diffUIDs(localUIDs, server)
	for _, uid := range added {
		if uid < st.UIDNext && len(c.MissingLocally) < maxListed {
			c.MissingLocally = append(c.MissingLocally, uid)
		}
	}
	c.MissingOnServer = gone[:min(len(gone), maxListed)]
	condstore := cmd.Capabilities().CondStore
	for start := 0; start < len(kept); start += 5 * m.cfg.Chunk {
		chunk := kept[start:min(start+5*m.cfg.Chunk, len(kept))]
		var ups []store.FlagUpdate
		if allMail {
			ups, err = cmd.FetchGmailChanges(ctx, chunk, 0)
		} else {
			ups, err = cmd.FetchFlags(ctx, chunk, 0)
		}
		if err != nil {
			return c, err
		}
		for _, u := range ups {
			l, ok := byUID[u.UID]
			if !ok || l.Queued || condstore && u.ModSeq > st.HighestModSeq {
				continue
			}
			if l.FlagsDiffer(u.Flags) {
				c.FlagDiffs++
			}
			if allMail && !slices.Equal(labelIDs(u.Labels, labels), l.Labels) {
				c.LabelDiffs++
			}
		}
	}
	return c, nil
}

// labelIDs maps X-GM-LABELS names to label mailboxes, ascending, skipping
// names with no mailbox.
func labelIDs(names []string, labels map[string]int64) []int64 {
	var out []int64
	for _, n := range names {
		if id, ok := labels[n]; ok && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}
