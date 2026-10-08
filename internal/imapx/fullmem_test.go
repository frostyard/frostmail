package imapx_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
)

func TestFullMemMoves(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	seedMem(t, mem)
	s, err := imapx.Open(t.Context(), mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Caps.Move || !s.Caps.UIDPlus || !s.Caps.ESearch {
		t.Fatalf("caps = %+v", s.Caps)
	}
	if _, err := s.Select(t.Context(), "INBOX"); err != nil {
		t.Fatal(err)
	}
	moved, err := s.Move(t.Context(), []uint32{1, 2}, "Archive")
	if err != nil || len(moved) != 2 || moved[1] == 0 {
		t.Fatalf("move = %v, %v", moved, err)
	}
}

func TestFullMemAppendAndSearchMessageID(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	s, err := imapx.Open(t.Context(), mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	raw := []byte("Message-ID: <draft-1@x.test>\r\nSubject: draft\r\n\r\nbody\r\n")
	uid, err := s.Append(t.Context(), "Drafts", raw, []string{`\Draft`, `\Seen`})
	if err != nil || uid == 0 {
		t.Fatalf("append = %d, %v; want a UID from UIDPLUS", uid, err)
	}
	if _, err := s.Append(t.Context(), "Drafts", []byte("Message-ID: <other@x.test>\r\n\r\nx\r\n"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Select(t.Context(), "Drafts"); err != nil {
		t.Fatal(err)
	}
	got, err := s.SearchMessageID(t.Context(), "draft-1@x.test")
	if err != nil || len(got) != 1 || got[0] != uid {
		t.Fatalf("SearchMessageID = %v, %v; want [%d]", got, err, uid)
	}
	none, err := s.SearchMessageID(t.Context(), "missing@x.test")
	if err != nil || len(none) != 0 {
		t.Fatalf("SearchMessageID(missing) = %v, %v", none, err)
	}
}

func TestReadOnlySessionExaminesAndRefusesChanges(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	seedMem(t, mem)
	opts := mem.DialOptions()
	opts.ReadOnly = true
	trace := &lockedTrace{}
	opts.Trace = trace
	s, err := imapx.Open(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Select(t.Context(), "INBOX"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(trace.String(), "EXAMINE INBOX") || strings.Contains(trace.String(), "SELECT INBOX") {
		t.Errorf("a read-only session must EXAMINE:\n%s", trace.String())
	}
	if uids, err := s.UIDs(t.Context()); err != nil || len(uids) == 0 {
		t.Fatalf("uids = %v, %v", uids, err)
	}
	before := len(trace.String())
	checks := map[string]error{}
	checks["store flags"] = s.StoreFlags(t.Context(), []uint32{1}, []string{`\Seen`}, nil)
	checks["store labels"] = s.StoreLabels(t.Context(), []uint32{1}, []string{"x"}, nil)
	_, checks["move"] = s.Move(t.Context(), []uint32{1}, "Archive")
	checks["expunge"] = s.Expunge(t.Context(), []uint32{1})
	_, checks["append"] = s.Append(t.Context(), "Drafts", []byte("Subject: x\r\n\r\nx\r\n"), nil)
	for what, err := range checks {
		if !errors.Is(err, imapx.ErrReadOnly) {
			t.Errorf("%s = %v, want ErrReadOnly", what, err)
		}
	}
	if len(trace.String()) != before {
		t.Errorf("refused commands reached the server:\n%s", trace.String()[before:])
	}
}

// lockedTrace collects a trace written from the connection's goroutine.
type lockedTrace struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedTrace) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedTrace) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
