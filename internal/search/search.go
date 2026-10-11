// Package search parses the search language (docs/specs/search.md) into a
// Query the store turns into SQL over messages and messages_fts. Parse is
// forgiving: anything it does not understand becomes a text term. Match and
// Exclude build the FTS5 expressions for the terms.
package search

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/frostyard/frostmail/api"
)

// Query is a parsed search.
type Query struct {
	Terms         []Term            // full-text conditions, in input order
	Unread        *bool             // is:unread / is:read
	Flagged       *bool             // is:flagged / -is:flagged
	HasAttachment *bool             // has:attachment / -has:attachment
	After         time.Time         // inclusive lower bound (zero: none)
	Before        time.Time         // exclusive upper bound (zero: none)
	Roles         []api.MailboxRole // in:inbox, in:sent, … (any of them)
	// AfterRel and BeforeRel are newer_than: and older_than:'s values
	// ("2d") when After or Before came from one, so a saved search stays
	// relative (ToConditions).
	AfterRel, BeforeRel string
}

// Term is one full-text condition.
type Term struct {
	Column string // "" (any column), "subject", "from_text", "to_text", "attachment_names"
	Text   string // the words, as typed, quotes removed
	Phrase bool   // typed in double quotes
	Not    bool   // prefixed with "-"
}

// token is one lexical unit of the search text.
type token struct {
	neg    bool   // the token started with "-"
	name   string // operator name before ":", "" for a plain word or phrase
	value  string // the operator value or the word, quotes removed
	phrase bool   // the value was typed in double quotes
	body   string // the token without its leading "-", for text terms
}

// Parse reads the search language. It never fails: unknown or malformed
// operators become text terms.
func Parse(text string, now time.Time, loc *time.Location) Query {
	var q Query
	for _, raw := range tokenize(text) {
		t := splitToken(raw)
		if !applyOperator(&q, t, now, loc) {
			addText(&q, t)
		}
	}
	return q
}

// tokenize splits the text on whitespace outside phrases. A quote opens a
// phrase only at the start of a token (after an optional "-") or right
// after "name:"; an unclosed phrase runs to the end.
func tokenize(s string) []string {
	r := []rune(s)
	var toks []string
	for i := 0; i < len(r); {
		if unicode.IsSpace(r[i]) {
			i++
			continue
		}
		start := i
		if r[i] == '-' {
			i++
		}
		if i < len(r) && r[i] == '"' {
			i = closePhrase(r, i+1)
			toks = append(toks, string(r[start:i]))
			continue
		}
		j := i
		for j < len(r) && isNameRune(r[j]) {
			j++
		}
		if j > i && j < len(r) && r[j] == ':' {
			v := j + 1
			if v < len(r) && r[v] == '"' {
				i = closePhrase(r, v+1)
			} else {
				for v < len(r) && !unicode.IsSpace(r[v]) {
					v++
				}
				i = v
			}
			toks = append(toks, string(r[start:i]))
			continue
		}
		for i < len(r) && !unicode.IsSpace(r[i]) {
			i++
		}
		toks = append(toks, string(r[start:i]))
	}
	return toks
}

// closePhrase returns the index just past the closing quote, or the end of
// r when the phrase is unclosed. start is just after the opening quote.
func closePhrase(r []rune, start int) int {
	for start < len(r) && r[start] != '"' {
		start++
	}
	if start < len(r) {
		start++
	}
	return start
}

// splitToken reads the structure of one raw token.
func splitToken(raw string) token {
	r := []rune(raw)
	var t token
	i := 0
	if len(r) > 0 && r[0] == '-' {
		t.neg = true
		i = 1
	}
	t.body = string(r[i:])
	j := i
	for j < len(r) && isNameRune(r[j]) {
		j++
	}
	if j > i && j < len(r) && r[j] == ':' {
		t.name = string(r[i:j])
		t.value, t.phrase = unquote(string(r[j+1:]))
		return t
	}
	if i < len(r) && r[i] == '"' {
		t.value, t.phrase = unquote(string(r[i:]))
	} else {
		t.value = string(r[i:])
	}
	return t
}

