// Package search parses the search language (docs/specs/search.md) into a
// Query the store turns into SQL. Task T-0048 implements Parse, Match,
// Exclude and Empty; the stubs find nothing.
package search

import (
	"time"

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
}

// Term is one full-text condition.
type Term struct {
	Column string // "" (any column), "subject", "from_text", "to_text", "attachment_names"
	Text   string // the words, as typed, quotes removed
	Phrase bool   // typed in double quotes
	Not    bool   // prefixed with "-"
}

// Parse reads the search language. It never fails: unknown or malformed
// operators become text terms.
func Parse(_ string, _ time.Time, _ *time.Location) Query { return Query{} }

// Match is the FTS5 expression for the positive terms ("" when none).
func (q Query) Match() string { return "" }

// Exclude is the FTS5 expression matching any negated term ("" when none).
func (q Query) Exclude() string { return "" }

// Empty reports whether the query has no condition at all.
func (q Query) Empty() bool { return true }
