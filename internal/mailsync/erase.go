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

// eraseDays is one of the two erase settings and the role mailbox it is
// about.
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

// eraseAccount erases the old mail of each of one account's role mailboxes.
// The account and its settings are read again in each mailbox's transaction,
// so this list only says which accounts and mailboxes to look at.
func (m *Manager) eraseAccount(ctx context.Context, acct store.Account, now time.Time) error {
	if acct.ReadOnly {
		return nil // a read-only account's mail is never erased
	}
	mbs, err := m.db.ListMailboxes(ctx, acct.ID)
	if err != nil {
		return err
	}
	var first error
	for _, s := range eraseSettings {
		for _, mb := range mbs {
			if mb.Role != s.role {
				continue
			}
			n, days, err := m.eraseOldMailbox(ctx, acct.ID, s, mb, now)
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
// that maild first saw there before the account's setting allows, and returns
// how many it erased and the days it used.
func (m *Manager) eraseOldMailbox(ctx context.Context, accountID int64, s eraseDays, mb store.Mailbox,
	now time.Time,
) (n, days int, err error) {
	// A test changes the account's settings here, between the list read above
	// and the transaction below (erase_test.go, TestEraseOldReadsSettingsInTheTransaction).
	if m.eraseHook != nil {
		m.eraseHook(ctx, accountID)
	}
	err = m.db.Tx(ctx, func(tx *store.Tx) error {
		// The account's settings are read in this transaction, not before it:
		// an account.update between the list read and here must win, so that
		// the pass never erases with a setting the user has changed
		// (AGENTS.md, optional fields).
		acct, err := tx.GetAccount(ctx, accountID)
		if err != nil {
			return err
		}
		if acct.ReadOnly {
			return fmt.Errorf("account %d: %w", accountID, ErrReadOnly)
		}
		days = s.days(acct)
		if days <= 0 {
			return nil // Never: nothing is erased
		}
		// The mailbox may have lost its role, or be gone, since the list read.
		cur, err := tx.GetMailbox(ctx, mb.ID)
		if err != nil {
			return err
		}
		if cur.Role != s.role {
			return nil
		}
		before := now.Add(-time.Duration(days) * 24 * time.Hour)
		members, err := tx.OldMailboxMembers(ctx, mb.ID, before, eraseBatch)
		if err != nil {
			return err
		}
		n, err = eraseMembers(ctx, tx, cur, members)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	return n, days, nil
}