// unquote strips a surrounding pair of double quotes, reporting whether it
// was quoted; an unclosed quote still yields the phrase content.
func unquote(s string) (string, bool) {
	if !strings.HasPrefix(s, `"`) {
		return s, false
	}
	inner := s[1:]
	if k := strings.IndexRune(inner, '"'); k >= 0 {
		return inner[:k], true
	}
	return inner, true
}

// applyOperator consumes the token as an operator, reporting whether it
// was one. A negated date or role operator is not an operator: it falls
// through as a negated text term.
func applyOperator(q *Query, t token, now time.Time, loc *time.Location) bool {
	switch strings.ToLower(t.name) {
	case "from":
		return addColumn(q, "from_text", t)
	case "to", "cc":
		return addColumn(q, "to_text", t)
	case "subject":
		return addColumn(q, "subject", t)
	case "filename":
		return addColumn(q, "attachment_names", t)
	case "is":
		return applyIs(q, t)
	case "has":
		if strings.EqualFold(t.value, "attachment") {
			q.HasAttachment = boolPtr(!t.neg)
			return true
		}
	case "after":
		if d, ok := parseDate(t.value, loc); ok && !t.neg {
			q.After, q.AfterRel = d, ""
			return true
		}
	case "before":
		if d, ok := parseDate(t.value, loc); ok && !t.neg {
			q.Before, q.BeforeRel = d, ""
			return true
		}
	case "on":
		if d, ok := parseDate(t.value, loc); ok && !t.neg {
			q.After, q.AfterRel = d, ""
			q.Before, q.BeforeRel = d.AddDate(0, 0, 1), ""
			return true
		}
	case "newer_than":
		if d, ok := ParseRelative(t.value, now, loc); ok && !t.neg {
			q.After, q.AfterRel = d, strings.ToLower(t.value)
			return true
		}
	case "older_than":
		if d, ok := ParseRelative(t.value, now, loc); ok && !t.neg {
			q.Before, q.BeforeRel = d, strings.ToLower(t.value)
			return true
		}
	case "in":
		if role, ok := roleFor(t.value); ok && !t.neg {
			addRole(q, role)
			return true
		}
	}
	return false
}

// applyIs handles is:unread, is:read, is:flagged and is:starred.
func applyIs(q *Query, t token) bool {
	switch strings.ToLower(t.value) {
	case "unread":
		q.Unread = boolPtr(!t.neg)
	case "read":
		q.Unread = boolPtr(t.neg)
	case "flagged", "starred":
		q.Flagged = boolPtr(!t.neg)
	default:
		return false
	}
	return true
}

// addColumn adds a column term unless the value is empty.
func addColumn(q *Query, column string, t token) bool {
	if t.value == "" {
		return false
	}
	q.Terms = append(q.Terms, Term{Column: column, Text: t.value, Phrase: t.phrase, Not: t.neg})
	return true
}

// addText adds a text term unless its text has no letter or digit. A
// failed operator becomes a text term with the whole token body.
func addText(q *Query, t token) {
	text, phrase := t.value, t.phrase
	if t.name != "" {
		text, phrase = t.body, false
	}
	if !hasAlnum(text) {
		return
	}
	q.Terms = append(q.Terms, Term{Text: text, Phrase: phrase, Not: t.neg})
}

// parseDate reads YYYY-MM-DD or YYYY/MM/DD as midnight in loc.
func parseDate(v string, loc *time.Location) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02", "2006/01/02"} {
		if d, err := time.ParseInLocation(layout, v, loc); err == nil {
			return d, true
		}
	}
	return time.Time{}, false
}

