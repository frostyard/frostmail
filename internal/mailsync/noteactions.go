package mailsync

import (
	"context"
	"fmt"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/notify"
	"github.com/frostyard/frostmail/internal/store"
)

// The buttons of a new-mail notification (docs/design/desktop.md,
// Notifications; parity P-805). The user clicks one without seeing where
// the message is now, perhaps long after it arrived, so a button only ever
// moves the message: it never erases one.

// archiveOf is where Archive moves an account's messages: its archive
// mailbox, or All Mail on an account whose mailboxes are Gmail labels; nil
// when the account has neither.
func archiveOf(mailboxes []store.Mailbox) *store.Mailbox {
	if mb := byRole(mailboxes, api.MailboxRoleArchive); mb != nil {
		return mb
	}
	for _, mb := range mailboxes {
		if mb.Label {
			return byRole(mailboxes, api.MailboxRoleAll)
		}
	}
	return nil
}

// noteButtons reports which of Archive and Trash a notification of the
// account's mail offers: each needs its mailbox.
func noteButtons(mailboxes []store.Mailbox) (archive, trash bool) {
	return archiveOf(mailboxes) != nil, byRole(mailboxes, api.MailboxRoleTrash) != nil
}

// ActOnAnnounced does what a notification's button asks of its message:
// notify.ActionArchive moves it to its account's archive and
// notify.ActionTrash to its Trash. It does nothing to a message that is
// gone or already in Trash, where a delete would erase it, and nothing on
// an account without the mailbox, where Trash would erase too. The check
// and the move are one transaction.
func (m *Manager) ActOnAnnounced(ctx context.Context, id int64, action string) error {
	if action != notify.ActionArchive && action != notify.ActionTrash {
		return fmt.Errorf("notification action %q: %w", action, ErrInvalid)
	}
	var accounts []int64
	err := m.db.Tx(ctx, func(tx *store.Tx) error {
		accounts = nil
		mems, err := tx.Memberships(ctx, []int64{id})
		if err != nil || len(mems) == 0 {
			return err
		}
		mailboxes, err := tx.ListMailboxes(ctx, mems[0].AccountID)
		if err != nil {
			return err
		}
		trash := byRole(mailboxes, api.MailboxRoleTrash)
		for _, mem := range mems {
			if trash != nil && mem.MailboxID == trash.ID {
				return nil
			}
		}
		if action == notify.ActionTrash {
			if trash == nil {
				return nil
			}
			accounts, err = m.deleteTx(ctx, tx, []int64{id})
			return err
		}
		archive := archiveOf(mailboxes)
		if archive == nil {
			return nil
		}
		acct, err := m.moveTx(ctx, tx, []int64{id}, 0, archive.ID)
		if acct != 0 {
			accounts = []int64{acct}
		}
		return err
	})
	if err != nil {
		return err
	}
	for _, acct := range accounts {
		m.kick(acct)
	}
	return nil
}
