package store

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// Mailbox operations made here (M5, docs/design/organize.md, Mailboxes):
// mailsync changes the store at once and queues the server's side as ops.

// GetMailbox returns one mailbox with its counts, or ErrNotFound.
func (d *DB) GetMailbox(ctx context.Context, id int64) (Mailbox, error) {
	return getMailbox(ctx, d.db, id)
}

// GetMailbox is DB.GetMailbox inside the transaction.
func (t *Tx) GetMailbox(ctx context.Context, id int64) (Mailbox, error) {
	return getMailbox(ctx, t, id)
}

func getMailbox(ctx context.Context, q querier, id int64) (Mailbox, error) {
	rows, err := q.QueryContext(ctx, `SELECT account_id FROM mailboxes WHERE id = ?`, id)
	if err != nil {
		return Mailbox{}, fmt.Errorf("mailbox %d: %w", id, err)
	}
	var acct int64
	found := rows.Next()
	if found {
		err = rows.Scan(&acct)
	}
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Mailbox{}, fmt.Errorf("mailbox %d: %w", id, err)
	}
	if !found {
		return Mailbox{}, fmt.Errorf("mailbox %d: %w", id, ErrNotFound)
	}
	mbs, err := listMailboxes(ctx, q, acct)
	if err != nil {
		return Mailbox{}, err
	}
	for _, mb := range mbs {
		if mb.ID == id {
			return mb, nil
		}
	}
	return Mailbox{}, fmt.Errorf("mailbox %d: %w", id, ErrNotFound)
}

// CreateMailbox stores a mailbox made here, before the server has it: an
// ordinary selectable mailbox (a label on Gmail) with no sync state yet.
// It emits api.MailboxChanged.
func (t *Tx) CreateMailbox(ctx context.Context, accountID int64, path, delim string, label bool) (Mailbox, error) {
	var id int64
	err := t.QueryRowContext(ctx, `INSERT INTO mailboxes
		(account_id, path, delimiter, name, role, attrs_json, selectable, subscribed, is_gmail_label)
		VALUES (?, ?, ?, ?, 'none', '[]', 1, 1, ?) RETURNING id`,
		accountID, path, delim, mailboxName(path, delim), bit(label)).Scan(&id)
	if err != nil {
		return Mailbox{}, fmt.Errorf("create mailbox %q: %w", path, err)
	}
	if err := t.Emit(ctx, api.MailboxChanged{ID: id, AccountID: accountID}); err != nil {
		return Mailbox{}, err
	}
	return t.GetMailbox(ctx, id)
}

// subtree is a mailbox and those inside it (their paths start with its path
// and the delimiter), shallowest first.
func (t *Tx) subtree(ctx context.Context, mb Mailbox) ([]Mailbox, error) {
	all, err := t.ListMailboxes(ctx, mb.AccountID)
	if err != nil {
		return nil, err
	}
	out := []Mailbox{mb}
	if mb.Delimiter != "" {
		for _, o := range all {
			if strings.HasPrefix(o.Path, mb.Path+mb.Delimiter) {
				out = append(out, o)
			}
		}
	}
	slices.SortStableFunc(out[1:], func(a, b Mailbox) int { return strings.Count(a.Path, a.Delimiter) - strings.Count(b.Path, b.Delimiter) })
	return out, nil
}

