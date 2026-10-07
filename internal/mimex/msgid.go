package mimex

import (
	"strings"
	"unicode"
)

// ParseMessageIDs extracts the message IDs from a References or
// In-Reply-To header value. It scans the string once, left to right:
// parenthesized comments nest and are skipped, an unterminated comment runs
// to the end; angle brackets delimit an ID whose inner whitespace is
// removed, empty IDs are dropped, and parentheses inside brackets are part
// of the ID. If the string held at least one bracketed ID, only those are
// returned; otherwise the comment-free text is split on whitespace and
// commas and the tokens containing '@' are kept. Duplicates are removed
// keeping the first occurrence and the order. It returns nil when there
// are no IDs.
func ParseMessageIDs(s string) []string {
	var (
		ids     []string
		plain   strings.Builder
		bracket bool
		comment int
		inID    bool
		idBuf   strings.Builder
	)
	for _, r := range s {
		switch {
		case inID:
			switch r {
			case '>':
				if id := idBuf.String(); id != "" {
					ids = append(ids, id)
				}
				idBuf.Reset()
				inID = false
			default:
				if !unicode.IsSpace(r) {
					idBuf.WriteRune(r)
				}
			}
		case comment > 0:
			switch r {
			case '(':
				comment++
			case ')':
				comment--
			}
		default:
			switch r {
			case '(':
				comment++
			case '<':
				bracket = true
				inID = true
			default:
				plain.WriteRune(r)
			}
		}
	}
	if inID {
		if id := idBuf.String(); id != "" {
			ids = append(ids, id)
		}
	}
	if !bracket {
		for _, tok := range strings.FieldsFunc(plain.String(), func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
			if strings.Contains(tok, "@") {
				ids = append(ids, tok)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
