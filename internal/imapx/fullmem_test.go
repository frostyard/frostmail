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