// ParseRelative reads Nd, Nw, Nm or Ny (N >= 1) as midnight in loc of
// the day N days, weeks, 30-day months or 365-day years before now's
// day in loc: newer_than: and older_than:, and the within and notwithin
// conditions (docs/design/organize.md).
func ParseRelative(v string, now time.Time, loc *time.Location) (time.Time, bool) {
	r := []rune(v)
	if len(r) < 2 {
		return time.Time{}, false
	}
	n, err := strconv.Atoi(string(r[:len(r)-1]))
	if err != nil || n < 1 {
		return time.Time{}, false
	}
	var mult int
	switch unicode.ToLower(r[len(r)-1]) {
	case 'd':
		mult = 1
	case 'w':
		mult = 7
	case 'm':
		mult = 30
	case 'y':
		mult = 365
	default:
		return time.Time{}, false
	}
	b := now.In(loc)
	return time.Date(b.Year(), b.Month(), b.Day()-n*mult, 0, 0, 0, 0, loc), true
}

// roleFor maps an in: value to a mailbox role.
func roleFor(v string) (api.MailboxRole, bool) {
	switch strings.ToLower(v) {
	case "inbox":
		return api.MailboxRoleInbox, true
	case "drafts":
		return api.MailboxRoleDrafts, true
	case "sent":
		return api.MailboxRoleSent, true
	case "junk", "spam":
		return api.MailboxRoleJunk, true
	case "trash":
		return api.MailboxRoleTrash, true
	case "archive":
		return api.MailboxRoleArchive, true
	}
	return "", false
}

// addRole appends the role unless it is already there.
func addRole(q *Query, role api.MailboxRole) {
	for _, r := range q.Roles {
		if r == role {
			return
		}
	}
	q.Roles = append(q.Roles, role)
}

// Match is the FTS5 expression for the positive terms ("" when none).
func (q Query) Match() string {
	var parts []string
	for _, t := range q.Terms {
		if !t.Not {
			parts = append(parts, fts(t))
		}
	}
	return strings.Join(parts, " ")
}

// Exclude is the FTS5 expression matching any negated term ("" when none).
func (q Query) Exclude() string {
	var parts []string
	for _, t := range q.Terms {
		if t.Not {
			parts = append(parts, fts(t))
		}
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return "(" + strings.Join(parts, " OR ") + ")"
	}
}

// fts formats one term as a quoted FTS5 phrase, prefix-matched when it
// is not a phrase.
func fts(t Term) string {
	s := ""
	if t.Column != "" {
		s = t.Column + " : "
	}
	s += `"` + strings.ReplaceAll(t.Text, `"`, `""`) + `"`
	if !t.Phrase {
		s += "*"
	}
	return s
}

// Words splits a condition's value into terms on column, as the search
// field would: a value wholly in double quotes is one phrase, otherwise
// each whitespace-separated word with a letter or digit is a prefix term.
func Words(value, column string) []Term {
	v := strings.TrimSpace(value)
	if r := []rune(v); len(r) >= 2 && r[0] == '"' && r[len(r)-1] == '"' {
		if text := string(r[1 : len(r)-1]); hasAlnum(text) {
			return []Term{{Column: column, Text: text, Phrase: true}}
		}
		return nil
	}
	var terms []Term
	for _, w := range strings.Fields(v) {
		if hasAlnum(w) {
			terms = append(terms, Term{Column: column, Text: w})
		}
	}
	return terms
}

// Empty reports whether the query has no condition at all.
func (q Query) Empty() bool {
	return len(q.Terms) == 0 && q.Unread == nil && q.Flagged == nil &&
		q.HasAttachment == nil && q.After.IsZero() && q.Before.IsZero() && len(q.Roles) == 0
}

// boolPtr returns a pointer to b.
func boolPtr(b bool) *bool { return &b }

// isNameRune reports whether r may appear in an operator name.
func isNameRune(r rune) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

// hasAlnum reports whether s contains a letter or a digit.
func hasAlnum(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
