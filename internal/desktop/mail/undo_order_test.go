package mail_test

import (
	"bufio"
	"encoding/json/v2"
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
	mu    sync.Mutex
	box   int64
	moves []int64
	slow  int64
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
	switch req.Method {
	case "message.summaries":
		s.mu.Lock()
		result = []api.MessageSummary{{ID: 1, MailboxIDs: []int64{s.box}}}
		s.mu.Unlock()
	case "message.move":
		var p api.MessageMoveParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return
		}
		s.mu.Lock()
		slow := p.MailboxID == s.slow
		s.mu.Unlock()
		if slow {
			time.Sleep(50 * time.Millisecond) // lets a second undo overtake, if it can
		}
		s.mu.Lock()
		s.box = p.MailboxID
		s.moves = append(s.moves, p.MailboxID)
		s.mu.Unlock()
		result = struct{}{}
	default:
		return
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

// Two undos at once run newest first: a message moved twice comes all the
// way back.
func TestUndosRunOneAtATime(t *testing.T) {
	s := &moveServer{box: 10}
	c := s.client(t)
	ctx := t.Context()
	var u mail.Undo
	for _, to := range []int64{20, 30} {
		if err := u.Move(ctx, c, []int64{1}, func() error {
			return c.Message().Move(ctx, &api.MessageMoveParams{IDs: []int64{1}, MailboxID: to})
		}); err != nil {
			t.Fatal(err)
		}
	}
	s.mu.Lock()
	s.slow = 20
	s.mu.Unlock()
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if _, err := u.Do(ctx, c); err != nil {
				t.Errorf("undo: %v", err)
			}
		})
	}
	wg.Wait()
	if want := []int64{20, 30, 20, 10}; !slices.Equal(s.moves, want) {
		t.Errorf("moves = %v, want %v", s.moves, want)
	}
}
