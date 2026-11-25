//go:build linux && integration

package main

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/problemstest"
	"github.com/frostyard/std/problems"
)

// TestLostLocalSocketIsRecorded checks the client's problem record
// (ADR-0049): when maild's local socket ends under the app, 1 record of
// bridge.socket_lost goes to the journal, and the client says it is
// reconnecting. A client that never connected, or one that is told
// nothing, records nothing.
func TestLostLocalSocketIsRecorded(t *testing.T) {
	journal := problemstest.Listen(t)
	rec := journal.Recorder(t, problems.ComponentShell)
	var lost atomic.Int32
	a, daemon := fixtureView(t, rootWith(func(err error) {
		lost.Add(1)
		rec.Error(codeSocketLost, err)
	}), "", 1280, 820, false)
	a.WaitFor("id:messages >> id:summary-when >> nth=0", 8*time.Second)
	journal.None(t)
	if err := daemon.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	a.WaitFor("id:connection >> text:Reconnecting", 10*time.Second)
	a.WaitUntil(5*time.Second, func() bool { return lost.Load() >= 1 })
	record := journal.NextError(t, problems.ComponentShell, codeSocketLost)
	if strings.TrimSpace(record.Message()) == "" {
		t.Errorf("the record has no message: %+v", record.Fields)
	}
	// The retries that follow are no new loss: the client was not
	// connected when they failed.
	a.Advance(3 * time.Second)
	a.Settle()
	if n := lost.Load(); n != 1 {
		t.Errorf("the lost socket was told %d times, want once", n)
	}
	journal.None(t)
}
