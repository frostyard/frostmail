// Package mimex parses and normalizes mail messages for frostmail; the x
// marks it as frostmail's layer over the standard mime packages.
package mimex

import (
	"regexp"
	"strings"
)

var (
	subjectSpaceRE  = regexp.MustCompile(`\s+`)
	subjectPrefixRE = regexp.MustCompile(`(?i)^(?:re|fwd|fw|aw|wg|sv|vs|antw|rif|tr|回复|答复|转发)(?:\[[0-9]+\]|\([0-9]+\))? *[:：] *`)
	subjectTagRE    = regexp.MustCompile(`^\[[^\]]*\] *`)
)

// NormalizeSubject returns the subject with reply/forward prefixes and
// mailing-list tags stripped from the front, so that replies and forwards of
// the same message share a threading key. Whitespace runs collapse to a
// single space and both ends are trimmed; case is preserved and brackets
// away from the start are left alone. The result is stable: normalizing an
// already-normalized subject changes nothing.
func NormalizeSubject(s string) string {
	s = strings.TrimSpace(subjectSpaceRE.ReplaceAllString(s, " "))
	for {
		if loc := subjectPrefixRE.FindStringIndex(s); loc != nil {
			s = s[loc[1]:]
			continue
		}
		if loc := subjectTagRE.FindStringIndex(s); loc != nil {
			s = s[loc[1]:]
			continue
		}
		return s
	}
}
