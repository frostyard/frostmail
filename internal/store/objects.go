package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ObjectKind is what an object's source holds.
type ObjectKind string

// The object kinds.
const (
	ObjectVCard  ObjectKind = "vcard"
	ObjectVEvent ObjectKind = "vevent"
	ObjectVTodo  ObjectKind = "vtodo"
	ObjectGTask  ObjectKind = "gtask"
	ObjectOther  ObjectKind = "other"
)

// Object is an objects row: a contact, calendar object or Google task as
// the server sent it (ADR-0018).
type Object struct {
	ID           int64
	CollectionID int64
	Href         string
	ETag         string
	Kind         ObjectKind
	UID          string
	Raw          []byte
	ParseError   string
	UpdatedAt    time.Time
}

// ObjectETags returns every object's exact href and ETag in the collection.
func (d *DB) ObjectETags(ctx context.Context, collectionID int64) (map[string]string, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT href, etag FROM objects WHERE collection_id = ?", collectionID)
	if err != nil {
		return nil, fmt.Errorf("object etags: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var href, etag string
		if err := rows.Scan(&href, &etag); err != nil {
			return nil, fmt.Errorf("object etags: %w", err)
		}
		out[href] = etag
	}
	return out, rows.Err()
}

const objectColumns = "id, collection_id, href, etag, kind, uid, raw, parse_error, updated_at"

func readObject(row *sql.Row) (Object, error) {
	var o Object
	var at string
	err := row.Scan(&o.ID, &o.CollectionID, &o.Href, &o.ETag, &o.Kind, &o.UID, &o.Raw, &o.ParseError, &at)
	if err == sql.ErrNoRows {
		return o, ErrNotFound
	}
	if err != nil {
		return o, fmt.Errorf("read object: %w", err)
	}
	o.UpdatedAt, err = ParseTime(at)
	return o, err
}

// GetObject returns one object, or ErrNotFound.
func (d *DB) GetObject(ctx context.Context, id int64) (Object, error) {
	return readObject(d.db.QueryRowContext(ctx, "SELECT "+objectColumns+" FROM objects WHERE id = ?", id))
}

// ObjectByHref returns the object at the exact href, or ErrNotFound.
func (d *DB) ObjectByHref(ctx context.Context, collectionID int64, href string) (Object, error) {
	return readObject(d.db.QueryRowContext(ctx, "SELECT "+objectColumns+" FROM objects WHERE collection_id = ? AND href = ?", collectionID, href))
}

// PendingHrefs returns a set of hrefs with queued or running local operations.
func (d *DB) PendingHrefs(ctx context.Context, collectionID int64) (map[string]bool, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT href FROM pim_ops WHERE collection_id = ? AND state IN ('queued', 'running')", collectionID)
	if err != nil {
		return nil, fmt.Errorf("pending hrefs: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var href string
		if err := rows.Scan(&href); err != nil {
			return nil, fmt.Errorf("pending hrefs: %w", err)
		}
		out[href] = true
	}
	return out, rows.Err()
}

// PutObject stores source and metadata, preserving the ID at an existing href.
func (t *Tx) PutObject(ctx context.Context, o Object) (int64, error) {
	var id int64
	err := t.QueryRowContext(ctx, `INSERT INTO objects
 (collection_id, href, etag, kind, uid, raw, parse_error, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT(collection_id, href) DO UPDATE SET etag = excluded.etag, kind = excluded.kind,
 uid = excluded.uid, raw = excluded.raw, parse_error = excluded.parse_error, updated_at = excluded.updated_at
 RETURNING id`, o.CollectionID, o.Href, o.ETag, o.Kind, o.UID, o.Raw, o.ParseError, FormatTime(t.Now())).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("put object: %w", err)
	}
	return id, nil
}

// DeleteObjects removes the named objects and returns how many existed.
func (t *Tx) DeleteObjects(ctx context.Context, collectionID int64, hrefs []string) (int, error) {
	var count int
	for _, href := range hrefs {
		res, err := t.ExecContext(ctx, "DELETE FROM objects WHERE collection_id = ? AND href = ?", collectionID, href)
		if err != nil {
			return 0, fmt.Errorf("delete object: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("delete object count: %w", err)
		}
		count += int(n)
	}
	return count, nil
}
