package live

import (
	"bufio"
	"encoding/json/v2"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// holdServer answers what a test says, and holds a method's replies until
// release is closed.
type holdServer struct {
	mu      sync.Mutex
	answers map[string]any
	held    string
	release chan struct{}
	asked   chan api.Frame
}

func newHoldServer(answers map[string]any) *holdServer {
	return &holdServer{answers: answers, release: make(chan struct{}), asked: make(chan api.Frame, 64)}
}

func (s *holdServer) client(t *testing.T) *api.Client {
	t.Helper()
	near, far := net.Pipe()
	c := api.NewClient(near)
	t.Cleanup(func() {
		_ = c.Close()
		_ = far.Close()
	})
	var write sync.Mutex
	go func() {
		lines := bufio.NewScanner(far)
		for lines.Scan() {
			var req api.Frame
			if err := json.Unmarshal(lines.Bytes(), &req); err != nil {
				return
			}
			go s.answer(far, &write, req)
		}
	}()
	return c
}

func (s *holdServer) answer(conn net.Conn, write *sync.Mutex, req api.Frame) {
	s.asked <- req
	if req.Method == s.held {
		<-s.release
	}
	s.mu.Lock()
	result, ok := s.answers[req.Method]
	s.mu.Unlock()
	if !ok {
		result = struct{}{}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return
	}
	line, err := json.Marshal(api.Frame{JSONRPC: "2.0", ID: req.ID, Result: data})
	if err != nil {
		return
	}
	write.Lock()
	defer write.Unlock()
	_, _ = conn.Write(append(line, '\n'))
}

// waitFor reads the requests until one of method comes.
func (s *holdServer) waitFor(t *testing.T, method string) api.Frame {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case req := <-s.asked:
			if req.Method == method {
				return req
			}
		case <-timeout:
			t.Fatalf("no %s", method)
		}
	}
}

// Close during view.open: maild opens the view whatever the client does
// meanwhile, and only its answer says which view to close. The client may
// see the answer and the Close at once, so the test closes many times.
func TestAViewClosedWhileOpeningIsClosedInMaild(t *testing.T) {
	for range 20 {
		s := newHoldServer(map[string]any{"view.open": api.ViewInfo{ID: 42, Count: 0}})
		s.held = "view.open"
		v := OpenView(s.client(t), NewEvents(), api.ViewQuery{}, func() {})
		s.waitFor(t, "view.open")
		v.Close()
		close(s.release)
		req := s.waitFor(t, "view.close")
		var p api.ViewCloseParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			t.Fatal(err)
		}
		if p.ID != 42 {
			t.Fatalf("closed view %d, want 42", p.ID)
		}
	}
}

// An answer that comes after the store moved to another connection is the
// last maild's, and must not land in this one's state.
func TestAnAnswerFromTheLastConnectionIsDropped(t *testing.T) {
	old := newHoldServer(map[string]any{"rule.list": []api.Rule{{ID: 7, Name: "Old maild's"}}})
	old.held = "rule.list"
	m := NewMail(func() {})
	m.c, m.ctx = old.client(t), t.Context()
	done := make(chan struct{})
	go func() { m.loadRules(); close(done) }()
	old.waitFor(t, "rule.list")
	next := newHoldServer(nil)
	m.mu.Lock()
	m.c = next.client(t)
	m.mu.Unlock()
	close(old.release)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the load did not end")
	}
	if got := m.State().Rules; len(got) != 0 {
		t.Errorf("rules = %+v, from the last maild", got)
	}
}

func TestForgetEmptiesTheStore(t *testing.T) {
	notified := 0
	m := NewMail(func() { notified++ })
	m.state = MailState{Loaded: true, Accounts: []api.Account{{ID: 1}}, VIPs: []api.Vip{{Address: "ann@x.test"}}, Rules: []api.Rule{{ID: 1}},
		Settings: &api.Settings{}, Smart: []api.SmartMailbox{{ID: 1}}, FollowUp: &api.Awaiting{}}
	m.Forget()
	st := m.State()
	if st.Loaded || st.Accounts != nil || st.VIPs != nil || st.Rules != nil || st.Settings != nil || st.Smart != nil || st.FollowUp != nil {
		t.Errorf("state = %+v", st)
	}
	if notified != 1 {
		t.Errorf("notified %d times", notified)
	}
}

