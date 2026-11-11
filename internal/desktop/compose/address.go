package compose

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/frostyard/frostmail/api"
)

// An address's parts hold no white space or control character, and none of
// the characters that split or quote a recipient field's text.
var (
	angleForm   = regexp.MustCompile(`^(.*)<([^<>]*)>$`)
	addressForm = regexp.MustCompile(`^[^@\s\x00-\x1f\x7f,;"<>]+@[^@\s.\x00-\x1f\x7f,;"<>]+(\.[^@\s.\x00-\x1f\x7f,;"<>]+)+$`)
)

// fieldSpecials are the characters that mean something in a recipient
// field's text: RFC 5322's specials, whose name must be quoted.
const fieldSpecials = `()<>[]:;@\,."`

// ValidAddress reports whether text is a plausible email address.
func ValidAddress(text string) bool { return addressForm.MatchString(text) }

func splitPieces(text string) []string {
	var pieces []string
	var cur strings.Builder
	quoted := false
	for _, r := range text {
		switch {
		case r == '"':
			quoted = !quoted
			cur.WriteRune(r)
		case !quoted && strings.ContainsRune(",;\n\r", r):
			pieces = append(pieces, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(pieces, cur.String())
}

func unquote(name string) string {
	if len(name) >= 2 && strings.HasPrefix(name, `"`) && strings.HasSuffix(name, `"`) {
		return name[1 : len(name)-1]
	}
	return name
}

func addressOf(piece string) (api.Address, bool) {
	if m := angleForm.FindStringSubmatch(piece); m != nil {
		address := strings.TrimSpace(m[2])
		if !ValidAddress(address) {
			return api.Address{}, false
		}
		return api.Address{Name: unquote(strings.TrimSpace(m[1])), Address: address}, true
	}
	if !ValidAddress(piece) {
		return api.Address{}, false
	}
	return api.Address{Address: piece}, true
}

// ParseAddresses splits typed or pasted text into addresses, dropping
// repeats without regard to case; pieces that do not parse come back as
// invalid, trimmed.
func ParseAddresses(text string) (addresses []api.Address, invalid []string) {
	seen := map[string]bool{}
	for _, raw := range splitPieces(text) {
		piece := strings.TrimSpace(raw)
		if piece == "" {
			continue
		}
		a, ok := addressOf(piece)
		if !ok {
			invalid = append(invalid, piece)
			continue
		}
		if key := strings.ToLower(a.Address); !seen[key] {
			seen[key] = true
			addresses = append(addresses, a)
		}
	}
	return addresses, invalid
}

// plain removes what a token cannot hold, which mail can put there: line
// breaks and other control characters become spaces, so that they split
// nothing, and quotes go, so that they end no quotation.
func plain(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		switch {
		case r == '"':
			return -1
		case unicode.IsControl(r), r == '\u2028', r == '\u2029':
			return ' '
		}
		return r
	}, s))
}

// Token writes an address as a recipient field's token, which reads back
// as this one recipient whatever the mail it came from says: the text is
// untrusted. An address that cannot be written plainly becomes one quoted
// piece, which is invalid and holds the send back.
func Token(a api.Address) string {
	address := plain(a.Address)
	if address != a.Address || strings.ContainsAny(address, ",;<> \t") {
		address = `"` + address + `"`
	}
	name := plain(a.Name)
	switch {
	case name == "":
		return address
	case strings.ContainsAny(name, fieldSpecials):
		return `"` + name + `" <` + address + ">"
	}
	return name + " <" + address + ">"
}

// Choice is a Send Later choice and its time.
type Choice struct {
	Label string
	At    time.Time
}

// SendChoices offers tonight at 9 PM while that is ahead, then tomorrow
// at 8 AM, in now's zone.
func SendChoices(now time.Time) []Choice {
	y, m, d := now.Date()
	tonight := time.Date(y, m, d, 21, 0, 0, 0, now.Location())
	tomorrow := time.Date(y, m, d+1, 8, 0, 0, 0, now.Location())
	var out []Choice
	if now.Before(tonight) {
		out = append(out, Choice{"Send " + tonight.Format("3:04 PM") + " Tonight", tonight})
	}
	return append(out, Choice{"Send " + tomorrow.Format("3:04 PM") + " Tomorrow", tomorrow})
}

// TomorrowMorning is where the Send Later sheet starts.
func TomorrowMorning(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d+1, 8, 0, 0, 0, now.Location())
}
