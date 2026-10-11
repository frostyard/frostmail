package store

import (
	"encoding/json/v2"
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func muted(t *testing.T, d *DB, id int64) bool {
	t.Helper()
	rows, err := d.Summaries(t.Context(), []int64{id})
	if err != nil || len(rows) != 1 {
		t.Fatalf("summary %d: %v, %d rows", id, err, len(rows))
	}
	return rows[0].Muted
}

// TestMuteThreads: muting marks the whole conversation, says so for every
// message of it, and survives a merge with another conversation.
func TestMuteThreads(t *testing.T) {
	d := threadFixture(t,
		threadMsg{id: 1, msgid: "x", subject: "X", day: 1},
		threadMsg{id: 2, msgid: "y", subject: "Y", day: 1},
		threadMsg{id: 4, msgid: "y2", irt: "y", refs: []string{"y"}, subject: "Re: Y", day: 2},
	)
	assign(t, d, 1, 2, 4)
	var events []api.EventEnvelope
	d.OnCommit = func(evs []api.EventEnvelope) { events = append(events, evs...) }
	ctx := t.Context()
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.MuteThreads(ctx, []int64{2}, true) }); err != nil {
		t.Fatal(err)
	}
	if muted(t, d, 1) || !muted(t, d, 2) || !muted(t, d, 4) {
		t.Fatalf("muted = %v %v %v, want false true true", muted(t, d, 1), muted(t, d, 2), muted(t, d, 4))
	}
	if len(events) != 1 || events[0].Event != "message.changed" {
		t.Fatalf("events = %+v, want one message.changed", events)
	}
	var ev api.MessageChanged
	if err := json.Unmarshal(events[0].Data, &ev); err != nil || !slices.Equal(ev.IDs, []int64{2, 4}) {
		t.Errorf("message.changed = %s, %v; want ids [2 4]", events[0].Data, err)
	}

	// x is older, so y merges into it: the merged conversation is muted.
	addThreadMsgs(t, d, threadMsg{id: 3, msgid: "z", refs: []string{"x", "y"}, subject: "Re: both", day: 3})
	assign(t, d, 3)
	for _, id := range []int64{1, 2, 3, 4} {
		if !muted(t, d, id) {
			t.Errorf("message %d is not muted after the merge", id)
		}
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.MuteThreads(ctx, []int64{3}, false) }); err != nil {
		t.Fatal(err)
	}
	if muted(t, d, 1) {
		t.Error("unmuting left the conversation muted")
	}
}

// TestBlockedSenders: blocking is by lowercased address, matches senders
// whatever their case, and emits block.changed only when something changed.
func TestBlockedSenders(t *testing.T) {
	d := threadFixture(t)
	ctx := t.Context()
	if _, err := d.db.ExecContext(ctx, `INSERT INTO messages (id, account_id, from_addr, internal_date) VALUES
		(1, 1, 'Spam@Y.test', 'now'), (2, 1, 'ann@x.test', 'now')`); err != nil {
		t.Fatal(err)
	}
	var events []api.EventEnvelope
	d.OnCommit = func(evs []api.EventEnvelope) { events = append(events, evs...) }
	block := func(f func(tx *Tx) error) {
		t.Helper()
		if err := d.Tx(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	block(func(tx *Tx) error { return tx.Block(ctx, []string{" SPAM@y.test "}) })
	block(func(tx *Tx) error { return tx.Block(ctx, []string{"spam@y.test"}) })
	list, err := d.BlockedSenders(ctx)
	if err != nil || len(list) != 1 || list[0].Address != "spam@y.test" || !list[0].At.Equal(testNow) {
		t.Fatalf("blocked = %+v, %v", list, err)
	}
	got, err := d.BlockedAmong(ctx, []int64{1, 2, 3})
	if err != nil || !slices.Equal(got, []int64{1}) {
		t.Fatalf("blocked among = %v, %v; want [1]", got, err)
	}
	block(func(tx *Tx) error { return tx.Unblock(ctx, []string{"Spam@Y.test"}) })
	block(func(tx *Tx) error { return tx.Unblock(ctx, []string{"spam@y.test"}) })
	if got, _ := d.BlockedAmong(ctx, []int64{1, 2}); len(got) != 0 {
		t.Errorf("after unblocking: %v", got)
	}
	if len(events) != 2 || events[0].Event != "block.changed" || events[1].Event != "block.changed" {
		t.Errorf("events = %+v, want two block.changed", events)
	}
}

// TestBlockActionDefaultsToMark: a new database marks blocked mail.
func TestBlockActionDefaultsToMark(t *testing.T) {
	d, _ := openTest(t)
	s, err := d.Settings(t.Context())
	if err != nil || s.BlockAction != string(api.BlockActionMark) {
		t.Fatalf("blockAction = %q, %v; want mark", s.BlockAction, err)
	}
}
