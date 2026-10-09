package store

import (
	"context"
	"errors"
)

// errNotYet marks what task card T-0062 has yet to write.
var errNotYet = errors.New("store: not implemented yet")

// PersonRow is a person as the People list and contact cards show them.
type PersonRow struct {
	ID           int64
	DisplayName  string
	SortKey      string
	Organization string
	Email        string // the first email of the person's first contact, or ""
	HasPhoto     bool
	ContactIDs   []int64 // the object IDs of the person's contacts, ascending
}

// Task T-0062 writes the functions below.

// RelinkPeople rebuilds people from the contacts.
func (t *Tx) RelinkPeople(ctx context.Context) error {
	return errNotYet
}

// People returns the people matching a query, sorted by name.
func (d *DB) People(ctx context.Context, query string) ([]PersonRow, error) {
	return nil, errNotYet
}

// Person returns one person, or ErrNotFound.
func (d *DB) Person(ctx context.Context, id int64) (PersonRow, error) {
	return PersonRow{}, errNotYet
}

// PersonByEmail returns the person with an email address, or ErrNotFound.
func (d *DB) PersonByEmail(ctx context.Context, email string) (PersonRow, error) {
	return PersonRow{}, errNotYet
}

// PersonPhoto returns a person's photo, or ErrNotFound.
func (d *DB) PersonPhoto(ctx context.Context, id int64) ([]byte, string, error) {
	return nil, "", errNotYet
}

// SuggestContacts returns contacts' addresses matching a typed prefix.
func (d *DB) SuggestContacts(ctx context.Context, prefix string, limit int) ([]Address, error) {
	return nil, errNotYet
}

// MessagesWithAddress returns the newest messages from or to an address.
func (d *DB) MessagesWithAddress(ctx context.Context, email string, limit int) ([]int64, error) {
	return nil, errNotYet
}

// SeenName returns the name last seen with an address in mail.
func (d *DB) SeenName(ctx context.Context, email string) (string, error) {
	return "", errNotYet
}

// WritableAddressBooks returns the address books a new contact can go in.
func (d *DB) WritableAddressBooks(ctx context.Context) ([]Collection, error) {
	return nil, errNotYet
}
