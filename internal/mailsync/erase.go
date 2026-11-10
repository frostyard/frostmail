package mailsync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// eraseBatch is how many messages one pass erases from one mailbox: the
// next pass takes the rest (ADR-0051).
const eraseBatch = 1000

// eraseDays is an account's erase setting for one role mailbox and the
// role it looks at.
type eraseDays struct {
	role api.MailboxRole
	days func(store.Account) int
}

// eraseSettings are the two settings of ADR-0051: the account's trash
// mailbox is erased after eraseTrashDays days, its junk mailbox after
// eraseJunkDays.
var eraseSettings = []eraseDays{
	{api.MailboxRoleTrash, func(a store.Account) int { return a.EraseTrashDays }},
	{api.MailboxRoleJunk, func(a store.Account) int { return a.EraseJunkDays }},
}

// EraseOld erases the mail that has been at least the account's chosen number
// of days in its Trash or Junk mailbox (ADR-0051, Erase Deleted Items and
// Erase Junk Mail): as mailbox.erase does, the old members are hidden at once
// and one expunge op is queued for them, in one transaction per mailbox. It
// looks only at the mailbox that has the trash or junk role when it runs, and
// never at a read-only account's mail. One account's error is logged and the
// pass goes on; the first error is returned.
func (m *Manager) EraseOld(ctx context.Context, now time.Time) error {
	accounts, err := m.db.ListAccounts(ctx)
	if err != nil {
		return fmt.Errorf("erase old mail: %w", err)
	}
	var first error
	for _, acct := range accounts {
		if err := m.eraseAccount(ctx, acct, now); err != nil {
			if ctx.Err() != nil {
				return err // the pass was stopped: the rest would fail too
			}
			m.log.Warn("old mail not erased", "account", acct.ID, "err", err)
			if first == nil {
				first = err
			}
		}
	}
	return first
}

// eraseAccount erases the old mail of each of one account's role mailboxes
// whose setting is above 0.
func (m *Manager) eraseAccount(ctx context.Context, acct store.Account, now time.Time) error {
	if acct.ReadOnly {
		return nil // a read-only account's mail is never erased
	}
	wanted := false
	for _, s := range eraseSettings {
		wanted = wanted || s.days(acct) > 0
	}
	if !wanted {
		return nil
	}
	mbs, err := m.db.ListMailboxes(ctx, acct.ID)
	if err != nil {
		return err
	}
	var first error
	for _, s := range eraseSettings {
		days := s.days(acct)
		if days <= 0 {
			continue
		}
		for _, mb := range mbs {
			if mb.Role != s.role {
				continue
			}
			n, err := m.eraseOldMailbox(ctx, acct, mb, now.Add(-time.Duration(days)*24*time.Hour))
			if errors.Is(err, ErrReadOnly) || errors.Is(err, store.ErrNotFound) {
				continue // the account or the mailbox is gone or read-only now
			}
			if err != nil {
				if ctx.Err() != nil {
					return err
				}
				m.log.Warn("old mail not erased", "account", acct.ID, "mailbox", mb.ID, "err", err)
				if first == nil {
					first = err
				}
				continue
			}
			if n == 0 {
				continue
			}
			m.log.Info("erased old mail", "account", acct.ID, "mailbox", mb.Path, "role", mb.Role, "messages", n, "days", days)
			m.kick(acct.ID)
		}
	}
	return first
}

// eraseOldMailbox erases at most eraseBatch of the messages of one mailbox
// that maild first saw there before the given time.
func (m *Manager) eraseOldMailbox(ctx context.Context, acct store.Account, mb store.Mailbox, before time.Time) (int, error) {
	n := 0
	err := m.db.Tx(ctx, func(tx *store.Tx) error {
		// The account may have become read-only since the list was read.
		if err := refuseReadOnly(ctx, tx, acct.ID); err != nil {
			return err
		}
		// The mailbox may have lost its role, or be gone, since then.
		cur, err := tx.GetMailbox(ctx, mb.ID)
		if err != nil {
			return err
		}
		if cur.Role != mb.Role {
			return nil
		}
		members, err := tx.OldMailboxMembers(ctx, mb.ID, before, eraseBatch)
		if err != nil {
			return err
		}
		n, err = eraseMembers(ctx, tx, cur, members)
		return err
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}
