package mailsync

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/notify"
	"github.com/frostyard/frostmail/internal/store"
)

// FireReminders fires the Remind Me reminders due by now (ADR-0025,
// docs/design/organize.md, Remind Me), including those that fell due while
// maild was stopped. For each account, one transaction drops the
// reminders, lists the messages at now, and moves those that left the
// inbox back to it (an op; a label edit on Gmail). The account's
// notifications then announce them with "Reminder" before the subject. A
// read-only account's messages come back to the top without moving.
func (m *Manager) FireReminders(ctx context.Context, now time.Time) error {
	due, err := m.db.DueMessageReminders(ctx, now)
	if err != nil || len(due) == 0 {
		return err
	}
	byAccount := map[int64][]int64{}
	for _, r := range due {
		byAccount[r.AccountID] = append(byAccount[r.AccountID], r.MessageID)
	}
	var firstErr error
	for _, acct := range slices.Sorted(maps.Keys(byAccount)) {
		if err := m.fireReminders(ctx, acct, byAccount[acct], now); err != nil {
			m.log.Warn("reminders", "account", acct, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (m *Manager) fireReminders(ctx context.Context, acct int64, ids []int64, now time.Time) error {
	account, err := m.db.GetAccount(ctx, acct)
	if err != nil {
		return err
	}
	moved := false
	err = m.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.FireMessageReminders(ctx, ids, now); err != nil {
			return err
		}
		if !account.ReadOnly {
			away, inbox, err := awayFromInbox(ctx, tx, acct, ids)
			if err != nil {
				return err
			}
			if len(away) > 0 {
				if _, err := m.moveTx(ctx, tx, away, 0, inbox); err != nil {
					return err
				}
				moved = true
			}
		}
		return tx.Emit(ctx, api.MessageChanged{AccountID: acct, IDs: ids})
	})
	if err != nil {
		return err
	}
	if moved {
		m.kick(acct)
	}
	m.log.Info("reminders fired", "account", acct, "messages", len(ids))
	return m.announceReminders(ctx, acct, ids)
}

// awayFromInbox is those of ids not in the account's inbox, and the inbox;
// none without one.
func awayFromInbox(ctx context.Context, tx *store.Tx, acct int64, ids []int64) ([]int64, int64, error) {
	mailboxes, err := tx.ListMailboxes(ctx, acct)
	if err != nil {
		return nil, 0, err
	}
	inbox := byRole(mailboxes, api.MailboxRoleInbox)
	if inbox == nil {
		return nil, 0, nil
	}
	mems, err := tx.Memberships(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	in := map[int64]bool{}
	for _, mem := range mems {
		if mem.MailboxID == inbox.ID {
			in[mem.MessageID] = true
		}
	}
	return slices.DeleteFunc(slices.Clone(ids), func(id int64) bool { return in[id] }), inbox.ID, nil
}

// announceReminders shows the fired reminders as notifications, whatever
// the notification scope: the user asked for them.
func (m *Manager) announceReminders(ctx context.Context, acct int64, ids []int64) error {
	if m.cfg.Announce == nil {
		return nil
	}
	rows, err := m.db.Summaries(ctx, ids)
	if err != nil {
		return err
	}
	mail := make([]notify.Mail, 0, len(rows))
	for _, r := range rows {
		mail = append(mail, notify.Mail{ID: r.ID, FromName: r.From.Name, FromAddr: r.From.Addr,
			Subject: "Reminder: " + r.Subject, Preview: r.Preview})
	}
	return m.cfg.Announce(ctx, acct, mail)
}
