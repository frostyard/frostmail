package store

import "context"

// SearchDoc is the text indexed for one message.
type SearchDoc struct {
	Subject         string
	From            string // names and addresses, space-separated
	To              string // To and Cc names and addresses
	Body            string // plain text; the preview until the body is fetched
	AttachmentNames string // space-separated file names
}

// IndexMessage writes the search entry of message id, replacing any previous
// one. Task T-0011 implements it.
func (t *Tx) IndexMessage(ctx context.Context, id int64, doc SearchDoc) error {
	return errNotImplemented
}

// RemoveFromIndex deletes the search entries of ids; ids without an entry
// are ignored. Task T-0011 implements it.
func (t *Tx) RemoveFromIndex(ctx context.Context, ids []int64) error {
	return errNotImplemented
}

// SearchQuery turns what a user typed into an FTS5 MATCH expression, or ""
// when there is nothing to search for. Task T-0011 implements it.
func SearchQuery(input string) string {
	return ""
}
