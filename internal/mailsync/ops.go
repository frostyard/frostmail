package mailsync

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// Offline actions (docs/design/sync.md): each call changes the store at once
// and queues an op in the same transaction; the account's actor replays
// queued ops against the server before every pass.

const (
	opFlags   = "flags"
	opMove    = "move"
	opExpunge = "expunge"
)

// flagsOp sets and clears IMAP flags on messages.
type flagsOp struct {
	Messages []int64  `json:"messages"`
	Add      []string `json:"add"`
	Remove   []string `json:"remove"`
}

// moveItem is one message and the UID it had in the source mailbox.
type moveItem struct {
	Message int64  `json:"message"`
	UID     uint32 `json:"uid"`
}

// moveOp moves messages between two mailboxes of an account.
type moveOp struct {
	From  int64      `json:"from"`
	To    int64      `json:"to"`
	Items []moveItem `json:"items"`
}

func (o *moveOp) messages() []int64 { return itemMessages(o.Items) }

// itemMessages lists the messages of move or expunge items.
func itemMessages(items []moveItem) []int64 {
	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.Message
	}
	return ids
}

// expungeOp permanently deletes messages from a mailbox (Trash).
type expungeOp struct {
	Mailbox int64      `json:"mailbox"`
	Items   []moveItem `json:"items"`
}

// SetFlags changes flags locally and queues the change for each account the
// messages belong to.
func (m *Manager) SetFlags(ctx context.Context, ids []int64, c store.FlagChange) error {
	add, remove := imapFlagChange(c)
	accounts := map[int64]bool{}
	err := m.db.Tx(ctx, func(tx *store.Tx) error {
		changed, err := tx.ChangeFlags(ctx, ids, c)
		if err != nil {
			return err
		}
		mems, err := tx.Memberships(ctx, ids)
		if err != nil {
			return err
		}
		byAccount := map[int64][]int64{}
		for _, mem := range mems {
			if len(byAccount[mem.AccountID]) == 0 || byAccount[mem.AccountID][len(byAccount[mem.AccountID])-1] != mem.MessageID {
				byAccount[mem.AccountID] = append(byAccount[mem.AccountID], mem.MessageID)
			}
		}
		for acct, msgs := range byAccount {
			if _, err := tx.QueueOp(ctx, acct, opFlags, flagsOp{Messages: msgs, Add: add, Remove: remove}, msgs); err != nil {
				return err
			}
			accounts[acct] = true
			if err := emitChanged(ctx, tx, acct, intersect(changed, msgs)); err != nil {
				return err
			}
		}
		// Changed flags move the counts of every mailbox holding the messages.
		touched := map[int64]bool{}
		for _, mem := range mems {
			if !touched[mem.MailboxID] && slices.Contains(changed, mem.MessageID) {
				touched[mem.MailboxID] = true
				if err := tx.Emit(ctx, api.MailboxChanged{ID: mem.MailboxID, AccountID: mem.AccountID}); err != nil {
					return err
				}
			}
		}
		return refreshThreadsOf(ctx, tx, changed)
	})
	if err != nil {
		return err
	}
	for acct := range accounts {
		m.kick(acct)
	}
	return nil
}

// imapFlagChange maps a FlagChange to IMAP flags to add and remove,
// including Mail.app's color bits.
func imapFlagChange(c store.FlagChange) (add, remove []string) {
	set := func(on bool, flag string) {
		if on {
			add = append(add, flag)
		} else {
			remove = append(remove, flag)
		}
	}
	if c.Seen != nil {
		set(*c.Seen, `\Seen`)
	}
	if c.Answered != nil {
		set(*c.Answered, `\Answered`)
	}
	if c.Flagged != nil && c.Color == nil {
		set(*c.Flagged, `\Flagged`)
	}
	if c.Color != nil {
		set(*c.Color > 0, `\Flagged`)
		bits := 0
		if *c.Color > 1 {
			bits = *c.Color - 1
		}
		for i, kw := range []string{"$MailFlagBit0", "$MailFlagBit1", "$MailFlagBit2"} {
			set(bits&(1<<i) != 0, kw)
		}
	}
	return add, remove
}

