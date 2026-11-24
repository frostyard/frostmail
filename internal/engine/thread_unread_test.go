package engine_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

func TestViewThreadUnread(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ids := conversation(t, srv, c)
	ctx := t.Context()
	if err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE messages SET seen = 1 WHERE id IN (?, ?)`, ids[1], ids[2])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	inbox := api.MailboxRoleInbox
	for _, threads := range []bool{true, false} {
		v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Role: &inbox, Threads: &threads}})
		if err != nil {
			t.Fatal(err)
		}
		rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 10})
		if err != nil {
			t.Fatal(err)
		}
		wantLen := 3
		if threads {
			wantLen = 2
		}
		if len(rows) != wantLen {
			t.Fatalf("threads %v: %d rows, want %d", threads, len(rows), wantLen)
		}
		for _, row := range rows {
			if !threads {
				if row.ThreadUnread != nil {
					t.Errorf("unthreaded row %d: threadUnread = %v", row.ID, *row.ThreadUnread)
				}
				continue
			}
			want := int64(0)
			if row.ID == ids[1] {
				want = 1
			} else if row.ID != ids[2] {
				t.Fatalf("unexpected thread row %d", row.ID)
			}
			if !row.Flags.Seen {
				t.Errorf("thread row %d should be seen", row.ID)
			}
			if row.ThreadUnread == nil || *row.ThreadUnread != want {
				t.Errorf("thread row %d: threadUnread = %v, want %d", row.ID, row.ThreadUnread, want)
			}
		}
		if err := c.View().Close(ctx, &api.ViewCloseParams{ID: v.ID}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(ids) {
		t.Fatalf("summaries: %d rows, want %d", len(rows), len(ids))
	}
	for _, row := range rows {
		if row.ThreadUnread != nil {
			t.Errorf("message.summaries row %d: threadUnread present", row.ID)
		}
	}
}
