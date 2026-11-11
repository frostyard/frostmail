package mailsync

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
)

// A Gmail pass records its entry, and counts the messages that changed, not
// the rows the server reported: a row can come back with nothing new.
func TestGmailActivityCountsWhatChanged(t *testing.T) {
	e := newGmailEnv(t)
	hub := activity.New(nil)
	e.a.m.cfg.Activity = hub
	g := e.g
	a := g.add(gAll, "A", 0, `\Inbox`)
	b := g.add(gAll, "B", 0, `\Inbox`)
	e.pass()

	all := func() api.ActivityEntry {
		t.Helper()
		entries := hub.Recent(activity.MaxRecent)
		for i := len(entries) - 1; i >= 0; i-- {
			if x := entries[i]; x.Kind == api.ActivityKindMailboxsynced && *x.MailboxID == e.mbs[gAll] {
				return x
			}
		}
		t.Fatal("no entry of All Mail's pass")
		return api.ActivityEntry{}
	}
	if x := all(); *x.Added != 2 || *x.Changed != 0 {
		t.Errorf("the first pass = %+v, want 2 added, 0 changed", x)
	}

	// Both rows come back; only A's labels are new.
	g.label(a, []string{"Work"}, nil)
	g.touch(b)
	e.pass()
	if x := all(); *x.Changed != 1 {
		t.Errorf("the pass of one label change = %+v, want 1 changed", x)
	}
}
