package store

import (
	"slices"
	"strings"
)

// Flags are a message's flags as stored (docs/design/storage.md).
type Flags struct {
	Seen      bool
	Flagged   bool
	Answered  bool
	Forwarded bool // the $Forwarded keyword
	Draft     bool
	Deleted   bool // \Deleted: marked for expunge; hidden from views
	// Color is 0 when unflagged, else 1-7: Mail.app's flag color
	// ($MailFlagBit0-2 read as a 3-bit number) plus one, so red, which has
	// no bits set, is 1.
	Color int
	// Keywords are the other keywords, sorted, without duplicates.
	Keywords []string
}

// FlagsFromIMAP maps IMAP flags and keywords to Flags, comparing names
// without case. \Seen, \Flagged, \Answered, \Draft and \Deleted set their
// fields; every other name starting with \ (such as \Recent and \*) is
// ignored. $Forwarded sets Forwarded. $MailFlagBit0, $MailFlagBit1 and
// $MailFlagBit2 are read as a 3-bit number, and Color is that number plus
// one when Flagged, else 0: red is Color 1 with no bits, and a color
// without Flagged means nothing. Every other non-empty name is a keyword,
// keeping the first spelling of each and sorted by lowercase.
func FlagsFromIMAP(flags []string) Flags {
	var f Flags
	var bits int
	var keywords []string
	seen := make(map[string]bool)
	for _, name := range flags {
		switch lower := strings.ToLower(name); lower {
		case "":
			continue
		case `\seen`:
			f.Seen = true
		case `\flagged`:
			f.Flagged = true
		case `\answered`:
			f.Answered = true
		case `\draft`:
			f.Draft = true
		case `\deleted`:
			f.Deleted = true
		case `$forwarded`:
			f.Forwarded = true
		case `$mailflagbit0`:
			bits |= 1
		case `$mailflagbit1`:
			bits |= 2
		case `$mailflagbit2`:
			bits |= 4
		default:
			if strings.HasPrefix(name, `\`) {
				continue
			}
			if !seen[lower] {
				seen[lower] = true
				keywords = append(keywords, name)
			}
		}
	}
	if f.Flagged {
		f.Color = bits + 1
	}
	slices.SortFunc(keywords, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	f.Keywords = keywords
	return f
}

// IMAPFlags maps Flags back to the IMAP flags and keywords that represent
// them, in this order: \Seen, \Answered, \Flagged, \Deleted and \Draft if
// set, then $Forwarded if set, then, when Flagged and Color > 1,
// $MailFlagBit0, $MailFlagBit1 and $MailFlagBit2 for the set bits of
// Color-1, then the keywords as they are. A Color without Flagged is
// dropped. The result is nil when nothing is set.
func (f Flags) IMAPFlags() []string {
	var out []string
	if f.Seen {
		out = append(out, `\Seen`)
	}
	if f.Answered {
		out = append(out, `\Answered`)
	}
	if f.Flagged {
		out = append(out, `\Flagged`)
	}
	if f.Deleted {
		out = append(out, `\Deleted`)
	}
	if f.Draft {
		out = append(out, `\Draft`)
	}
	if f.Forwarded {
		out = append(out, `$Forwarded`)
	}
	if f.Flagged && f.Color > 1 {
		bits := f.Color - 1
		if bits&1 != 0 {
			out = append(out, `$MailFlagBit0`)
		}
		if bits&2 != 0 {
			out = append(out, `$MailFlagBit1`)
		}
		if bits&4 != 0 {
			out = append(out, `$MailFlagBit2`)
		}
	}
	out = append(out, f.Keywords...)
	return out
}
