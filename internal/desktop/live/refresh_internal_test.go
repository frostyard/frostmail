package live

import (
	"bufio"
	"encoding/json/v2"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// scriptServer hands each request to a test, which answers it when it
// chooses: the order of the answers is the test's.
type scriptServer struct {
	conn  net.Conn
	write sync.Mutex
	asked chan api.Frame
}

func newScriptServer(t *testing.T) (*scriptServer, *api.Client) {
	t.Helper()
	near, far := net.Pipe()
	c := api.NewClient(near)
	s := &scriptServer{conn: far, asked: make(chan api.Frame, 64)}
	t.Cleanup(func() {
		_ = c.Close()
		_ = far.Close()
	})
	go func() {
		lines := bufio.NewScanner(far)
		for lines.Scan() {
			var req api.Frame
			if err := json.Unmarshal(lines.Bytes(), &req); err != nil {
				return
			}
			s.asked <- req
		}
	}()
	return s, c
}

// next returns the next request, which must be of method.
func (s *scriptServer) next(t *testing.T, method string) api.Frame {
	t.Helper()
	select {
	case req := <-s.asked:
		if req.Method != method {
			t.Fatalf("asked %s, want %s", req.Method, method)
		}
		return req
	case <-time.After(10 * time.Second):
		t.Fatalf("no %s", method)
		return api.Frame{}
	}
}

func (s *scriptServer) answer(t *testing.T, req api.Frame, result any) {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	line, err := json.Marshal(api.Frame{JSONRPC: "2.0", ID: req.ID, Result: data})
	if err != nil {
		t.Fatal(err)
	}
	s.write.Lock()
	defer s.write.Unlock()
	if _, err := s.conn.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}

// scriptView is an open view of 1 row on a script server; changed gets a
// value at each change.
func scriptView(t *testing.T, rows map[int64]api.MessageSummary) (*View, *scriptServer, chan struct{}) {
	t.Helper()
	s, c := newScriptServer(t)
	changed := make(chan struct{}, 16)
	v := &View{c: c, ctx: t.Context(), opened: true, id: 1, total: 1, rows: rows, window: [2]int64{0, 1},
		loading: map[int64]bool{}, departed: map[int64]bool{}, stale: map[int64]bool{}, changing: map[int64]bool{},
		notify: func() {
			select {
			case changed <- struct{}{}:
			default:
			}
		}}
	return v, s, changed
}

// untilRow waits for a change that leaves row 0 as ok wants.
func untilRow(t *testing.T, v *View, changed chan struct{}, what string, ok func(api.MessageSummary) bool) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		if r, has := v.Row(0); has && ok(r) {
			return
		}
		select {
		case <-changed:
		case <-timeout:
			r, _ := v.Row(0)
			t.Fatalf("no %s; row 0 is %+v", what, r)
		}
	}
}

// Two changes to one message, close together: the first refresh's answer is
// from before the second change. Side by side, the older answer could land
// last and stay (issue 317: a row with no reminder mark). The second
// refresh asks only after the first has its answer.
func TestRefreshesOfOneViewRunOneAtATime(t *testing.T) {
	v, s, changed := scriptView(t, map[int64]api.MessageSummary{0: {ID: 1}})
	at := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	v.event(api.MessageChanged{IDs: []int64{1}})
	first := s.next(t, "message.summaries")
	v.event(api.MessageChanged{IDs: []int64{1}})
	select {
	case req := <-s.asked:
		t.Fatalf("%s was asked while the first refresh had no answer", req.Method)
	default:
	}
	v.mu.Lock()
	queued := v.stale[1]
	v.mu.Unlock()
	if !queued {
		t.Fatal("the second change is not queued behind the first refresh")
	}
	s.answer(t, first, []api.MessageSummary{{ID: 1, Subject: "old"}})
	second := s.next(t, "message.summaries")
	s.answer(t, second, []api.MessageSummary{{ID: 1, Subject: "new", RemindAt: &at}})
	untilRow(t, v, changed, "refreshed row", func(r api.MessageSummary) bool { return r.RemindAt != nil })
	if r, _ := v.Row(0); r.Subject != "new" {
		t.Errorf("row 0 is %+v, want the second answer", r)
	}
}

// A message changes while its page is in flight, and the page's answer is
// from before the change. The row is not loaded when the event comes, so
// only the page's answer can say that it needs a refresh.
func TestARowChangedWhileItsPageLoadsIsReadAgain(t *testing.T) {
	v, s, changed := scriptView(t, map[int64]api.MessageSummary{})
	at := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	v.Ensure(0, 1)
	page := s.next(t, "view.range")
	v.event(api.MessageChanged{IDs: []int64{1}})
	s.answer(t, page, []api.MessageSummary{{ID: 1, Subject: "old"}})
	again := s.next(t, "message.summaries")
	s.answer(t, again, []api.MessageSummary{{ID: 1, Subject: "new", RemindAt: &at}})
	untilRow(t, v, changed, "refreshed row", func(r api.MessageSummary) bool { return r.RemindAt != nil })
	v.mu.Lock()
	left := len(v.changing)
	v.mu.Unlock()
	if left != 0 {
		t.Errorf("%d messages still kept for a page, with none in flight", left)
	}
}