// While activity.recent waits, what comes is bounded, and what had no room
// is counted.
func TestPendingActivityIsBounded(t *testing.T) {
	var got []api.ActivityEntry
	a := NewActivity(func(e api.ActivityEntry) { got = append(got, e) })
	const extra = 40
	for i := range maxPending + extra {
		a.event(api.ActivityAdded{Entry: api.ActivityEntry{Seq: int64(i + 1), Kind: api.ActivityKindPushed}})
	}
	if len(a.pending) != maxPending {
		t.Fatalf("%d entries pending, want %d", len(a.pending), maxPending)
	}
	s := newHoldServer(map[string]any{"activity.subscribe": api.ActivitySubscription{Seq: 0}, "activity.recent": []api.ActivityEntry{}})
	a.mu.Lock()
	kept := slices.Clone(a.pending)
	lost := a.lost
	a.mu.Unlock()
	if lost != extra {
		t.Fatalf("lost %d, want %d", lost, extra)
	}
	if kept[0].Seq != extra+1 {
		t.Errorf("the oldest kept is %d, want the newest to stay", kept[0].Seq)
	}
	events := NewEvents()
	// Connect starts over, so fill it again once it listens.
	off := events.On(func(api.Event) {})
	defer off()
	s.held = "activity.recent"
	done := make(chan error, 1)
	go func() { done <- a.Connect(t.Context(), s.client(t), events) }()
	s.waitFor(t, "activity.recent")
	for i := range maxPending + extra {
		events.Deliver(api.ActivityAdded{Entry: api.ActivityEntry{Seq: int64(i + 1), Kind: api.ActivityKindPushed}})
	}
	close(s.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(got) != maxPending+1 {
		t.Fatalf("handed on %d entries, want %d and one dropped", len(got), maxPending)
	}
	if got[0].Kind != api.ActivityKindDropped || got[0].N == nil || *got[0].N != extra {
		t.Errorf("first = %+v, want %d dropped", got[0], extra)
	}
}

func departedView(rows ...int64) *View {
	v := &View{opened: true, id: 1, total: int64(len(rows)), rows: map[int64]api.MessageSummary{}, loading: map[int64]bool{}, departed: map[int64]bool{}}
	for i, id := range rows {
		v.rows[int64(i)] = api.MessageSummary{ID: id}
	}
	v.window = [2]int64{0, int64(len(rows))}
	return v
}

func TestTakeDepartedNamesWhatLeft(t *testing.T) {
	v := departedView(1, 2, 3)
	v.applyLocked(api.ViewDelta{ID: 1, Count: 2, Ops: []api.ViewOp{{Op: api.ViewOpKindRemove, At: 1, Count: 1}}})
	if got := v.TakeDeparted(); !slices.Equal(got, []int64{2}) {
		t.Errorf("departed = %v, want [2]", got)
	}
	if got := v.TakeDeparted(); got != nil {
		t.Errorf("taken twice: %v", got)
	}
}

// A row that moves is a remove and an insert: it has not left, and says so
// when its page comes back.
func TestTakeDepartedWaitsForAMovedRow(t *testing.T) {
	v := departedView(1, 2, 3)
	v.applyLocked(api.ViewDelta{ID: 1, Count: 3, Ops: []api.ViewOp{
		{Op: api.ViewOpKindRemove, At: 2, Count: 1}, {Op: api.ViewOpKindInsert, At: 0, Count: 1}}})
	if got := v.TakeDeparted(); got != nil {
		t.Errorf("departed %v while row 0 is not loaded", got)
	}
	v.storeLocked(0, summaries(3, 1, 2))
	if got := v.TakeDeparted(); got != nil {
		t.Errorf("departed = %v, the row only moved", got)
	}
}

// A row dropped to save memory has not left the view.
func TestEvictedRowsHaveNotDeparted(t *testing.T) {
	v := departedView(1, 2)
	v.window = [2]int64{0, 0}
	clear(v.rows)
	v.evictLocked()
	if got := v.TakeDeparted(); got != nil {
		t.Errorf("departed = %v", got)
	}
}
