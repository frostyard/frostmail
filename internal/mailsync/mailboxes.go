package mailsync

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// Mailbox operations (M5, docs/design/organize.md, Mailboxes): the store
// changes at once and the server follows when the op replays, as for
// message actions. A refused op is marked failed and the next full pass
// lists the server's mailboxes again, which puts the store right.

const (
	opMbCreate = "mbcreate"
	opMbRename = "mbrename"
	opMbDelete = "mbdelete"
)

// mailboxOp is a mailbox op's payload: the path, and a rename's new one.
type mailboxOp struct {
	Path string `json:"path"`
	To   string `json:"to,omitzero"`
}

// ErrConflict is an operation the store's state does not allow now.
var ErrConflict = errors.New("mailsync: conflict")

// maxMailboxName bounds a mailbox's own name, in characters.
const maxMailboxName = 100

// checkMailboxName checks a mailbox's own name: trimmed, 1-100 characters, no
// control characters, and not the delimiter.
func checkMailboxName(name, delim string) (string, error) {
	name = strings.TrimSpace(name)
	n := len([]rune(name))
	switch {
	case n == 0 || n > maxMailboxName:
		return "", fmt.Errorf("a mailbox's name must be 1 to %d characters: %w", maxMailboxName, ErrInvalid)
	case strings.ContainsFunc(name, unicode.IsControl):
		return "", fmt.Errorf("a mailbox's name cannot hold control characters: %w", ErrInvalid)
	case delim != "" && strings.Contains(name, delim):
		return "", fmt.Errorf("a mailbox's name cannot hold %q: %w", delim, ErrInvalid)
	}
	return name, nil
}

// fixed reports why a mailbox cannot be renamed, moved or deleted: it has a
// role, or it is one of Gmail's own folders.
func fixed(mb store.Mailbox) error {
	if mb.Role != api.MailboxRoleNone || strings.HasPrefix(mb.Path, "[Gmail]") {
		return fmt.Errorf("mailbox %q is one of the account's own: %w", mb.Path, ErrConflict)
	}
	return nil
}

// writable refuses a read-only account.
func (m *Manager) writable(ctx context.Context, accountID int64) (store.Account, error) {
	acct, err := m.db.GetAccount(ctx, accountID)
	if err != nil {
		return store.Account{}, err
	}
	if acct.ReadOnly {
		return store.Account{}, fmt.Errorf("account %d: %w", accountID, ErrReadOnly)
	}
	return acct, nil
}

// settledOps refuses while the account has changes waiting to reach the
// server, so that a rename or delete cannot overtake an earlier op that
// names the mailbox by its old path.
func (m *Manager) settledOps(ctx context.Context, accountID int64) error {
	busy, err := m.db.HasQueuedOps(ctx, accountID)
	if err != nil {
		return err
	}
	if busy {
		return fmt.Errorf("account %d has changes waiting to reach the server; try again when it is online: %w",
			accountID, ErrConflict)
	}
	return nil
}

// delimiterOf is the hierarchy delimiter of an account's mailboxes: a
// parent's, else any mailbox's.
func delimiterOf(mbs []store.Mailbox) string {
	for _, mb := range mbs {
		if mb.Delimiter != "" {
			return mb.Delimiter
		}
	}
	return ""
}

// childPath is the path of name inside parent, or at the top level.
func childPath(parent *store.Mailbox, name, delim string) string {
	if parent == nil {
		return name
	}
	return parent.Path + delim + name
}

// taken reports whether an account already has a mailbox at path; INBOX in
// any case is always taken.
func taken(mbs []store.Mailbox, path string) bool {
	if strings.EqualFold(path, "INBOX") {
		return true
	}
	for _, mb := range mbs {
		if mb.Path == path {
			return true
		}
	}
	return false
}

// parentOf reads a parent mailbox of the account; nil for none.
func (m *Manager) parentOf(ctx context.Context, accountID int64, parentID *int64) (*store.Mailbox, error) {
	if parentID == nil {
		return nil, nil
	}
	p, err := m.db.GetMailbox(ctx, *parentID)
	if err != nil {
		return nil, err
	}
	if p.AccountID != accountID {
		return nil, fmt.Errorf("mailbox %d is not in account %d: %w", p.ID, accountID, ErrInvalid)
	}
	return &p, nil
}

