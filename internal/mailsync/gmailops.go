package mailsync

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// Offline actions on a Gmail account (ADR-0012) act on each message's copy
// in All Mail, Spam or Trash. Moving between labels edits labels; moving
// to Spam or Trash, or out of them, moves the message; Starred follows
// \Flagged.

const opLabels = "labels"

// labelsOp adds and removes Gmail labels on the All Mail copies of messages.
type labelsOp struct {
	Messages []int64  `json:"messages"`
	Add      []string `json:"add"`
	Remove   []string `json:"remove"`
}

// gmailMove is Move on a Gmail account, inside Move's transaction, with the
// destination already checked. from 0 is the message's INBOX label when it
// has one, else the folder that holds it.
func (m *Manager) gmailMove(ctx context.Context, tx *store.Tx, acct int64, mems []store.Membership, from, to int64) error {
	mailboxes, err := tx.ListMailboxes(ctx, acct)
	if err != nil {
		return err
	}
	byID := map[int64]store.Mailbox{}
	for _, mb := range mailboxes {
		byID[mb.ID] = mb
	}
	names, err := tx.GmailLabelNames(ctx, acct)
	if err != nil {
		return err
	}
	dest := byID[to]
	inbox := byRole(mailboxes, api.MailboxRoleInbox)
	labelOps := map[string]*labelsOp{}
	moves := map[int64]*moveOp{} // by source folder
	touched := map[int64]bool{}
	var changed []int64
	for _, ms := range byMessage(mems) {
		id := ms[0].MessageID
		var synced *store.Membership
		have := map[int64]bool{}
		for i := range ms {
			have[ms[i].MailboxID] = true
			if isSynced(ms[i].Role) {
				synced = &ms[i]
			}
		}
		if synced == nil || synced.UID == 0 {
			continue // a move is still pending
		}
		src := from
		if src == 0 {
			src = synced.MailboxID
			if synced.Role == api.MailboxRoleAll && inbox != nil && have[inbox.ID] {
				src = inbox.ID
			}
		}
		// The source label, when the message leaves one.
		var leave []int64
		if byID[src].Label && have[src] && src != to {
			leave = []int64{src}
		}
		switch {
		case isSynced(dest.Role) && dest.ID != synced.MailboxID,
			!isSynced(dest.Role) && synced.Role != api.MailboxRoleAll:
			// Into Spam or Trash, back to All Mail, or out of Spam or Trash
			// into a label (Gmail restores it to All Mail with that label):
			// the message itself moves.
			op := moves[synced.MailboxID]
			if op == nil {
				op = &moveOp{From: synced.MailboxID, To: to}
				moves[synced.MailboxID] = op
			}
			op.Items = append(op.Items, moveItem{Message: id, UID: synced.UID})
			if err := tx.MoveMembership(ctx, id, synced.MailboxID, to); err != nil {
				return err
			}
			touched[synced.MailboxID], touched[to] = true, true
			if isSynced(dest.Role) {
				dropped, err := tx.DropGmailLabels(ctx, id)
				if err != nil {
					return err
				}
				for _, mb := range dropped {
					touched[mb] = true
				}
			}
		case isSynced(dest.Role):
			// Already in that folder: to All Mail from a label is Archive.
			if len(leave) == 0 {
				continue
			}
			if err := m.editLabels(ctx, tx, labelOps, names, id, nil, leave, touched); err != nil {
				return err
			}
		default:
			// Between labels: add the target, leave the source.
			var add []int64
			if !have[to] {
				add = []int64{to}
			}
			if len(add) == 0 && len(leave) == 0 {
				continue
			}
			if err := m.editLabels(ctx, tx, labelOps, names, id, add, leave, touched); err != nil {
				return err
			}
		}
		changed = append(changed, id)
	}
	for _, op := range moves {
		if _, err := tx.QueueOp(ctx, acct, opMove, op, op.messages()); err != nil {
			return err
		}
	}
	for _, op := range labelOps {
		if _, err := tx.QueueOp(ctx, acct, opLabels, op, op.Messages); err != nil {
			return err
		}
	}
	for mb := range touched {
		if err := tx.Emit(ctx, api.MailboxChanged{ID: mb, AccountID: acct}); err != nil {
			return err
		}
	}
	return emitChanged(ctx, tx, acct, changed)
}