// RenameMailbox gives a mailbox the path newPath; the mailboxes inside it
// and the account's role choices follow. It emits api.MailboxChanged for
// each.
func (t *Tx) RenameMailbox(ctx context.Context, id int64, newPath string) error {
	mb, err := t.GetMailbox(ctx, id)
	if err != nil {
		return err
	}
	tree, err := t.subtree(ctx, mb)
	if err != nil {
		return err
	}
	for _, o := range tree {
		path := newPath + strings.TrimPrefix(o.Path, mb.Path)
		if _, err := t.ExecContext(ctx, `UPDATE mailboxes SET path = ?, name = ? WHERE id = ?`,
			path, mailboxName(path, o.Delimiter), o.ID); err != nil {
			return fmt.Errorf("rename mailbox %d: %w", o.ID, err)
		}
		if _, err := t.ExecContext(ctx, `UPDATE mailbox_roles SET path = ? WHERE account_id = ? AND path = ?`,
			path, o.AccountID, o.Path); err != nil {
			return fmt.Errorf("rename mailbox %d: %w", o.ID, err)
		}
		if err := t.Emit(ctx, api.MailboxChanged{ID: o.ID, AccountID: o.AccountID}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteMailboxTree deletes a mailbox and those inside it, with the
// messages left in no mailbox and the account's role choices for them. It
// returns their paths, deepest first, and emits as ReplaceMailboxes does.
func (t *Tx) DeleteMailboxTree(ctx context.Context, id int64) ([]string, error) {
	mb, err := t.GetMailbox(ctx, id)
	if err != nil {
		return nil, err
	}
	tree, err := t.subtree(ctx, mb)
	if err != nil {
		return nil, err
	}
	slices.Reverse(tree)
	paths := make([]string, 0, len(tree))
	for _, o := range tree {
		if err := t.deleteMailbox(ctx, o.AccountID, o.ID); err != nil {
			return nil, err
		}
		if _, err := t.ExecContext(ctx, `DELETE FROM mailbox_roles WHERE account_id = ? AND path = ?`, o.AccountID, o.Path); err != nil {
			return nil, fmt.Errorf("delete mailbox %d: %w", o.ID, err)
		}
		paths = append(paths, o.Path)
	}
	return paths, nil
}

// SetMailboxRole makes a mailbox its account's mailbox of role, over the
// server's choice, now and at every later listing; the mailbox that had the
// role loses it. It emits api.MailboxChanged for both.
func (t *Tx) SetMailboxRole(ctx context.Context, id int64, role api.MailboxRole) error {
	mb, err := t.GetMailbox(ctx, id)
	if err != nil {
		return err
	}
	if _, err := t.ExecContext(ctx, `INSERT INTO mailbox_roles (account_id, role, path) VALUES (?, ?, ?)
		ON CONFLICT (account_id, role) DO UPDATE SET path = excluded.path`, mb.AccountID, role, mb.Path); err != nil {
		return fmt.Errorf("set mailbox role: %w", err)
	}
	rows, err := t.QueryContext(ctx, `UPDATE mailboxes SET role = 'none'
		WHERE account_id = ? AND role = ? AND id <> ? RETURNING id`, mb.AccountID, role, id)
	if err != nil {
		return fmt.Errorf("set mailbox role: %w", err)
	}
	var changed []int64
	for rows.Next() {
		var other int64
		if err := rows.Scan(&other); err != nil {
			rows.Close()
			return fmt.Errorf("set mailbox role: %w", err)
		}
		changed = append(changed, other)
	}
	rows.Close()
	if _, err := t.ExecContext(ctx, `UPDATE mailboxes SET role = ? WHERE id = ?`, role, id); err != nil {
		return fmt.Errorf("set mailbox role: %w", err)
	}
	for _, other := range append(changed, id) {
		if err := t.Emit(ctx, api.MailboxChanged{ID: other, AccountID: mb.AccountID}); err != nil {
			return err
		}
	}
	return nil
}

// applyRoleChoices puts an account's role choices over the server's roles
// in a listing: the chosen path takes the role, any other loses it. A
// choice whose mailbox is not listed leaves the server's roles alone.
func (t *Tx) applyRoleChoices(ctx context.Context, accountID int64, list []ServerMailbox) error {
	rows, err := t.QueryContext(ctx, `SELECT role, path FROM mailbox_roles WHERE account_id = ?`, accountID)
	if err != nil {
		return fmt.Errorf("mailbox roles: %w", err)
	}
	chosen := map[api.MailboxRole]string{}
	for rows.Next() {
		var role, path string
		if err := rows.Scan(&role, &path); err != nil {
			rows.Close()
			return fmt.Errorf("mailbox roles: %w", err)
		}
		chosen[api.MailboxRole(role)] = path
	}
	rows.Close()
	for role, path := range chosen {
		if !slices.ContainsFunc(list, func(sm ServerMailbox) bool { return sm.Path == path }) {
			continue
		}
		for i := range list {
			switch {
			case list[i].Path == path:
				list[i].Role = role
			case list[i].Role == role:
				list[i].Role = api.MailboxRoleNone
			}
		}
	}
	return nil
}

// HasQueuedOps reports whether an account has offline actions not yet
// replayed or failed.
func (d *DB) HasQueuedOps(ctx context.Context, accountID int64) (bool, error) {
	var n int
	if err := d.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pending_ops WHERE account_id = ? AND state = 'queued')`,
		accountID).Scan(&n); err != nil {
		return false, fmt.Errorf("queued ops: %w", err)
	}
	return n == 1, nil
}

// MailboxMember is a message in a mailbox and its UID there; 0 while a
// move into the mailbox is pending.
type MailboxMember struct {
	Message int64
	UID     uint32
}

// MailboxMembers returns the messages in a mailbox that are not marked
// deleted, by message ID.
func (t *Tx) MailboxMembers(ctx context.Context, mailboxID int64) ([]MailboxMember, error) {
	rows, err := t.QueryContext(ctx, `SELECT mm.message_id, COALESCE(mm.uid, 0) FROM message_mailbox mm
		JOIN messages m ON m.id = mm.message_id WHERE mm.mailbox_id = ? AND m.deleted = 0 ORDER BY mm.message_id`, mailboxID)
	if err != nil {
		return nil, fmt.Errorf("mailbox members: %w", err)
	}
	defer rows.Close()
	var out []MailboxMember
	for rows.Next() {
		var mm MailboxMember
		var uid int64
		if err := rows.Scan(&mm.Message, &uid); err != nil {
			return nil, fmt.Errorf("mailbox members: %w", err)
		}
		mm.UID = uint32(uid)
		out = append(out, mm)
	}
	return out, rows.Err()
}
