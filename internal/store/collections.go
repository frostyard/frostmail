package store

import (
	"context"
	"errors"
	"time"

	"github.com/frostyard/frostmail/api"
)

// errNotYet marks M4.5 store functions a task card has yet to write.
var errNotYet = errors.New("store: not implemented yet")

// Collection is a collections row: an address book, calendar or Google
// Tasks list.
type Collection struct {
	ID          int64
	AccountID   int64
	Kind        api.CollectionKind
	Href        string // the collection's path, or the Google list's ID
	Name        string
	Description string
	Color       string   // #rrggbb, or ""
	Components  []string // calendars: VEVENT, VTODO; stored comma-separated
	ReadOnly    bool
	Enabled     bool
	IsDefault   bool
	Position    int
	SyncToken   string
	CTag        string
	SyncedAt    *time.Time
}

// RemoteCollection is a collection as the server lists it.
type RemoteCollection struct {
	Href        string
	Name        string
	Description string
	Color       string
	Components  []string
	ReadOnly    bool
}

// CollectionFilter selects collections; zero fields match every one.
type CollectionFilter struct {
	AccountID   int64
	Kind        api.CollectionKind
	EnabledOnly bool
}

// Task T-0061 writes the functions below.

// Collections returns the matching collections by account ID, then
// position, then ID.
func (d *DB) Collections(ctx context.Context, f CollectionFilter) ([]Collection, error) {
	return nil, errNotYet
}

// GetCollection returns one collection, or ErrNotFound.
func (d *DB) GetCollection(ctx context.Context, id int64) (Collection, error) {
	return Collection{}, errNotYet
}

// ReplaceCollections makes an account's collections of a kind match the
// server's list.
func (t *Tx) ReplaceCollections(ctx context.Context, accountID int64, kind api.CollectionKind, list []RemoteCollection) ([]Collection, error) {
	return nil, errNotYet
}

// SetCollectionSync records a collection's sync token and ctag at the end
// of its pass.
func (t *Tx) SetCollectionSync(ctx context.Context, id int64, token, ctag string) error {
	return errNotYet
}

// UpdateCollection changes the user's settings of a collection.
func (t *Tx) UpdateCollection(ctx context.Context, id int64, enabled *bool, makeDefault bool) (Collection, error) {
	return Collection{}, errNotYet
}