// CreateMailbox makes a mailbox at the top level or inside parentID.
func (m *Manager) CreateMailbox(ctx context.Context, accountID int64, name string, parentID *int64) (store.Mailbox, error) {
	acct, err := m.writable(ctx, accountID)
	if err != nil {
		return store.Mailbox{}, err
	}
	mbs, err := m.db.ListMailboxes(ctx, accountID)
	if err != nil {
		return store.Mailbox{}, err
	}
	parent, err := m.parentOf(ctx, accountID, parentID)
	if err != nil {
		return store.Mailbox{}, err
	}
	delim := delimiterOf(mbs)
	if parent != nil && parent.Delimiter != "" {
		delim = parent.Delimiter
	}
	if parent != nil && delim == "" {
		return store.Mailbox{}, fmt.Errorf("account %d's mailboxes cannot be nested: %w", accountID, ErrInvalid)
	}
	if name, err = checkMailboxName(name, delim); err != nil {
		return store.Mailbox{}, err
	}
	path := childPath(parent, name, delim)
	if taken(mbs, path) {
		return store.Mailbox{}, fmt.Errorf("a mailbox %q already exists: %w", path, ErrConflict)
	}
	gmail := m.isGmail(ctx, acct)
	var out store.Mailbox
	err = m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		if out, err = tx.CreateMailbox(ctx, accountID, path, delim, gmail); err != nil {
			return err
		}
		_, err = tx.QueueOp(ctx, accountID, opMbCreate, mailboxOp{Path: path}, nil)
		return err
	})
	if err != nil {
		return store.Mailbox{}, err
	}
	m.kick(accountID)
	return out, nil
}

// isGmail reports whether an account's mailboxes are Gmail labels.
func (m *Manager) isGmail(ctx context.Context, acct store.Account) bool {
	var gmail bool
	_ = m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		gmail, err = tx.GmailAccount(ctx, acct.ID)
		return err
	})
	return gmail
}

// RenameMailbox gives a mailbox a new name in the same place.
func (m *Manager) RenameMailbox(ctx context.Context, id int64, name string) (store.Mailbox, error) {
	mb, err := m.db.GetMailbox(ctx, id)
	if err != nil {
		return store.Mailbox{}, err
	}
	if name, err = checkMailboxName(name, mb.Delimiter); err != nil {
		return store.Mailbox{}, err
	}
	parent := ""
	if i := strings.LastIndex(mb.Path, mb.Delimiter); mb.Delimiter != "" && i >= 0 {
		parent = mb.Path[:i+len(mb.Delimiter)]
	}
	return m.relocate(ctx, mb, parent+name)
}

// MoveMailbox puts a mailbox inside parentID, or at the top level.
func (m *Manager) MoveMailbox(ctx context.Context, id int64, parentID *int64) (store.Mailbox, error) {
	mb, err := m.db.GetMailbox(ctx, id)
	if err != nil {
		return store.Mailbox{}, err
	}
	parent, err := m.parentOf(ctx, mb.AccountID, parentID)
	if err != nil {
		return store.Mailbox{}, err
	}
	if parent != nil {
		if parent.ID == mb.ID || (mb.Delimiter != "" && strings.HasPrefix(parent.Path, mb.Path+mb.Delimiter)) {
			return store.Mailbox{}, fmt.Errorf("a mailbox cannot go inside itself: %w", ErrConflict)
		}
		if mb.Delimiter == "" {
			return store.Mailbox{}, fmt.Errorf("account %d's mailboxes cannot be nested: %w", mb.AccountID, ErrInvalid)
		}
	}
	return m.relocate(ctx, mb, childPath(parent, mb.Name, mb.Delimiter))
}

// relocate renames a mailbox to path, here and then on the server.
func (m *Manager) relocate(ctx context.Context, mb store.Mailbox, path string) (store.Mailbox, error) {
	if _, err := m.writable(ctx, mb.AccountID); err != nil {
		return store.Mailbox{}, err
	}
	if err := fixed(mb); err != nil {
		return store.Mailbox{}, err
	}
	if path == mb.Path {
		return mb, nil
	}
	if err := m.settledOps(ctx, mb.AccountID); err != nil {
		return store.Mailbox{}, err
	}
	mbs, err := m.db.ListMailboxes(ctx, mb.AccountID)
	if err != nil {
		return store.Mailbox{}, err
	}
	if taken(mbs, path) {
		return store.Mailbox{}, fmt.Errorf("a mailbox %q already exists: %w", path, ErrConflict)
	}
	var out store.Mailbox
	err = m.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.RenameMailbox(ctx, mb.ID, path); err != nil {
			return err
		}
		if _, err := tx.QueueOp(ctx, mb.AccountID, opMbRename, mailboxOp{Path: mb.Path, To: path}, nil); err != nil {
			return err
		}
		var err error
		out, err = tx.GetMailbox(ctx, mb.ID)
		return err
	})
	if err != nil {
		return store.Mailbox{}, err
	}
	m.kick(mb.AccountID)
	return out, nil
}

