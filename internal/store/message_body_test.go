package store

import (
	"errors"
	"testing"
)

// TestMessageBody: the narrow one-row read of a message's body blobs and
// subject agrees with GetMessage, and a missing message is ErrNotFound.
func TestMessageBody(t *testing.T) {
	d := readFixture(t)
	ctx := t.Context()
	if _, err := d.db.ExecContext(ctx,
		`UPDATE messages SET blob_sha = 'a1', display_sha = 'd1' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	got, err := d.MessageBody(ctx, 1)
	detail, err2 := d.GetMessage(ctx, 1)
	if err != nil || err2 != nil {
		t.Fatalf("MessageBody(1) = %+v, %v; GetMessage(1) = %v", got, err, err2)
	}
	if want := (MessageBody{Blob: detail.BlobID, Display: detail.DisplayBlobID, Subject: detail.Subject}); got != want ||
		got.Blob != "a1" || got.Display != "d1" || got.Subject != "Re: Plan" {
		t.Fatalf("MessageBody(1) = %+v, want %+v", got, want)
	}
	// Message 2 has neither blob.
	if got, err := d.MessageBody(ctx, 2); err != nil || got != (MessageBody{Subject: "No date"}) {
		t.Fatalf("MessageBody(2) = %+v, %v; want no blobs", got, err)
	}
	if _, err := d.MessageBody(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("MessageBody(99) = %v, want ErrNotFound", err)
	}
}