// Move moves messages to mailboxID, which must belong to their account.
func (m *Manager) Move(ctx context.Context, ids []int64, mailboxID int64) error {
	var acct int64
	err := m.db.Tx(ctx, func(tx *store.Tx) error {
		mems, err := tx.Memberships(ctx, ids)
		if err != nil {
			return err
		}
		ops := map[int64]*moveOp{} // by source mailbox
		var moved []int64
		for _, mem := range mems {
			if acct == 0 {
				acct = mem.AccountID
			}
			if mem.AccountID != acct {
				return fmt.Errorf("messages from more than one account: %w", ErrInvalid)
			}
			if mem.MailboxID == mailboxID || mem.UID == 0 {
				continue // already there, or a move is still pending
			}
			op := ops[mem.MailboxID]
			if op == nil {
				op = &moveOp{From: mem.MailboxID, To: mailboxID}
				ops[mem.MailboxID] = op
			}
			op.Items = append(op.Items, moveItem{Message: mem.MessageID, UID: mem.UID})
			if err := tx.MoveMembership(ctx, mem.MessageID, mem.MailboxID, mailboxID); err != nil {
				return err
			}
			moved = append(moved, mem.MessageID)
		}
		if acct != 0 && !ownsMailbox(ctx, tx, acct, mailboxID) {
			return fmt.Errorf("mailbox %d is not in account %d: %w", mailboxID, acct, ErrInvalid)
		}
		for _, op := range ops {
			if _, err := tx.QueueOp(ctx, acct, opMove, op, op.messages()); err != nil {
				return err
			}
			if err := tx.Emit(ctx, api.MailboxChanged{ID: op.From, AccountID: acct}); err != nil {
				return err
			}
		}
		if len(moved) > 0 {
			if err := tx.Emit(ctx, api.MailboxChanged{ID: mailboxID, AccountID: acct}); err != nil {
				return err
			}
		}
		return emitChanged(ctx, tx, acct, moved)
	})
	if err == nil && acct != 0 {
		m.kick(acct)
	}
	return err
}

