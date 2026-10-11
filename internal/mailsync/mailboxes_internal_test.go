package mailsync

import (
	"errors"
	"testing"
)

// TestMailboxChangesWaitForQueuedOps: while an account's ops wait for the
// server, its mailboxes are not renamed, moved or deleted, so a rename
// cannot overtake an op naming the old path; a Gmail mailbox made here is a
// label.
func TestMailboxChangesWaitForQueuedOps(t *testing.T) {
	e := newGmailEnv(t) // no actor runs: ops stay queued
	ctx := t.Context()
	work, err := e.a.m.CreateMailbox(ctx, e.a.acct.ID, "Clients", nil)
	if err != nil || !work.Label || work.Path != "Clients" {
		t.Fatalf("create = %+v, %v", work, err)
	}
	if _, err := e.a.m.RenameMailbox(ctx, work.ID, "Customers"); !errors.Is(err, ErrConflict) {
		t.Errorf("rename with a queued op: %v, want ErrConflict", err)
	}
	if err := e.a.m.DeleteMailbox(ctx, work.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("delete with a queued op: %v, want ErrConflict", err)
	}
	if _, err := e.a.m.SetMailboxRole(ctx, work.ID, "trash"); !errors.Is(err, ErrConflict) {
		t.Errorf("a Gmail role: %v, want ErrConflict", err)
	}
	if _, err := e.a.m.MoveMailbox(ctx, e.mbs["Work"], &work.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("move with a queued op: %v, want ErrConflict", err)
	}
}
