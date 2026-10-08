package imapx_test

import (
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
