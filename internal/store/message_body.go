package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MessageBody is the little a reader needs of a message besides its body:
// the blobs of its stored bodies and its subject.
type MessageBody struct {
	Blob    string // the raw message in the blob store; "" until fetched
	Display string // its display copy (ADR-0037); "" until fetched
	Subject string
}

// MessageBody returns one message's body blobs and subject in one row read,
// where GetMessage takes three queries. A missing row is ErrNotFound.
func (d *DB) MessageBody(ctx context.Context, id int64) (MessageBody, error) {
	var b MessageBody
	var blobSHA, displaySHA sql.NullString
	err := d.db.QueryRowContext(ctx,
		`SELECT blob_sha, display_sha, subject FROM messages WHERE id = ?`, id).
		Scan(&blobSHA, &displaySHA, &b.Subject)
	if errors.Is(err, sql.ErrNoRows) {
		return MessageBody{}, ErrNotFound
	}
	if err != nil {
		return MessageBody{}, fmt.Errorf("message body %d: %w", id, err)
	}
	b.Blob, b.Display = blobSHA.String, displaySHA.String
	return b, nil
}
