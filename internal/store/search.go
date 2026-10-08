package store

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
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

// SearchDocFor is the search entry for a newly stored message: its subject,
// sender, recipients, preview and attachment names.
func SearchDocFor(h MessageHeader) SearchDoc {
	var from, to, names bytes.Buffer
	from.WriteString(h.From.Name + " " + h.From.Addr)
	for _, a := range append(slices.Clone(h.To), h.Cc...) {
		to.WriteString(a.Name + " " + a.Addr + " ")
	}
	for _, p := range h.Parts {
		if p.Filename != "" {
			names.WriteString(p.Filename + " ")
		}
	}
	return SearchDoc{
		Subject: h.Subject, From: from.String(), To: strings.TrimSpace(to.String()),
		Body: h.Preview, AttachmentNames: strings.TrimSpace(names.String()),
	}
}