// gmailCopy is Copy on a Gmail account, inside Copy's transaction, with the
// destination's account already checked: it adds the destination label to
// messages in All Mail. Spam, Trash and All Mail itself are not labels,
// Gmail files a message in a label only from All Mail, and Starred
// follows \Flagged.
func (m *Manager) gmailCopy(ctx context.Context, tx *store.Tx, acct int64, mems []store.Membership, to int64) error {
	mailboxes, err := tx.ListMailboxes(ctx, acct)
	if err != nil {
		return err
	}
	dest := byID(mailboxes, to)
	if dest == nil || !dest.Label {
		return fmt.Errorf("mailbox %d is not a Gmail label: %w", to, ErrInvalid)
	}
	if dest.Role == api.MailboxRoleFlagged {
		return fmt.Errorf("starred follows the flag; flag the messages instead: %w", ErrInvalid)
	}
	names, err := tx.GmailLabelNames(ctx, acct)
	if err != nil {
		return err
	}
	labelOps := map[string]*labelsOp{}
	touched := map[int64]bool{}
	var changed []int64
	for _, ms := range byMessage(mems) {
		inAll, have := false, false
		for _, mem := range ms {
			inAll = inAll || mem.Role == api.MailboxRoleAll && mem.UID != 0
			have = have || mem.MailboxID == to
		}
		if !inAll || have {
			continue // in Spam or Trash, a move pending, or already labeled
		}
		id := ms[0].MessageID
		if err := m.editLabels(ctx, tx, labelOps, names, id, []int64{to}, nil, touched); err != nil {
			return err
		}
		changed = append(changed, id)
	}
	for _, op := range labelOps {
		if _, err := tx.QueueOp(ctx, acct, opLabels, op, op.Messages); err != nil {
			return err
		}
	}
	for mb := range touched {
		if err := tx.Emit(ctx, api.MailboxChanged{ID: mb, AccountID: acct}); err != nil {
			return err
		}
	}
	return emitChanged(ctx, tx, acct, changed)
}

func byID(mbs []store.Mailbox, id int64) *store.Mailbox {
	for i := range mbs {
		if mbs[i].ID == id {
			return &mbs[i]
		}
	}
	return nil
}

// editLabels changes a message's labels locally and adds it to the queued
// label edit with the same names.
func (m *Manager) editLabels(ctx context.Context, tx *store.Tx, ops map[string]*labelsOp, names map[int64]string,
	id int64, add, remove []int64, touched map[int64]bool) error {
	mbs, err := tx.EditGmailLabels(ctx, id, add, remove)
	if err != nil {
		return err
	}
	for _, mb := range mbs {
		touched[mb] = true
	}
	op := labelsOp{Add: labelNames(names, add), Remove: labelNames(names, remove)}
	key := strings.Join(op.Add, "\x00") + "\x01" + strings.Join(op.Remove, "\x00")
	if ops[key] == nil {
		ops[key] = &op
	}
	ops[key].Messages = append(ops[key].Messages, id)
	return nil
}

func labelNames(names map[int64]string, ids []int64) []string {
	out := []string{}
	for _, id := range ids {
		if n, ok := names[id]; ok {
			out = append(out, n)
		}
	}
	return out
}

// byMessage groups memberships, which Memberships orders by message.
func byMessage(mems []store.Membership) [][]store.Membership {
	var out [][]store.Membership
	for i, mem := range mems {
		if i == 0 || mems[i-1].MessageID != mem.MessageID {
			out = append(out, nil)
		}
		out[len(out)-1] = append(out[len(out)-1], mem)
	}
	return out
}

// followStar puts the Gmail messages of mems in Starred, or takes them out,
// as Gmail does when \Flagged changes; messages in Spam and Trash are in no
// label. It returns the Starred mailbox when it changed.
func followStar(ctx context.Context, tx *store.Tx, acct int64, mems []store.Membership, starred bool) (int64, error) {
	star, err := tx.MailboxByRole(ctx, acct, api.MailboxRoleFlagged)
	if errors.Is(err, store.ErrNotFound) || err == nil && !star.Label {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var changed bool
	for _, ms := range byMessage(mems) {
		if !slices.ContainsFunc(ms, func(mem store.Membership) bool { return mem.Role == api.MailboxRoleAll && mem.UID != 0 }) {
			continue
		}
		add, remove := []int64{star.ID}, []int64(nil)
		if !starred {
			add, remove = nil, add
		}
		mbs, err := tx.EditGmailLabels(ctx, ms[0].MessageID, add, remove)
		if err != nil {
			return 0, err
		}
		changed = changed || len(mbs) > 0
	}
	if !changed {
		return 0, nil
	}
	return star.ID, nil
}

// replayLabels sends a label edit to the All Mail copies of its messages;
// messages no longer in All Mail are skipped.
func (a *actor) replayLabels(ctx context.Context, cmd conn, op store.Op) error {
	var p labelsOp
	if err := json.Unmarshal(op.Payload, &p); err != nil {
		return err
	}
	var mems []store.Membership
	if err := a.m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		mems, err = tx.Memberships(ctx, p.Messages)
		return err
	}); err != nil {
		return err
	}
	var path string
	var uids []uint32
	for _, mem := range mems {
		if mem.Role == api.MailboxRoleAll && mem.UID != 0 {
			path = mem.MailboxPath
			uids = append(uids, mem.UID)
		}
	}
	if len(uids) == 0 {
		return nil
	}
	if _, err := cmd.Select(ctx, path); err != nil {
		return err
	}
	return cmd.StoreLabels(ctx, uids, p.Add, p.Remove)
}
