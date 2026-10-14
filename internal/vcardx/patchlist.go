package vcardx

import (
	"errors"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

var errListsNotYet = errors.New("vcardx: patching emails, phones, addresses and URLs comes with task card T-0133")

// listEdits returns the edits that make card's EMAIL, TEL, ADR and URL
// lines say f's lists, in a source whose lines end in eol; v4 is a vCard
// 4.0. It returns no edits when f sets no list.
func listEdits(src []byte, card *contentline.Component, f Fields, v4 bool, eol string) ([]contentline.Edit, error) {
	if f.Emails != nil || f.Phones != nil || f.Addresses != nil || f.URLs != nil {
		return nil, errListsNotYet
	}
	return nil, nil
}

// NewCard writes the vCard 3.0 people.create stores: a new contact with
// f's fields. It refuses what Patch refuses.
func NewCard(uid string, f Fields, now time.Time) ([]byte, error) {
	return nil, errListsNotYet
}
