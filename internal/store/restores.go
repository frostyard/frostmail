package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RestoreObject is one object a contact or event write touched, as it was
// before (ADR-0032).
type RestoreObject struct {
	CollectionID int64
	Href         string
	UID          string
	Raw          []byte // nil when the write created the object
}

// Restore is what one write replaced, for people.restore and
// calendar.restore.
type Restore struct {
	ID        string
	AccountID int64
	CreatedAt time.Time
	Objects   []RestoreObject
}

// RecordRestore keeps what a write replaced, dated now.
func (t *Tx) RecordRestore(ctx context.Context, r Restore) error {
	if _, err := t.ExecContext(ctx, `INSERT INTO pim_restores (id, account_id, created_at) VALUES (?, ?, ?)`,
		r.ID, r.AccountID, FormatTime(t.Now())); err != nil {
		return fmt.Errorf("record restore: %w", err)
	}
	for i, o := range r.Objects {
		if _, err := t.ExecContext(ctx, `INSERT INTO pim_restore_objects
			(restore_id, position, collection_id, href, uid, raw) VALUES (?, ?, ?, ?, ?, ?)`,
			r.ID, i, o.CollectionID, o.Href, o.UID, o.Raw); err != nil {
			return fmt.Errorf("record restore object: %w", err)
		}
	}
	return nil
}

// TakeRestore reads a restore and removes it, so it runs once: ErrNotFound
// when there is none (never made, used, or dropped after a day).
func (t *Tx) TakeRestore(ctx context.Context, id string) (Restore, error) {
	r := Restore{ID: id}
	var created string
	err := t.QueryRowContext(ctx, `SELECT account_id, created_at FROM pim_restores WHERE id = ?`, id).
		Scan(&r.AccountID, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Restore{}, ErrNotFound
	}
	if err != nil {
		return Restore{}, fmt.Errorf("take restore: %w", err)
	}
	if r.CreatedAt, err = ParseTime(created); err != nil {
		return Restore{}, err
	}
	rows, err := t.QueryContext(ctx, `SELECT collection_id, href, uid, raw FROM pim_restore_objects
		WHERE restore_id = ? ORDER BY position`, id)
	if err != nil {
		return Restore{}, fmt.Errorf("take restore: %w", err)
	}
	for rows.Next() {
		var o RestoreObject
		if err := rows.Scan(&o.CollectionID, &o.Href, &o.UID, &o.Raw); err != nil {
			_ = rows.Close()
			return Restore{}, fmt.Errorf("take restore: %w", err)
		}
		r.Objects = append(r.Objects, o)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return Restore{}, fmt.Errorf("take restore: %w", err)
	}
	if _, err := t.ExecContext(ctx, `DELETE FROM pim_restores WHERE id = ?`, id); err != nil {
		return Restore{}, fmt.Errorf("take restore: %w", err)
	}
	return r, nil
}

// DropRestoresBefore removes the restores made before a time and returns
// how many there were.
func (t *Tx) DropRestoresBefore(ctx context.Context, before time.Time) (int, error) {
	res, err := t.ExecContext(ctx, `DELETE FROM pim_restores WHERE created_at < ?`, FormatTime(before))
	if err != nil {
		return 0, fmt.Errorf("drop restores: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// QueuedPut returns the put waiting for an object (queued, not running),
// whose payload a later edit extends.
func (t *Tx) QueuedPut(ctx context.Context, objectID int64) (PIMOp, bool, error) {
	op := PIMOp{ObjectID: &objectID, Kind: "put"}
	err := t.QueryRowContext(ctx, `SELECT id, account_id, collection_id, href, if_match, payload_json
		FROM pim_ops WHERE object_id = ? AND kind = 'put' AND state = 'queued' ORDER BY id LIMIT 1`, objectID).
		Scan(&op.ID, &op.AccountID, &op.CollectionID, &op.Href, &op.IfMatch, &op.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return PIMOp{}, false, nil
	}
	if err != nil {
		return PIMOp{}, false, fmt.Errorf("queued put: %w", err)
	}
	return op, true, nil
}

// SetPIMOpPayload replaces a waiting change's payload.
func (t *Tx) SetPIMOpPayload(ctx context.Context, id int64, payload string) error {
	if _, err := t.ExecContext(ctx, `UPDATE pim_ops SET payload_json = ? WHERE id = ?`, payload, id); err != nil {
		return fmt.Errorf("set pim op payload: %w", err)
	}
	return nil
}

// PersonOfContact returns the person a contact belongs to, or ErrNotFound.
func (d *DB) PersonOfContact(ctx context.Context, objectID int64) (PersonRow, error) {
	var id sql.NullInt64
	err := d.db.QueryRowContext(ctx, `SELECT person_id FROM contacts WHERE object_id = ?`, objectID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !id.Valid {
		return PersonRow{}, ErrNotFound
	}
	if err != nil {
		return PersonRow{}, fmt.Errorf("person of contact: %w", err)
	}
	return d.Person(ctx, id.Int64)
}