// DeleteMailbox deletes a mailbox, those inside it, and their messages (on
// Gmail the labels; the messages stay in All Mail).
func (m *Manager) DeleteMailbox(ctx context.Context, id int64) error {
	mb, err := m.db.GetMailbox(ctx, id)
	if err != nil {
		return err
	}
	if _, err := m.writable(ctx, mb.AccountID); err != nil {
		return err
	}
	if err := fixed(mb); err != nil {
		return err
	}
	if err := m.settledOps(ctx, mb.AccountID); err != nil {
		return err
	}
	err = m.db.Tx(ctx, func(tx *store.Tx) error {
		paths, err := tx.DeleteMailboxTree(ctx, mb.ID)
		if err != nil {
			return err
		}
		for _, path := range paths { // deepest first
			if _, err := tx.QueueOp(ctx, mb.AccountID, opMbDelete, mailboxOp{Path: path}, nil); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.kick(mb.AccountID)
	return nil
}

// choosableRoles are the roles Use This Mailbox For offers.
var choosableRoles = []api.MailboxRole{api.MailboxRoleDrafts, api.MailboxRoleSent, api.MailboxRoleJunk,
	api.MailboxRoleTrash, api.MailboxRoleArchive}

// SetMailboxRole makes a mailbox its account's mailbox of role (Use This
// Mailbox For). Gmail's mailboxes keep their roles.
func (m *Manager) SetMailboxRole(ctx context.Context, id int64, role api.MailboxRole) (store.Mailbox, error) {
	ok := false
	for _, r := range choosableRoles {
		ok = ok || r == role
	}
	if !ok {
		return store.Mailbox{}, fmt.Errorf("%q is not a role a mailbox can be used for: %w", role, ErrInvalid)
	}
	mb, err := m.db.GetMailbox(ctx, id)
	if err != nil {
		return store.Mailbox{}, err
	}
	acct, err := m.writable(ctx, mb.AccountID)
	if err != nil {
		return store.Mailbox{}, err
	}
	if m.isGmail(ctx, acct) {
		return store.Mailbox{}, fmt.Errorf("account %d is Gmail, whose mailboxes keep their roles: %w", acct.ID, ErrConflict)
	}
	if mb.Role == api.MailboxRoleInbox {
		return store.Mailbox{}, fmt.Errorf("the inbox keeps its role: %w", ErrConflict)
	}
	var out store.Mailbox
	err = m.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetMailboxRole(ctx, id, role); err != nil {
			return err
		}
		var err error
		out, err = tx.GetMailbox(ctx, id)
		return err
	})
	return out, err
}

// EraseMailbox deletes every message of a trash or junk mailbox for good
// (Erase Deleted Items, Erase Junk Mail) and returns how many: they are
// hidden at once and expunged when the op replays.
func (m *Manager) EraseMailbox(ctx context.Context, id int64) (int, error) {
	mb, err := m.db.GetMailbox(ctx, id)
	if err != nil {
		return 0, err
	}
	if mb.Role != api.MailboxRoleTrash && mb.Role != api.MailboxRoleJunk {
		return 0, fmt.Errorf("only a trash or junk mailbox is erased: %w", ErrInvalid)
	}
	if _, err := m.writable(ctx, mb.AccountID); err != nil {
		return 0, err
	}
	n := 0
	err = m.db.Tx(ctx, func(tx *store.Tx) error {
		uids, err := tx.MailboxMembers(ctx, mb.ID)
		if err != nil {
			return err
		}
		op := &expungeOp{Mailbox: mb.ID}
		var ids []int64
		for _, it := range uids {
			if it.UID == 0 {
				continue // a move is still pending; it lands and stays
			}
			op.Items = append(op.Items, moveItem{Message: it.Message, UID: it.UID})
			ids = append(ids, it.Message)
		}
		n = len(ids)
		if n == 0 {
			return nil
		}
		if err := tx.MarkDeleted(ctx, ids); err != nil {
			return err
		}
		if _, err := tx.QueueOp(ctx, mb.AccountID, opExpunge, op, ids); err != nil {
			return err
		}
		if err := tx.Emit(ctx, api.MessageChanged{AccountID: mb.AccountID, IDs: ids}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.MailboxChanged{ID: mb.ID, AccountID: mb.AccountID})
	})
	if err != nil {
		return 0, err
	}
	if n > 0 {
		m.kick(mb.AccountID)
	}
	return n, nil
}

// replayMailbox sends a mailbox op to the server.
func (a *actor) replayMailbox(ctx context.Context, cmd conn, op store.Op) error {
	var p mailboxOp
	if err := json.Unmarshal(op.Payload, &p); err != nil {
		return err
	}
	switch op.Kind {
	case opMbCreate:
		return cmd.Create(ctx, p.Path)
	case opMbRename:
		return cmd.Rename(ctx, p.Path, p.To)
	default:
		return cmd.Delete(ctx, p.Path)
	}
}
