package store

import "context"

// ContactIndex is what the contacts and contact_emails tables hold for one
// vCard (internal/vcardx builds it).
type ContactIndex struct {
	DisplayName  string
	SortKey      string
	GivenName    string
	FamilyName   string
	Organization string
	Emails       []ContactEmail
	Photo        []byte
	PhotoType    string
}

// ContactEmail is one of a contact's email addresses.
type ContactEmail struct {
	Email string
	Label string
}

// Task T-0061 writes the functions below.

// IndexContact replaces an object's contact rows.
func (t *Tx) IndexContact(ctx context.Context, objectID int64, c ContactIndex) error {
	return errNotYet
}

// RemoveContact removes an object's contact rows, for an object that is
// no longer a contact.
func (t *Tx) RemoveContact(ctx context.Context, objectID int64) error {
	return errNotYet
}

// Contact returns an object's contact index, or ErrNotFound when the
// object is not a contact.
func (d *DB) Contact(ctx context.Context, objectID int64) (ContactIndex, error) {
	return ContactIndex{}, errNotYet
}
