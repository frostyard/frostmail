package store

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// SearchDoc is the text indexed for one message.
type SearchDoc struct {
	Subject         string
	From            string // names and addresses, space-separated
	To              string // To and Cc names and addresses
	Body            string // plain text; the preview until the body is fetched
	AttachmentNames string // space-separated file names
}

// IndexMessage writes the search entry of message id, replacing any previous
// one. The messages_fts table is contentless, so a re-index is a delete of
// the old row followed by an insert.
func (t *Tx) IndexMessage(ctx context.Context, id int64, doc SearchDoc) error {
	if _, err := t.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, id); err != nil {
		return fmt.Errorf("index message %d: %w", id, err)
	}
	_, err := t.ExecContext(ctx, `INSERT INTO messages_fts (rowid, subject, from_text, to_text, body_text, attachment_names)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, doc.Subject, doc.From, doc.To, doc.Body, doc.AttachmentNames)
	if err != nil {
		return fmt.Errorf("index message %d: %w", id, err)
	}
	return nil
}

// RemoveFromIndex deletes the search entries of ids; ids without an entry
// are ignored.
func (t *Tx) RemoveFromIndex(ctx context.Context, ids []int64) error {
	for _, id := range ids {
		if _, err := t.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, id); err != nil {
			return fmt.Errorf("remove message %d from index: %w", id, err)
		}
	}
	return nil
}

// SearchQuery turns what a user typed into an FTS5 MATCH expression, or ""
// when there is nothing to search for. Input is split on whitespace; terms
// with no letter or digit are dropped. Each remaining term becomes a quoted
// prefix phrase: every double quote in it is doubled, the term is wrapped in
// double quotes and suffixed with *. Quoting keeps FTS5 operators (OR, NOT,
// -, ") in the input from being read as syntax. Juxtaposed phrases are ANDed.
func SearchQuery(input string) string {
	var phrases []string
	for _, term := range strings.Fields(input) {
		if !strings.ContainsFunc(term, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			continue
		}
		phrases = append(phrases, `"`+strings.ReplaceAll(term, `"`, `""`)+`"*`)
	}
	return strings.Join(phrases, " ")
}
