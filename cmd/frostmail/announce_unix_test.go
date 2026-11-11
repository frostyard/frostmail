//go:build !windows

package main

import (
	"testing"

	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
)

// A click on a notification brings the window forward: the message would
// otherwise open behind whatever the user is looking at.
func TestNoteClickRaisesTheWindow(t *testing.T) {
	raised := 0
	post := func(fn func()) { fn() }
	w := mainwin.New(mainwin.Options{Target: conn.Target{Socket: "none"}, Post: post, Raise: func() { raised++ }})
	noteClicked(post, func() *mainwin.Window { return w }, 5)()
	if raised != 1 {
		t.Errorf("raised %d times", raised)
	}
}
