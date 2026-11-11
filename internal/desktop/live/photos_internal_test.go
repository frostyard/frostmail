package live

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// peopleServer answers people.senders and people.photo on a connection, as
// each test says: maild does not fail on demand.
type peopleServer struct {
	mu          sync.Mutex
	failSenders bool
	failPhoto   bool
	hold        chan struct{} // when set, people.senders waits for it
	senders     [][]string    // the addresses of each people.senders
	photos      int
}

func (s *peopleServer) client(t *testing.T) *api.Client {
	t.Helper()
	near, far := net.Pipe()
	c := api.NewClient(near)
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
			go s.answer(far, req)
		}
	}()
	return c
}

func (s *peopleServer) answer(conn net.Conn, req api.Frame) {
	res := api.Frame{JSONRPC: "2.0", ID: req.ID}
	var result any
	fail := false
	switch req.Method {
	case "people.senders":
		var p api.PeopleSendersParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return
		}
		s.mu.Lock()
		s.senders = append(s.senders, p.Addresses)
		fail = s.failSenders
		hold := s.hold
		s.mu.Unlock()
		if hold != nil {
			<-hold
		}
		out := []api.SenderPhoto{}
		for _, a := range p.Addresses {
			out = append(out, api.SenderPhoto{Address: a, PersonID: 7})
		}
		result = out
	case "people.photo":
		s.mu.Lock()
		s.photos++
		fail = s.failPhoto
		s.mu.Unlock()
		result = api.Photo{ContentType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("picture"))}
	}
	if fail {
		res.Error = &api.Error{Code: api.ErrorCode(-32000), Message: "no"}
	} else {
		raw, err := json.Marshal(result)
		if err != nil {
			return
		}
		res.Result = jsontext.Value(raw)
	}
	line, err := json.Marshal(res)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(line, '\n'))
}

func (s *peopleServer) counts() (senders, photos int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.senders), s.photos
}

func (s *peopleServer) set(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

func until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for !ok() {
		select {
		case <-time.After(time.Millisecond):
		case <-timeout:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func TestPhotosAskAgainAfterAFailedLookup(t *testing.T) {
	s := &peopleServer{failSenders: true}
	p := NewPhotos(t.Context(), s.client(t), func() {})
	want := []string{"Ann@x.test"}

	until(t, "people.senders to be asked twice", func() bool {
		p.Want(want)
		n, _ := s.counts()
		return n >= 2
	})
	s.set(func() { s.failSenders, s.failPhoto = false, true })
	until(t, "people.photo to be asked twice", func() bool {
		p.Want(want)
		_, n := s.counts()
		return n >= 2
	})
	s.set(func() { s.failPhoto = false })
	until(t, "the photo", func() bool {
		p.Want(want)
		return p.Get("ann@x.test") != nil
	})
	if got := string(p.Get("ann@x.test")); got != "picture" {
		t.Errorf("photo = %q", got)
	}

	// Found once, it is not asked for again.
	p.Want(want)
	p.Want([]string{"bo@x.test"})
	until(t, "the next address's lookup", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return slices.Contains(s.senders[len(s.senders)-1], "bo@x.test")
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	if last := s.senders[len(s.senders)-1]; slices.Contains(last, "ann@x.test") {
		t.Errorf("the last lookup asked for a known address: %q", last)
	}
}

func TestPhotosEndWithTheirContext(t *testing.T) {
	s := &peopleServer{hold: make(chan struct{})}
	t.Cleanup(func() { close(s.hold) })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p := NewPhotos(ctx, s.client(t), func() {})

	p.Want([]string{"ann@x.test"})
	until(t, "the lookup to reach maild", func() bool { n, _ := s.counts(); return n == 1 })
	cancel()
	// Its answer never comes: only the context can end the lookup.
	until(t, "the lookup to end", func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return !p.asked["ann@x.test"]
	})

	p.Want([]string{"bo@x.test"})
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.asked) != 0 {
		t.Errorf("after the connection closed, asked for %v", p.asked)
	}
}
