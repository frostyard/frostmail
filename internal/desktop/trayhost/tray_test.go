package trayhost

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostedAsksForTheWatcher(t *testing.T) {
	cases := []struct {
		name  string
		owned bool
		err   error
		want  bool
	}{
		{"owned", true, nil, true},
		{"not owned", false, nil, false},
		{"the bus fails", true, errors.New("no bus"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var asked []string
			h := &Hosts{
				ask: func(name string) (bool, error) {
					asked = append(asked, name)
					return tc.owned, tc.err
				},
				deadline: func() <-chan time.Time { return nil }, // never fires
			}
			if got := h.Hosted(); got != tc.want {
				t.Errorf("Hosted() = %v, want %v", got, tc.want)
			}
			if len(asked) != 1 || asked[0] != "org.kde.StatusNotifierWatcher" {
				t.Errorf("asked %q", asked)
			}
		})
	}
}

// A stalled bus answers false at the deadline, and the call after it waits
// on the same query rather than starting another.
func TestHostedGivesUpAtTheDeadline(t *testing.T) {
	release, started := make(chan struct{}), make(chan struct{}, 2)
	var asks atomic.Int32
	fired := make(chan time.Time)
	close(fired)
	h := &Hosts{
		ask: func(string) (bool, error) {
			asks.Add(1)
			started <- struct{}{}
			<-release
			return true, nil
		},
		deadline: func() <-chan time.Time { return fired },
	}
	if h.Hosted() {
		t.Error("a stalled query answered true")
	}
	<-started
	if h.Hosted() {
		t.Error("the second call answered true while the query stalls")
	}
	if n := asks.Load(); n != 1 {
		t.Errorf("%d queries ran, want 1", n)
	}
	close(release)
	h.deadline = func() <-chan time.Time { return nil } // never fires
	// The stalled query may still be in flight: the next call waits on it
	// and reads its answer, or starts a new one once it ended.
	if !h.Hosted() {
		t.Error("the answer after the stall is false")
	}
}

func TestNewHasItsPlatformsCalls(t *testing.T) {
	h := New()
	if h.ask == nil || h.deadline == nil {
		t.Fatal("New left a call unset")
	}
}
