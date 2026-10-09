// Package vcardx reads the fields Frostmail shows from vCards (RFC 2426
// version 3.0 and RFC 6350 version 4.0) over internal/contentline, and
// builds the vCard Add to Contacts creates (docs/design/pim.md).
package vcardx

import "errors"

// errNotYet marks what task card T-0060 has yet to write.
var errNotYet = errors.New("vcardx: not implemented yet")

// Card is what Frostmail reads from a vCard.
type Card struct {
	UID           string
	Kind          string // KIND (or X-ADDRESSBOOKSERVER-KIND), lowercased; "" means individual
	FormattedName string // FN
	FamilyName    string // N's fields
	GivenName     string
	MiddleName    string
	Prefix        string
	Suffix        string
	Nickname      string
	Organization  string // ORG's first field
	Department    string // ORG's other fields, joined by ", "
	Title         string
	Emails        []Labeled
	Phones        []Labeled
	Addresses     []Address
	URLs          []Labeled
	Birthday      string // YYYY-MM-DD, --MM-DD, or ""
	Note          string
	Photo         Photo
}

// Labeled is an email address, phone number or URL with its label.
type Labeled struct {
	Label string
	Value string
	Pref  bool
}

// Address is a postal address.
type Address struct {
	Label    string
	Street   string
	Locality string
	Region   string
	Postcode string
	Country  string
}

// Photo is a contact's photo: inline bytes, or a URI to fetch.
type Photo struct {
	Type string // the media type of Data, such as image/jpeg
	Data []byte
	URI  string
}

// Parse reads the first vCard in raw. Task T-0060 writes it.
func Parse(raw []byte) (*Card, error) {
	return nil, errNotYet
}

// DisplayName is the name to show for the card. Task T-0060 writes it.
func (c *Card) DisplayName() string { return "" }

// SortKey is the card's place in a list sorted by name. Task T-0060
// writes it.
func (c *Card) SortKey() string { return "" }

// New builds the vCard 3.0 Add to Contacts stores. Task T-0060 writes it.
func New(uid, name, email string) []byte { return nil }
