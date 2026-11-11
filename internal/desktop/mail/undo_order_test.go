package mail_test

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"errors"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// moveServer answers message.summaries and message.move for message 1,
// keeping its mailbox, and answers a move to slow late.
type moveServer struct {
	mu      sync.Mutex
	box     int64
	moves   []int64
	slow    int64
	started chan int64
	release <-chan struct{}
	skips   int // moves still to report done without doing, as maild does for a move not yet replayed
	// failRead is the message.summaries call, counted from 1, answered with
	// an error; reads counts them.
	failRead, reads int
}

func (s *moveServer) client(t *testing.T) *api.Client {
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

func (s *moveServer) answer(conn net.Conn, write *sync.Mutex, req api.Frame) {
	var result any
	var failure *api.Error
	switch req.Method {
	case "message.summaries":
		s.mu.Lock()
		result = []api.MessageSummary{{ID: 1, MailboxIDs: []int64{s.box}}}
		s.reads++
		if s.reads == s.failRead {
			failure = api.Errorf(api.CodeInternal, "the read failed")
		}
		s.mu.Unlock()
	case "message.move":
		var p api.MessageMoveParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return
		}
		s.mu.Lock()
		slow := p.MailboxID == s.slow
		s.mu.Unlock()
		if s.started != nil {
			s.started <- p.MailboxID
		}
		if slow {
			<-s.release
		}
		s.mu.Lock()
		if s.skips > 0 {
			s.skips--
		} else {
			s.box = p.MailboxID
			s.moves = append(s.moves, p.MailboxID)
		}
		s.mu.Unlock()
		result = struct{}{}
	default:
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		return
	}
	frame := api.Frame{JSONRPC: "2.0", ID: req.ID, Result: data}
	if failure != nil {
		frame.Result, frame.Error = nil, failure
	}
	line, err := json.Marshal(frame)
	if err != nil {
		return
	}
	write.Lock()
	defer write.Unlock()
	_, _ = conn.Write(append(line, '\n'))
}

