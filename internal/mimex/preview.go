package mimex

import (
	"regexp"
	"strings"
	"unicode"
)

// attributionRE matches the localized verbs of a reply attribution line,
// "On Tue ... wrote:", in the languages frostmail sees most.
var attributionRE = regexp.MustCompile(`wrote|schrieb|écrit|escribió`)

// Preview returns a one-line preview of a message body: the first words
// the sender actually wrote, with quoted replies, attribution lines such
// as "On Tue ... wrote:" and signatures dropped. Whitespace collapses to
// single spaces. The result is at most max runes; when it must be cut,
// the cut lands on the last space past the halfway point if there is
// one, and the preview ends with an ellipsis (U+2026). max <= 0 returns
// an empty string.
func Preview(text string, max int) string {
	if max <= 0 {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for i, line := range lines {
		if isStopLine(line) {
			break
		}
		if isQuotedLine(line) {
			continue
		}
		if isAttributionLine(line) && nextNonBlankIsQuote(lines, i) {
			continue
		}
		kept = append(kept, line)
	}
	return truncate(strings.Join(strings.Fields(strings.Join(kept, " ")), " "), max)
}

func isStopLine(line string) bool {
	if strings.TrimRightFunc(line, unicode.IsSpace) == "--" {
		return true
	}
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "-----Original Message-----") {
		return true
	}
	return len(trimmed) >= 10 && strings.Trim(trimmed, "_") == ""
}

func isQuotedLine(line string) bool {
	return strings.HasPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), ">")
}

func isAttributionLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasSuffix(trimmed, ":") && attributionRE.MatchString(strings.ToLower(trimmed))
}

func nextNonBlankIsQuote(lines []string, i int) bool {
	for j := i + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "" {
			continue
		}
		return isQuotedLine(lines[j])
	}
	return false
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	cut := max - 1
	for j := cut - 1; j > cut/2; j-- {
		if runes[j] == ' ' {
			return strings.TrimRight(string(runes[:j]), " ") + "…"
		}
	}
	return string(runes[:cut]) + "…"
}
