package mailsync

import (
	"context"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/notify"
	"github.com/frostyard/frostmail/internal/store"
)

// announceNew shows the new unread mail collected since the last
// announcement that the notification scope takes (docs/design/organize.md,
// Notifications; docs/design/desktop.md). Accounts with notifications off,
// and read-only accounts, announce nothing; neither does a folder's first
// sync, which never collects.
func (a *actor) announceNew(ctx context.Context) {
	ids := a.fresh
	a.fresh = nil
	if len(ids) == 0 || a.m.cfg.Announce == nil || !a.acct.Notify || a.acct.ReadOnly {
		return
	}
	mail, err := a.inScope(ctx, ids)
	if err != nil {
		a.m.log.Warn("announce", "account", a.acct.ID, "err", err)
		return
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

// inScope is the unread mail of ids that the notification scope takes: in
// the inbox (also from a VIP, or from People), or for All Mailboxes in any
// mailbox but Junk, Trash, Sent and Drafts.
func (a *actor) inScope(ctx context.Context, ids []int64) ([]notify.Mail, error) {
	settings, err := a.m.db.Settings(ctx)
	if err != nil {
		return nil, err
	}
	scope := api.NotifyScope(settings.NotifyScope)
	mailboxes, err := a.m.db.ListMailboxes(ctx, a.acct.ID)
	if err != nil {
		return nil, err
	}
	roles := map[int64]api.MailboxRole{}
	for _, mb := range mailboxes {
		roles[mb.ID] = mb.Role
	}
	inMailbox := func(id int64) bool {
		role, ok := roles[id]
		if scope == api.NotifyScopeAll {
			return ok && role != api.MailboxRoleJunk && role != api.MailboxRoleTrash &&
				role != api.MailboxRoleSent && role != api.MailboxRoleDrafts
		}
		return role == api.MailboxRoleInbox
	}
	rows, err := a.m.db.Summaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	var kept []store.Summary
	for _, r := range rows {
		if !r.Flags.Seen && slices.ContainsFunc(r.MailboxIDs, inMailbox) {
			kept = append(kept, r)
		}
	}
	if sender, ok := map[api.NotifyScope]api.ConditionField{
		api.NotifyScopeVips: api.ConditionFieldVip, api.NotifyScopeContacts: api.ConditionFieldContact,
	}[scope]; ok && len(kept) > 0 {
		keptIDs := make([]int64, len(kept))
		for i, r := range kept {
			keptIDs[i] = r.ID
		}
		match, err := a.m.db.MatchingIDs(ctx, api.Conditions{Match: api.ConditionMatchAll,
			Conditions: []api.Condition{{Field: sender, Op: api.ConditionOpIs, Value: "true"}}}, keptIDs, time.Local)
		if err != nil {
			return nil, err
		}
		kept = slices.DeleteFunc(kept, func(r store.Summary) bool { return !slices.Contains(match, r.ID) })
	}
	mail := make([]notify.Mail, 0, len(kept))
	for _, r := range kept {
		mail = append(mail, notify.Mail{ID: r.ID, FromName: r.From.Name, FromAddr: r.From.Addr, Subject: r.Subject, Preview: r.Preview})
	}
	return mail, nil
}
