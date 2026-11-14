package store

import (
	"errors"
	"fmt"
	"testing"
)

// MessagesAtUIDs resolves a batch larger than 1 query's chunk, and leaves
// out UIDs the mailbox does not hold.
func TestMessagesAtUIDs(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, archive := mailboxFixture(t, d)
	n := uidChunk + 7
	hs := make([]MessageHeader, n)
	uids := make([]uint32, n)
	for i := range hs {
		uids[i] = uint32(i + 1)
		hs[i] = header(uids[i], fmt.Sprintf("s%d", i))
	}
	ids := insertHeaders(t, d, acct, inbox, hs...)
	other := insertHeaders(t, d, acct, archive, header(1, "elsewhere"))
	err := d.Tx(ctx, func(tx *Tx) error {
		at, err := tx.MessagesAtUIDs(ctx, inbox, append(uids, 9999))
		if err != nil {
			return err
		}
		if len(at) != n {
			t.Fatalf("%d messages for %d stored UIDs and 1 missing", len(at), n)
		}
		for i, uid := range uids {
			if at[uid] != ids[i] {
				t.Fatalf("UID %d = message %d, want %d", uid, at[uid], ids[i])
			}
		}
		if at[1] == other[0] {
			t.Fatal("UID 1 resolved to the other mailbox's message")
		}
		none, err := tx.MessagesAtUIDs(ctx, inbox, nil)
		if err != nil || len(none) != 0 {
			t.Fatalf("no UIDs = %v, %v", none, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// MailboxByID reads 1 mailbox of its account and refuses another account's.
func TestMailboxByID(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, _ := mailboxFixture(t, d)
	insertHeaders(t, d, acct, inbox, header(1, "a"))
	mb, err := d.MailboxByID(ctx, acct, inbox)
	if err != nil {
		t.Fatal(err)
	}
	all, err := d.ListMailboxes(ctx, acct)
	if err != nil {
		t.Fatal(err)
	}
	var want Mailbox
	for _, m := range all {
		if m.ID == inbox {
			want = m
		}
	}
	want.Total, want.Unread = 0, 0
	if mb != want {
		t.Fatalf("mailbox = %+v, want %+v", mb, want)
	}
	if _, err := d.MailboxByID(ctx, acct+1, inbox); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another account's mailbox = %v, want ErrNotFound", err)
	}
	if _, err := d.MailboxByID(ctx, acct, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing mailbox = %v, want ErrNotFound", err)
	}
}
