package mailsync

import (
	"context"
	"slices"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/notify"
)

// announceNew shows the new unread inbox mail collected since the last
// announcement (docs/design/desktop.md, Notifications). Accounts with
// notifications off, and read-only accounts, announce nothing; neither does
// a folder's first sync, which never collects.
func (a *actor) announceNew(ctx context.Context) {
	ids := a.fresh
	a.fresh = nil
	if len(ids) == 0 || a.m.cfg.Announce == nil || !a.acct.Notify || a.acct.ReadOnly {
		return
	}
	inbox, err := a.m.db.MailboxByRole(ctx, a.acct.ID, api.MailboxRoleInbox)
	if err != nil {
		return
	}
	rows, err := a.m.db.Summaries(ctx, ids)
	if err != nil {
		a.m.log.Warn("announce", "account", a.acct.ID, "err", err)
		return
	}
	var mail []notify.Mail
	for _, r := range rows {
		if r.Flags.Seen || !slices.Contains(r.MailboxIDs, inbox.ID) {
			continue
		}
		mail = append(mail, notify.Mail{ID: r.ID, FromName: r.From.Name, FromAddr: r.From.Addr, Subject: r.Subject, Preview: r.Preview})
	}
	if len(mail) == 0 {
		return
	}
	if err := a.m.cfg.Announce(ctx, a.acct.ID, mail); err != nil {
		a.m.log.Warn("announce", "account", a.acct.ID, "err", err)
		return
	}
	a.m.log.Info("announced new mail", "account", a.acct.ID, "messages", len(mail))
}
