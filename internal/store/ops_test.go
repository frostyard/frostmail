package store

import (
	"slices"
	"testing"
)

// A queued action keeps its messages' local flags until it is replayed or
// fails: a reconcile pass that fetched the server's flags before the replay
// must not undo the user's change (docs/design/sync.md).
func TestUpdateFlagsKeepsMessagesWithQueuedOps(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, _ := mailboxFixture(t, d)
	ids := insertHeaders(t, d, acct, inbox, header(1, "a"), header(2, "b"), header(3, "c"))
	seen := true
	var flagsOp, deleteOp int64
	err := d.Tx(ctx, func(tx *Tx) error {
		if _, err := tx.ChangeFlags(ctx, ids[:1], FlagChange{Seen: &seen}); err != nil {
			return err
		}
		var err error
		if flagsOp, err = tx.QueueOp(ctx, acct, "flags", map[string]any{}, ids[:1]); err != nil {
			return err
		}
		if err := tx.MarkDeleted(ctx, ids[1:2]); err != nil {
			return err
		}
		deleteOp, err = tx.QueueOp(ctx, acct, "expunge", map[string]any{}, ids[1:2])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// The server still reports the old flags for all three.
	server := []FlagUpdate{
		{UID: 1, ModSeq: 10, Flags: Flags{}},
		{UID: 2, ModSeq: 11, Flags: Flags{}},
		{UID: 3, ModSeq: 12, Flags: Flags{Flagged: true}},
	}
	update := func() []int64 {
		t.Helper()
		var changed []int64
		if err := d.Tx(ctx, func(tx *Tx) error {
			var err error
			changed, err = tx.UpdateFlags(ctx, inbox, server)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return changed
	}
	if got := update(); !slices.Equal(got, ids[2:3]) {
		t.Fatalf("changed = %v, want only the message without a queued op (%d)", got, ids[2])
	}
	var s, del int64
	if err := d.db.QueryRowContext(ctx, `SELECT seen FROM messages WHERE id = ?`, ids[0]).Scan(&s); err != nil || s != 1 {
		t.Fatalf("seen after reconcile = %d, %v; want the local 1", s, err)
	}
	if err := d.db.QueryRowContext(ctx, `SELECT deleted FROM messages WHERE id = ?`, ids[1]).Scan(&del); err != nil || del != 1 {
		t.Fatalf("deleted after reconcile = %d, %v; want the local 1", del, err)
	}

	// Once the actions are done (replayed) or failed, the server wins again.
	if err := d.Tx(ctx, func(tx *Tx) error {
		if err := tx.DeleteOp(ctx, flagsOp); err != nil {
			return err
		}
		return tx.FailOp(ctx, deleteOp, "refused")
	}); err != nil {
		t.Fatal(err)
	}
	if got := update(); !slices.Equal(got, ids[:2]) {
		t.Fatalf("changed after the ops ended = %v, want %v", got, ids[:2])
	}
	var n int
	if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM pending_op_messages`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("pending_op_messages rows = %d, %v; want 1 (the failed op's)", n, err)
	}
}
