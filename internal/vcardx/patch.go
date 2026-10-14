package vcardx

import (
	"errors"
	"time"
)

// Fields is a change to a contact (people.update's ContactFields, ADR-0030):
// nil fields keep their values, and a list replaces the whole list.
type Fields struct {
	GivenName    *string
	FamilyName   *string
	Nickname     *string
	Organization *string
	Title        *string
	Emails       *[]Labeled // Pref is ignored: a rewritten line keeps its own
	Phones       *[]Labeled
	Addresses    *[]Address
	URLs         *[]Labeled
	Birthday     *string // YYYY-MM-DD, --MM-DD, or "" to remove BDAY
	Note         *string // "" removes NOTE
	Photo        *Photo  // a Photo without Data removes PHOTO
}

var errPatchNotYet = errors.New("vcardx: Patch comes with task card T-0132")

// Patch applies f to the first VCARD of src, rewriting only the lines it
// touches and leaving every other byte as the server sent it (ADR-0018,
// ADR-0030).
func Patch(src []byte, f Fields, now time.Time) ([]byte, error) {
	if _, err := listEdits(src, nil, f, false, "\r\n"); err != nil {
		return nil, err
	}
	return nil, errPatchNotYet
}
