package mailsync

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// Rules (ADR-0024, docs/design/organize.md, Rules): maild runs the enabled
// rules, in order, on new inbox mail before announcing it, and on chosen
// messages for rule.apply. Each rule's conditions see the messages as they
// were before the run's actions; a rule with Stop Evaluating Rules takes
// the messages it matched out of the rest of the run.

// rulePlan is what the run does to one message.
type rulePlan struct {
	read   bool
	color  int     // flag with this color; 0 for none
	copies []int64 // mailboxes, in order, without repeats
	move   int64   // the last move's mailbox; 0 for none
	delete bool    // overrides any move
	notify bool
}

// planRules evaluates the enabled rules on ids and returns each matched
// message's plan.
func (m *Manager) planRules(ctx context.Context, ids []int64) (map[int64]*rulePlan, error) {
	rules, err := m.db.Rules(ctx)
	if err != nil {
		return nil, err
	}
	plans := map[int64]*rulePlan{}
	live := slices.Clone(ids)
	for _, r := range rules {
		if !r.Enabled || len(live) == 0 {
			continue
		}
		matched, err := m.db.MatchingIDs(ctx, r.Conditions, live, time.Local)
		if err != nil {
			m.log.Warn("rule skipped", "rule", r.ID, "err", err)
			continue
		}
		stop := false
		for _, id := range matched {
			p := plans[id]
			if p == nil {
				p = &rulePlan{}
				plans[id] = p
			}
			for _, a := range r.Actions {
				switch a.Kind {
				case api.RuleActionKindRead:
					p.read = true
				case api.RuleActionKindFlag:
					if a.Color != nil {
						p.color = int(*a.Color)
					}
				case api.RuleActionKindCopy:
					if a.MailboxID != nil && !slices.Contains(p.copies, *a.MailboxID) {
						p.copies = append(p.copies, *a.MailboxID)
					}
				case api.RuleActionKindMove:
					if a.MailboxID != nil {
						p.move = *a.MailboxID
					}
				case api.RuleActionKindDelete:
					p.delete = true
				case api.RuleActionKindNotify:
					p.notify = true
				case api.RuleActionKindStop:
					stop = true
				}
			}
		}
		if stop {
			live = slices.DeleteFunc(live, func(id int64) bool { return slices.Contains(matched, id) })
		}
	}
	return plans, nil
}

// applyPlans carries out plans in tx: read marks and flags, then copies,
// then each message's move or delete. A move or copy acts only on messages
// of its mailbox's account. It returns the accounts it queued ops for.
func (m *Manager) applyPlans(ctx context.Context, tx *store.Tx, plans map[int64]*rulePlan) ([]int64, error) {
	ids := slices.Sorted(maps.Keys(plans))
	if len(ids) == 0 {
		return nil, nil
	}
	mems, err := tx.Memberships(ctx, ids)
	if err != nil {
		return nil, err
	}
	accountOf := map[int64]int64{} // message → account
	for _, mem := range mems {
		accountOf[mem.MessageID] = mem.AccountID
	}
	mailboxes, err := tx.ListMailboxes(ctx, 0)
	if err != nil {
		return nil, err
	}
	mailboxAccount := map[int64]int64{}
	for _, mb := range mailboxes {
		mailboxAccount[mb.ID] = mb.AccountID
	}
	accounts := map[int64]bool{}
	add := func(accts ...int64) {
		for _, a := range accts {
			if a != 0 {
				accounts[a] = true
			}
		}
	}
	same := func(id, mailbox int64) bool {
		acct, ok := mailboxAccount[mailbox]
		return ok && acct == accountOf[id]
	}

	var read []int64
	byColor := map[int][]int64{}
	copies := map[int64][]int64{} // mailbox → messages
	moves := map[int64][]int64{}  // mailbox → messages
	var deletes []int64
	for _, id := range ids {
		p := plans[id]
		if p.read {
			read = append(read, id)
		}
		if p.color != 0 {
			byColor[p.color] = append(byColor[p.color], id)
		}
		for _, mb := range p.copies {
			if same(id, mb) {
				copies[mb] = append(copies[mb], id)
			}
		}
		switch {
		case p.delete:
			deletes = append(deletes, id)
		case p.move != 0 && same(id, p.move):
			moves[p.move] = append(moves[p.move], id)
		}
	}
	seen := true
	if len(read) > 0 {
		accts, err := m.setFlagsTx(ctx, tx, read, store.FlagChange{Seen: &seen})
		if err != nil {
			return nil, err
		}
		add(accts...)
	}
	flagged := true
	for _, color := range slices.Sorted(maps.Keys(byColor)) {
		accts, err := m.setFlagsTx(ctx, tx, byColor[color], store.FlagChange{Flagged: &flagged, Color: &color})
		if err != nil {
			return nil, err
		}
		add(accts...)
	}
	for _, mb := range slices.Sorted(maps.Keys(copies)) {
		acct, err := m.copyTx(ctx, tx, copies[mb], mb)
		if err != nil {
			return nil, err
		}
		add(acct)
	}
	for _, mb := range slices.Sorted(maps.Keys(moves)) {
		acct, err := m.moveTx(ctx, tx, moves[mb], 0, mb)
		if err != nil {
			return nil, err
		}
		add(acct)
	}
	if len(deletes) > 0 {
		accts, err := m.deleteTx(ctx, tx, deletes)
		if err != nil {
			return nil, err
		}
		add(accts...)
	}
	return slices.Sorted(maps.Keys(accounts)), nil
}

