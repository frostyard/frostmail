package store

import (
	"context"
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

// Task T-0061 writes the functions below.

// ObjectETags returns the href and ETag of every object in a collection.
func (d *DB) ObjectETags(ctx context.Context, collectionID int64) (map[string]string, error) {
	return nil, errNotYet
}

// GetObject returns one object, or ErrNotFound.
func (d *DB) GetObject(ctx context.Context, id int64) (Object, error) {
	return Object{}, errNotYet
}

// ObjectByHref returns the object at href in a collection, or ErrNotFound.
func (d *DB) ObjectByHref(ctx context.Context, collectionID int64, href string) (Object, error) {
	return Object{}, errNotYet
}

// PendingHrefs returns the hrefs of a collection's objects that have
// changes waiting in pim_ops.
func (d *DB) PendingHrefs(ctx context.Context, collectionID int64) (map[string]bool, error) {
	return nil, errNotYet
}

// PutObject stores an object as the server sent it.
func (t *Tx) PutObject(ctx context.Context, o Object) (int64, error) {
	return 0, errNotYet
}

// DeleteObjects removes objects of a collection by href.
func (t *Tx) DeleteObjects(ctx context.Context, collectionID int64, hrefs []string) (int, error) {
	return 0, errNotYet
}
