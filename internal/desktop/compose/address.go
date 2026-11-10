package compose

import (
	"regexp"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
)

var (
	angleForm   = regexp.MustCompile(`^(.*)<([^<>]*)>$`)
	addressForm = regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)
)

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

// Token writes an address as a recipient field's token.
func Token(a api.Address) string {
	if a.Name == "" {
		return a.Address
	}
	if strings.ContainsAny(a.Name, `,;"`) {
		return `"` + strings.ReplaceAll(a.Name, `"`, "") + `" <` + a.Address + ">"
	}
	return a.Name + " <" + a.Address + ">"
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