// runRules applies the rules to the account's waiting mail (ADR-0024) and
// remembers the messages a rule asked to announce. The transaction that
// acts also clears the waiting marks; when it fails, the marks are cleared
// on their own, so one bad message cannot hold the rules forever.
func (a *actor) runRules(ctx context.Context) {
	waiting, err := a.m.db.RulesWaiting(ctx, a.acct.ID)
	if err != nil || len(waiting) == 0 {
		if err != nil {
			a.m.log.Warn("rules", "account", a.acct.ID, "err", err)
		}
		return
	}
	plans, err := a.m.planRules(ctx, waiting)
	if err != nil {
		a.m.log.Warn("rules", "account", a.acct.ID, "err", err)
		plans = nil
	}
	var accounts []int64
	err = a.m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		if accounts, err = a.m.applyPlans(ctx, tx, plans); err != nil {
			return err
		}
		return tx.ClearRulesWaiting(ctx, waiting)
	})
	if err != nil {
		a.m.log.Warn("rules not applied; the messages stay as they came", "account", a.acct.ID, "err", err)
		if err := a.m.db.Tx(ctx, func(tx *store.Tx) error { return tx.ClearRulesWaiting(ctx, waiting) }); err != nil {
			a.m.log.Warn("rules", "account", a.acct.ID, "err", err)
		}
		return
	}
	for _, id := range slices.Sorted(maps.Keys(plans)) {
		if plans[id].notify {
			a.ruleNotify = append(a.ruleNotify, id)
		}
	}
	for _, acct := range accounts {
		a.m.kick(acct)
	}
	if len(plans) > 0 {
		a.m.log.Info("rules applied", "account", a.acct.ID, "messages", len(plans))
	}
}

// ApplyRules runs the enabled rules on chosen messages now (rule.apply) and
// returns how many met a rule's conditions. Messages of a read-only
// account are refused with ErrReadOnly.
func (m *Manager) ApplyRules(ctx context.Context, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	rows, err := m.db.Summaries(ctx, ids)
	if err != nil {
		return 0, err
	}
	if len(rows) != len(slices.Compact(slices.Sorted(slices.Values(ids)))) {
		return 0, fmt.Errorf("a message does not exist: %w", store.ErrNotFound)
	}
	for _, r := range rows {
		acct, err := m.db.GetAccount(ctx, r.AccountID)
		if err != nil {
			return 0, err
		}
		if acct.ReadOnly {
			return 0, fmt.Errorf("account %d: %w", acct.ID, ErrReadOnly)
		}
	}
	plans, err := m.planRules(ctx, ids)
	if err != nil {
		return 0, err
	}
	var accounts []int64
	err = m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		accounts, err = m.applyPlans(ctx, tx, plans)
		return err
	})
	if err != nil {
		return 0, err
	}
	for _, acct := range accounts {
		m.kick(acct)
	}
	return len(plans), nil
}