// Two undos at once run newest first: a message moved twice comes all the
// way back.
func TestUndosRunOneAtATime(t *testing.T) {
	s := &moveServer{box: 10}
	c := s.client(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var u mail.Undo
	for _, to := range []int64{20, 30} {
		if _, err := u.Move(ctx, c, []int64{1}, func() error {
			return c.Message().Move(ctx, &api.MessageMoveParams{IDs: []int64{1}, MailboxID: to})
		}); err != nil {
			t.Fatal(err)
		}
	}
	s.mu.Lock()
	s.slow = 20
	s.mu.Unlock()
	release := make(chan struct{})
	var workers sync.WaitGroup
	defer func() {
		close(release)
		cancel()
		workers.Wait()
	}()
	s.started, s.release = make(chan int64, 2), release
	waiting := make(chan struct{}, 2)
	u.BeforeUndo = func() { waiting <- struct{}{} }
	done := make(chan error, 2)
	run := func() {
		_, err := u.Do(ctx, c)
		done <- err
	}
	workers.Go(run)
	select {
	case to := <-s.started:
		if to != 20 {
			t.Fatalf("first undo moves to %d, want 20", to)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("first undo did not reach the server")
	}
	<-waiting
	workers.Go(run)
	select {
	case <-waiting:
	case <-time.After(30 * time.Second):
		t.Fatal("second undo did not reach its wait")
	}
	select {
	case to := <-s.started:
		t.Fatalf("second undo overtook the held move: %d", to)
	case <-time.After(50 * time.Millisecond):
		// Only a bound on the negative assertion: the first move stays
		// held until this check ends, however slow the runner is.
	}
	release <- struct{}{}
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("undo: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("undo did not finish")
		}
	}
	if want := []int64{20, 30, 20, 10}; !slices.Equal(s.moves, want) {
		t.Errorf("moves = %v, want %v", s.moves, want)
	}
}

// One account's messages moved and the next account's failed: the move
// made can still be undone.
func TestAMoveThatFailsPartWayCanBeUndone(t *testing.T) {
	s := &moveServer{box: 10}
	c := s.client(t)
	ctx := t.Context()
	u := mail.Undo{Retry: time.Millisecond}
	refused := errors.New("refused")
	after, err := u.Move(ctx, c, []int64{1}, func() error {
		if err := c.Message().Move(ctx, &api.MessageMoveParams{IDs: []int64{1}, MailboxID: 20}); err != nil {
			return err
		}
		return refused // the next account's group
	})
	if !errors.Is(err, refused) {
		t.Fatalf("move = %v, want the group's error", err)
	}
	if len(after) != 1 || !slices.Equal(after[0].MailboxIDs, []int64{20}) {
		t.Errorf("after = %+v", after)
	}
	if u.Len() != 1 {
		t.Fatalf("%d entries, want the group that moved", u.Len())
	}
	if ok, err := u.Do(ctx, c); !ok || err != nil {
		t.Fatalf("undo = %v, %v", ok, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.box != 10 {
		t.Errorf("the message is in %d, want 10", s.box)
	}
}

// An action done but not read back says so, apart from one that failed:
// it was done, and Undo cannot take it back.
func TestAMoveNotReadBack(t *testing.T) {
	s := &moveServer{box: 10, failRead: 2}
	c := s.client(t)
	ctx := t.Context()
	var u mail.Undo
	after, err := u.Move(ctx, c, []int64{1}, func() error {
		return c.Message().Move(ctx, &api.MessageMoveParams{IDs: []int64{1}, MailboxID: 20})
	})
	if !errors.Is(err, mail.ErrNotRemembered) || after != nil {
		t.Fatalf("move = %+v, %v; want nothing read and ErrNotRemembered", after, err)
	}
	if u.Len() != 0 {
		t.Errorf("%d entries", u.Len())
	}
}

func movedOnce(t *testing.T, u *mail.Undo, c *api.Client) {
	t.Helper()
	ctx := t.Context()
	if _, err := u.Move(ctx, c, []int64{1}, func() error {
		return c.Message().Move(ctx, &api.MessageMoveParams{IDs: []int64{1}, MailboxID: 20})
	}); err != nil {
		t.Fatal(err)
	}
}

// maild skips a move whose message has an earlier move not yet replayed,
// and reports it done: the undo goes on until it takes effect.
func TestUndoWaitsForAMoveStillQueued(t *testing.T) {
	s := &moveServer{box: 10}
	c := s.client(t)
	u := mail.Undo{Retry: time.Millisecond, Patience: 10 * time.Second}
	movedOnce(t, &u, c)
	s.mu.Lock()
	s.skips = 3
	s.mu.Unlock()
	if ok, err := u.Do(t.Context(), c); !ok || err != nil {
		t.Fatalf("undo = %v, %v", ok, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.box != 10 {
		t.Errorf("the message is in %d, want 10", s.box)
	}
	if u.Len() != 0 {
		t.Errorf("%d entries left", u.Len())
	}
}

// A move that never lands says so, and can be undone later.
func TestUndoKeepsAnEntryStillMoving(t *testing.T) {
	s := &moveServer{box: 10}
	c := s.client(t)
	u := mail.Undo{Retry: time.Millisecond, Patience: 5 * time.Millisecond}
	movedOnce(t, &u, c)
	s.mu.Lock()
	s.skips = 1 << 30
	s.mu.Unlock()
	if _, err := u.Do(t.Context(), c); !errors.Is(err, mail.ErrUndoWaiting) {
		t.Fatalf("undo = %v, want ErrUndoWaiting", err)
	}
	if u.Len() != 1 {
		t.Fatalf("%d entries left, want the one that did not land", u.Len())
	}
	s.mu.Lock()
	s.skips = 0
	s.mu.Unlock()
	if ok, err := u.Do(t.Context(), c); !ok || err != nil {
		t.Fatalf("second undo = %v, %v", ok, err)
	}
	if u.Len() != 0 {
		t.Errorf("%d entries left", u.Len())
	}
}

func TestSelectionsMoveWithinTheirAccounts(t *testing.T) {
	mbs := []api.Mailbox{
		{ID: 1, AccountID: 1, Role: api.MailboxRoleInbox}, {ID: 2, AccountID: 1, Role: api.MailboxRoleJunk}, {ID: 3, AccountID: 1, Role: api.MailboxRoleArchive},
		{ID: 4, AccountID: 2, Role: api.MailboxRoleInbox}, {ID: 5, AccountID: 2, Role: api.MailboxRoleJunk}, {ID: 6, AccountID: 2, Role: api.MailboxRoleArchive},
		{ID: 7, AccountID: 3, Role: api.MailboxRoleInbox},
	}
	rows := []api.MessageSummary{{ID: 11, AccountID: 1, MailboxIDs: []int64{1}}, {ID: 21, AccountID: 2, MailboxIDs: []int64{4}},
		{ID: 12, AccountID: 1, MailboxIDs: []int64{1}}, {ID: 31, AccountID: 3, MailboxIDs: []int64{7}}}
	want := func(what string, got []mail.MoveGroup, ids [][]int64, to []int64) {
		t.Helper()
		if len(got) != len(ids) {
			t.Fatalf("%s: %+v", what, got)
		}
		for i, g := range got {
			if !slices.Equal(g.IDs, ids[i]) || g.To.ID != to[i] {
				t.Errorf("%s group %d = %v to %d, want %v to %d", what, i, g.IDs, g.To.ID, ids[i], to[i])
			}
		}
	}
	want("archive", mail.ArchiveGroups(rows, mbs), [][]int64{{11, 12}, {21}}, []int64{3, 6})
	notSpam, groups := mail.SpamGroups(rows, mbs)
	if notSpam {
		t.Error("rows in inboxes are Not Spam")
	}
	want("spam", groups, [][]int64{{11, 12}, {21}}, []int64{2, 5})
	junked := []api.MessageSummary{{ID: 11, AccountID: 1, MailboxIDs: []int64{2}}, {ID: 21, AccountID: 2, MailboxIDs: []int64{5}}}
	notSpam, groups = mail.SpamGroups(junked, mbs)
	if !notSpam {
		t.Error("rows each in their Junk are spam")
	}
	want("not spam", groups, [][]int64{{11}, {21}}, []int64{1, 4})
}
