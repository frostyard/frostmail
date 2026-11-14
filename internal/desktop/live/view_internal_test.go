package live

import (
	"context"
	"encoding/json/v2"
	"errors"
	"slices"
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
	v := &View{rows: map[int64]api.MessageSummary{0: {ID: 1}, 1: {ID: 3}, 150: {ID: 2}}, held: map[int64]uint64{}}
	v.storeLocked(0, summaries(2, 1), 0)
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
	v := &View{rows: map[int64]api.MessageSummary{0: {ID: 1}, 1: {ID: 2}}, total: 2, departed: map[int64]bool{}, held: map[int64]uint64{}}
	snap := v.Snapshot()
	v.applyLocked(api.ViewDelta{Count: 3, Ops: []api.ViewOp{{Op: api.ViewOpKindInsert, At: 0, Count: 1}}})
	v.storeLocked(0, summaries(9), 0)
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

// #317: two message.changed events for one message, whose
// message.summaries answers come in the wrong order. The older answer must
// not undo the newer one: nothing would correct it.
func TestAnOlderRefreshDoesNotUndoANewerOne(t *testing.T) {
	v := departedView(1)
	at := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	first := v.messageChangedLocked([]int64{1})
	second := v.messageChangedLocked([]int64{1})
	if !v.refreshLocked([]api.MessageSummary{{ID: 1, Subject: "new", RemindAt: &at}}, second) {
		t.Fatal("the newer answer changed nothing")
	}
	if v.refreshLocked([]api.MessageSummary{{ID: 1, Subject: "old"}}, first) {
		t.Error("the older answer reports a change")
	}
	if r := v.rows[0]; r.Subject != "new" || r.RemindAt == nil {
		t.Errorf("row 0 = %+v, want the newer answer", r)
	}
}

// #317: a view.range page asked before a message.changed of one of its
// rows answers after it. The row was not loaded when the event came, so
// the event's refresh found nothing; the page's old row must be asked for
// again.
func TestAPageOlderThanAnEventIsAskedAgain(t *testing.T) {
	v := departedView()
	v.total = 2
	v.loading[0] = v.seq
	asked := v.seq
	event := v.messageChangedLocked([]int64{2})
	again := v.storeLocked(0, []api.MessageSummary{{ID: 1, Subject: "one"}, {ID: 2, Subject: "old"}}, asked)
	if !slices.Equal(again, []int64{2}) {
		t.Fatalf("asked again for %v, want [2]", again)
	}
	if v.refreshLocked([]api.MessageSummary{{ID: 2, Subject: "new"}}, event); v.rows[1].Subject != "new" {
		t.Errorf("row 1 = %+v, want the refresh", v.rows[1])
	}
}

// #317: a refresh newer than a page in flight answers first. The page's
// older copy must not replace it; the row keeps the page's index.
func TestAPageKeepsANewerRefresh(t *testing.T) {
	v := departedView(2)
	v.loading[0] = v.seq
	asked := v.seq
	event := v.messageChangedLocked([]int64{2})
	v.refreshLocked([]api.MessageSummary{{ID: 2, Subject: "new"}}, event)
	again := v.storeLocked(1, []api.MessageSummary{{ID: 2, Subject: "old"}}, asked)
	if len(again) != 0 {
		t.Errorf("asked again for %v, the newer row is held", again)
	}
	if _, ok := v.rows[0]; ok {
		t.Error("the copy at index 0 stayed")
	}
	if r := v.rows[1]; r.ID != 2 || r.Subject != "new" {
		t.Errorf("row 1 = %+v, want the refresh", r)
	}
}

// #317, end to end: the page answers after the message.changed with the
// old row, and the view asks for it again.
func TestAPageInFlightDuringAnEventEndsCurrent(t *testing.T) {
	s := newHoldServer(map[string]any{
		"view.open":         api.ViewInfo{ID: 1, Count: 1},
		"view.range":        []api.MessageSummary{{ID: 7, Subject: "old"}},
		"message.summaries": []api.MessageSummary{{ID: 7, Subject: "new"}},
	})
	s.held = "view.range"
	events := NewEvents()
	changed := make(chan struct{}, 1)
	v := OpenView(s.client(t), events, api.ViewQuery{}, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	defer v.Close()
	v.Ensure(0, 1)
	s.waitFor(t, "view.range")
	events.Deliver(api.MessageChanged{IDs: []int64{7}})
	close(s.release)
	timeout := time.After(10 * time.Second)
	for {
		if r, ok := v.Row(0); ok && r.Subject == "new" {
			return
		}
		select {
		case <-changed:
		case <-timeout:
			r, _ := v.Row(0)
			t.Fatalf("row 0 = %+v, want the row after the event", r)
		}
	}
}

// Pages that overlap without end, as in a long scroll: an event that every
// page still out was asked after is forgotten, though some page is always
// out.
func TestChangedIsPrunedWhilePagesOverlap(t *testing.T) {
	v := departedView()
	v.total = 300
	v.loading[0] = v.seq
	v.messageChangedLocked([]int64{5})
	v.loading[1] = v.seq
	v.messageChangedLocked([]int64{6})
	delete(v.loading, 0)
	v.pruneChangedLocked()
	if _, ok := v.changed[5]; ok || v.changed[6] == 0 {
		t.Errorf("after page 0: changed = %v, want only message 6", v.changed)
	}
	v.loading[2] = v.seq
	delete(v.loading, 1)
	v.pruneChangedLocked()
	if len(v.changed) != 0 {
		t.Errorf("after page 1: changed = %v, want none", v.changed)
	}
}
