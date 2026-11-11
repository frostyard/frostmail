package live

import (
	"context"
	"encoding/json/v2"
	"errors"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

func summaries(ids ...int64) []api.MessageSummary {
	out := make([]api.MessageSummary, 0, len(ids))
	for _, id := range ids {
		out = append(out, api.MessageSummary{ID: id})
	}
	return out
}

// A view.range reply newer than the deltas applied: message 2 has moved to
// the top on the server, and the delta saying so has not arrived. Two
// indices holding one message give a list two rows with one key, which
// MyGo refuses with a panic.
func TestAMessageIsHeldAtOneIndexOnly(t *testing.T) {
	v := &View{rows: map[int64]api.MessageSummary{0: {ID: 1}, 1: {ID: 3}, 150: {ID: 2}}}
	v.storeLocked(0, summaries(2, 1))
	seen := map[int64]int64{}
	for i, r := range v.rows {
		if at, dup := seen[r.ID]; dup {
			t.Fatalf("message %d is at %d and %d", r.ID, at, i)
		}
		seen[r.ID] = i
	}
	if got := v.rows[0].ID; got != 2 {
		t.Errorf("row 0 is %d", got)
	}
	if _, ok := v.rows[150]; ok {
		t.Error("the older copy stayed")
	}
	if got := v.rows[1].ID; got != 1 {
		t.Errorf("row 1 is %d", got)
	}
}

// A frame reads one state, whatever changes while it builds.
func TestASnapshotDoesNotChange(t *testing.T) {
	v := &View{rows: map[int64]api.MessageSummary{0: {ID: 1}, 1: {ID: 2}}, total: 2, departed: map[int64]bool{}}
	snap := v.Snapshot()
	v.applyLocked(api.ViewDelta{Count: 3, Ops: []api.ViewOp{{Op: api.ViewOpKindInsert, At: 0, Count: 1}}})
	v.storeLocked(0, summaries(9))
	v.window = [2]int64{0, 0}
	v.evictLocked()
	if r, ok := snap.Row(0); !ok || r.ID != 1 {
		t.Errorf("row 0 is %+v, %v", r, ok)
	}
	if r, ok := snap.Row(1); !ok || r.ID != 2 {
		t.Errorf("row 1 is %+v, %v", r, ok)
	}
	if snap.Count() != 2 {
		t.Errorf("count %d", snap.Count())
	}
}

// M27: a view.open that answers after the view gave up on it. maild opened
// the view all the same, and only the answer says which: dropped, it would
// keep that view, and its deltas, until the connection ends.
func TestALateViewOpenIsClosedInMaild(t *testing.T) {
	s := newHoldServer(map[string]any{"view.open": api.ViewInfo{ID: 42, Count: 3}})
	s.held = "view.open"
	changed := make(chan struct{}, 1)
	v := openView(s.client(t), NewEvents(), api.ViewQuery{}, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}, time.Millisecond)
	defer v.Close()
	s.waitFor(t, "view.open")
	select {
	case <-changed:
	case <-time.After(10 * time.Second):
		t.Fatal("the view did not give up")
	}
	if err := v.Err(); !errors.Is(err, context.DeadlineExceeded) || v.Ready() {
		t.Fatalf("after the deadline: ready %v, err %v", v.Ready(), err)
	}
	close(s.release)
	req := s.waitFor(t, "view.close")
	var p api.ViewCloseParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		t.Fatal(err)
	}
	if p.ID != 42 {
		t.Fatalf("closed view %d, want 42", p.ID)
	}
	if v.Ready() || !errors.Is(v.Err(), context.DeadlineExceeded) {
		t.Errorf("the late answer opened the view: ready %v, err %v", v.Ready(), v.Err())
	}
}