// Delete moves messages to their account's Trash, or expunges those already
// there (and everything, on an account without a Trash mailbox).
func (m *Manager) Delete(ctx context.Context, ids []int64) error {
	accountOf := map[int64]int64{} // mailbox → account
	expunges := map[int64]*expungeOp{}
	moves := map[[2]int64]*moveOp{} // [from, trash]
	changed := map[int64][]int64{}  // account → messages
	err := m.db.Tx(ctx, func(tx *store.Tx) error {
		mems, err := tx.Memberships(ctx, ids)
		if err != nil {
			return err
		}
		var hidden []int64
		for _, mem := range mems {
			if mem.UID == 0 {
				continue // a move is pending; delete it after it lands
			}
			trash, err := trashOf(ctx, tx, mem.AccountID)
			if err != nil {
				return err
			}
			accountOf[mem.MailboxID] = mem.AccountID
			changed[mem.AccountID] = append(changed[mem.AccountID], mem.MessageID)
			item := moveItem{Message: mem.MessageID, UID: mem.UID}
			if trash == 0 || mem.MailboxID == trash {
				if expunges[mem.MailboxID] == nil {
					expunges[mem.MailboxID] = &expungeOp{Mailbox: mem.MailboxID}
				}
				expunges[mem.MailboxID].Items = append(expunges[mem.MailboxID].Items, item)
				hidden = append(hidden, mem.MessageID)
				continue
			}
			accountOf[trash] = mem.AccountID
			key := [2]int64{mem.MailboxID, trash}
			if moves[key] == nil {
				moves[key] = &moveOp{From: mem.MailboxID, To: trash}
			}
			moves[key].Items = append(moves[key].Items, item)
			if err := tx.MoveMembership(ctx, mem.MessageID, mem.MailboxID, trash); err != nil {
				return err
			}
		}
		if err := tx.MarkDeleted(ctx, hidden); err != nil {
			return err
		}
		for mailbox, op := range expunges {
			if _, err := tx.QueueOp(ctx, accountOf[mailbox], opExpunge, op, itemMessages(op.Items)); err != nil {
				return err
			}
		}
		for key, op := range moves {
			if _, err := tx.QueueOp(ctx, accountOf[key[0]], opMove, op, op.messages()); err != nil {
				return err
			}
		}
		for mailbox, acct := range accountOf {
			if err := tx.Emit(ctx, api.MailboxChanged{ID: mailbox, AccountID: acct}); err != nil {
				return err
			}
		}
		for acct, msgs := range changed {
			if err := emitChanged(ctx, tx, acct, msgs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for acct := range changed {
		m.kick(acct)
	}
	return nil
}

// ErrInvalid marks a request the caller must change.
var ErrInvalid = errors.New("mailsync: invalid request")

// kick tells an account's actor that ops are queued.
func (m *Manager) kick(accountID int64) {
	if a := m.actor(accountID); a != nil {
		select {
		case a.ops <- struct{}{}:
		default:
		}
	}
}

func ownsMailbox(ctx context.Context, tx *store.Tx, accountID, mailboxID int64) bool {
	var n int
	_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM mailboxes WHERE id = ? AND account_id = ?`, mailboxID, accountID).Scan(&n)
	return n == 1
}

func trashOf(ctx context.Context, tx *store.Tx, accountID int64) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM mailboxes WHERE account_id = ? AND role = 'trash' LIMIT 1`, accountID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func emitChanged(ctx context.Context, tx *store.Tx, accountID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return tx.Emit(ctx, api.MessageChanged{AccountID: accountID, IDs: ids})
}

func refreshThreadsOf(ctx context.Context, tx *store.Tx, ids []int64) error {
	threads, err := tx.ThreadsOf(ctx, ids)
	if err != nil {
		return err
	}
	return tx.RefreshThreads(ctx, threads)
}

func intersect(a, b []int64) []int64 {
	in := map[int64]bool{}
	for _, x := range b {
		in[x] = true
	}
	var out []int64
	for _, x := range a {
		if in[x] {
			out = append(out, x)
		}
	}
	return out
}

// replay sends queued ops to the server, oldest first. A refused op (a NO
// or BAD from the server) is marked failed and its local change undone by
// the next pass; any other error ends the session and the op is retried.
func (a *actor) replay(ctx context.Context, cmd *imapx.Session) error {
	ops, err := a.m.db.DueOps(ctx, a.acct.ID, time.Now())
	if err != nil {
		return err
	}
	for _, op := range ops {
		err := a.replayOne(ctx, cmd, op)
		var imapErr *imap.Error
		switch {
		case err == nil:
			if err := a.m.db.Tx(ctx, func(tx *store.Tx) error { return tx.DeleteOp(ctx, op.ID) }); err != nil {
				return err
			}
		case errors.As(err, &imapErr) || errors.Is(err, store.ErrNotFound):
			a.m.log.Warn("server refused an offline action", "account", a.acct.ID, "op", op.Kind, "err", err)
			if err := a.undo(ctx, op, err.Error()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("replay %s op %d: %w", op.Kind, op.ID, err)
		}
	}
	return nil
}

func (a *actor) replayOne(ctx context.Context, cmd *imapx.Session, op store.Op) error {
	switch op.Kind {
	case opFlags:
		var p flagsOp
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
		byPath := map[string][]uint32{}
		for _, mem := range mems {
			if mem.UID != 0 {
				byPath[mem.MailboxPath] = append(byPath[mem.MailboxPath], mem.UID)
			}
		}
		for path, uids := range byPath {
			if _, err := cmd.Select(ctx, path); err != nil {
				return err
			}
			if err := cmd.StoreFlags(ctx, uids, p.Add, p.Remove); err != nil {
				return err
			}
		}
		return nil
	case opMove:
		var p moveOp
		if err := json.Unmarshal(op.Payload, &p); err != nil {
			return err
		}
		from, err := a.mailboxPath(ctx, p.From)
		if err != nil {
			return err
		}
		to, err := a.mailboxPath(ctx, p.To)
		if err != nil {
			return err
		}
		if _, err := cmd.Select(ctx, from); err != nil {
			return err
		}
		uids := make([]uint32, len(p.Items))
		for i, it := range p.Items {
			uids[i] = it.UID
		}
		newUIDs, err := cmd.Move(ctx, uids, to)
		if err != nil {
			return err
		}
		return a.m.db.Tx(ctx, func(tx *store.Tx) error {
			for _, it := range p.Items {
				if uid := newUIDs[it.UID]; uid != 0 {
					if err := tx.SetMembership(ctx, it.Message, p.To, uid); err != nil {
						return err
					}
					continue
				}
				// No COPYUID: forget the local copy; the destination's next
				// pass fetches the message under its new UID.
				if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, it.Message); err != nil {
					return err
				}
				if err := tx.RemoveFromIndex(ctx, []int64{it.Message}); err != nil {
					return err
				}
				if err := tx.Emit(ctx, api.MessageRemoved{AccountID: a.acct.ID, IDs: []int64{it.Message}}); err != nil {
					return err
				}
			}
			// The moved messages now have their UIDs: they can be moved or
			// deleted again.
			return tx.Emit(ctx, api.MessageChanged{AccountID: a.acct.ID, IDs: itemIDs(p.Items)})
		})
	case opExpunge:
		var p expungeOp
		if err := json.Unmarshal(op.Payload, &p); err != nil {
			return err
		}
		path, err := a.mailboxPath(ctx, p.Mailbox)
		if err != nil {
			return err
		}
		if _, err := cmd.Select(ctx, path); err != nil {
			return err
		}
		uids := make([]uint32, len(p.Items))
		for i, it := range p.Items {
			uids[i] = it.UID
		}
		if err := cmd.StoreFlags(ctx, uids, []string{`\Deleted`}, nil); err != nil {
			return err
		}
		if cmd.Caps.UIDPlus {
			if err := cmd.Expunge(ctx, uids); err != nil {
				return err
			}
		}
		mb := store.Mailbox{ID: p.Mailbox, AccountID: a.acct.ID, Path: path}
		return a.remove(ctx, mb, uids)
	case opAppendSent:
		return a.replayAppendSent(ctx, cmd, op)
	case opRemoveCopy:
		return a.replayRemoveCopy(ctx, cmd, op)
	}
	return fmt.Errorf("unknown op kind %q", op.Kind)
}

// undo marks a refused op failed and restores the server's state locally.
func (a *actor) undo(ctx context.Context, op store.Op, reason string) error {
	return a.m.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.FailOp(ctx, op.ID, reason); err != nil {
			return err
		}
		switch op.Kind {
		case opMove:
			var p moveOp
			if err := json.Unmarshal(op.Payload, &p); err != nil {
				return err
			}
			for _, it := range p.Items {
				if err := tx.SetMembership(ctx, it.Message, p.From, it.UID); err != nil {
					return err
				}
			}
			return tx.Emit(ctx, api.MessageChanged{AccountID: a.acct.ID, IDs: itemIDs(p.Items)})
		case opExpunge:
			var p expungeOp
			if err := json.Unmarshal(op.Payload, &p); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE messages SET deleted = 0 WHERE id IN (SELECT message_id FROM message_mailbox WHERE mailbox_id = ?)`, p.Mailbox); err != nil {
				return err
			}
			return tx.ForgetFlagState(ctx, p.Mailbox)
		case opAppendSent:
			// The message went out; only its copy failed. Settle the row.
			var p appendSentOp
			if err := json.Unmarshal(op.Payload, &p); err != nil {
				return err
			}
			err := tx.MoveOutbox(ctx, p.Outbox, "accepted", "sent")
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) {
				return nil
			}
			if err != nil {
				return err
			}
			return tx.Emit(ctx, api.OutboxChanged{ID: p.Outbox, AccountID: a.acct.ID, State: api.OutboxStateSent})
		case opRemoveCopy:
			return nil // the copy stays; the next pass shows it
		default: // flags: the next pass refetches every flag
			rows, err := tx.QueryContext(ctx, `SELECT id FROM mailboxes WHERE account_id = ?`, a.acct.ID)
			if err != nil {
				return err
			}
			var ids []int64
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, id)
			}
			rows.Close()
			for _, id := range ids {
				if err := tx.ForgetFlagState(ctx, id); err != nil {
					return err
				}
			}
			return nil
		}
	})
}

func itemIDs(items []moveItem) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.Message
	}
	return out
}

func (a *actor) mailboxPath(ctx context.Context, id int64) (string, error) {
	mbs, err := a.m.db.ListMailboxes(ctx, a.acct.ID)
	if err != nil {
		return "", err
	}
	for _, mb := range mbs {
		if mb.ID == id {
			return mb.Path, nil
		}
	}
	return "", fmt.Errorf("mailbox %d: %w", id, store.ErrNotFound)
}
