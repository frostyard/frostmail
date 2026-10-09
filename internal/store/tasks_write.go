package store

import (
	"context"
	"database/sql"
	"fmt"
)

// TaskChange is the payload of a tasks.insert or tasks.patch op: the
// fields to send to Google, and for an insert the parent's object.
type TaskChange struct {
	Title     *string `json:"title,omitzero"`
	Notes     *string `json:"notes,omitzero"`
	Due       *string `json:"due,omitzero"` // YYYY-MM-DD, or "" to clear
	Completed *bool   `json:"completed,omitzero"`
	Parent    int64   `json:"parent,omitzero"`
}

// ReparentTasks points a list's subtasks at their parent's new UID (a task
// created here that Google gave an ID).
func (t *Tx) ReparentTasks(ctx context.Context, collectionID int64, oldUID, newUID string) error {
	_, err := t.ExecContext(ctx, `UPDATE tasks SET parent_uid = ? WHERE parent_uid = ? AND object_id IN
 (SELECT id FROM objects WHERE collection_id = ?)`, newUID, oldUID, collectionID)
	if err != nil {
		return fmt.Errorf("reparent tasks: %w", err)
	}
	return nil
}

// GmailThreadMessage returns the newest stored message of an account's
// Gmail thread, for a task made from mail; ErrNotFound when none is stored.
func (d *DB) GmailThreadMessage(ctx context.Context, accountID, gmThrID int64) (int64, error) {
	var id int64
	err := d.db.QueryRowContext(ctx, `SELECT id FROM messages WHERE account_id = ? AND gm_thrid = ?
 ORDER BY internal_date DESC, id DESC LIMIT 1`, accountID, gmThrID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("gmail thread message: %w", err)
	}
	return id, nil
}

// SetObjectHref moves an object to the href the server gave it (a task
// created here gets its ID from Google).
func (t *Tx) SetObjectHref(ctx context.Context, id int64, href string) error {
	res, err := t.ExecContext(ctx, `UPDATE objects SET href = ? WHERE id = ?`, href, id)
	if err != nil {
		return fmt.Errorf("set object href: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DropPIMOps removes an object's waiting changes, for an object deleted
// before the server ever had it, and returns how many there were.
func (t *Tx) DropPIMOps(ctx context.Context, objectID int64) (int, error) {
	res, err := t.ExecContext(ctx, `DELETE FROM pim_ops WHERE object_id = ? AND state != 'running'`, objectID)
	if err != nil {
		return 0, fmt.Errorf("drop pim ops: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
