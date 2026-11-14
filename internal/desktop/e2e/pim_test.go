//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

func showcase(t *testing.T) *session {
	t.Helper()
	return start(t, options{fixture: []string{"-showcase", filepath.Join(repoRoot, "tools", "uifixture", "showcase")}})
}

// writable is a calendar events can be made in.
func writable(t *testing.T, s *session) api.Collection {
	t.Helper()
	kind := api.CollectionKindCalendar
	cols, err := s.client.Account().Collections(t.Context(), &api.AccountCollectionsParams{Kind: &kind})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cols {
		if !c.ReadOnly && c.Events && c.Enabled {
			return c
		}
	}
	t.Fatalf("no writable calendar among %+v", cols)
	return api.Collection{}
}

// newEvent makes a timed event in the machine's zone.
func newEvent(ctx context.Context, t *testing.T, s *session, cal int64, summary string, start time.Time, alarms []int64) *api.EventWrite {
	t.Helper()
	zone, allDay := "UTC", false
	end := start.Add(time.Hour)
	start, end = start.UTC(), end.UTC()
	w, err := s.client.Calendar().Create(ctx, &api.CalendarCreateParams{CalendarID: &cal,
		Fields: api.EventFields{Summary: &summary, AllDay: &allDay, Start: &start, End: &end, TimeZone: &zone, Alarms: alarms}})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Dragging an event down the week moves it.
func TestDragMovesAnEvent(t *testing.T) {
	s := showcase(t)
	ctx := t.Context()
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 11, 0, 0, 0, time.Local)
	newEvent(ctx, t, s, writable(t, s).ID, "Drag me", start, nil)
	s.key(1, "CmdOrCtrl+2")
	s.key(1, "CmdOrCtrl+Alt+2")
	n := s.waitNode(1, "", "^Drag me")
	x, y := n.Bounds.X+n.Bounds.Width/2, n.Bounds.Y+n.Bounds.Height/2
	const hour = 60 * 0.8 // the week's pixels per minute
	s.input(1, map[string]any{"type": "pointerMove", "x": x, "y": y})
	s.input(1, map[string]any{"type": "pointerDown", "x": x, "y": y})
	for step := 1.0; step <= 6; step++ {
		s.input(1, map[string]any{"type": "pointerMove", "x": x, "y": y + hour*step/6})
	}
	s.input(1, map[string]any{"type": "pointerUp", "x": x, "y": y + hour})

	day := start.Format("2006-01-02")
	next := start.AddDate(0, 0, 1).Format("2006-01-02")
	eventually(t, "the event an hour later", func() bool {
		occ, err := s.client.Calendar().Range(ctx, &api.CalendarRangeParams{From: day, To: next})
		if err != nil {
			return false
		}
		for _, o := range occ {
			if o.Summary == "Drag me" {
				return o.Start.Equal(start.Add(time.Hour))
			}
		}
		return false
	})
}

// A due alert opens the reminders window; Dismiss answers it, and it closes
// with nothing left.
func TestRemindersWindowAnswers(t *testing.T) {
	s := showcase(t)
	ctx := t.Context()
	cal := writable(t, s).ID
	due := time.Now().Truncate(time.Minute).Add(time.Minute)
	if time.Until(due) < 10*time.Second {
		due = due.Add(time.Minute)
	}
	newEvent(ctx, t, s, cal, "Remind one", due.Add(5*time.Minute), []int64{5})
	newEvent(ctx, t, s, cal, "Remind two", due.Add(5*time.Minute), []int64{5})

	time.Sleep(time.Until(due))
	w := s.waitWindow("^Reminders$")
	s.waitNode(w, "", "Remind one")
	s.waitNode(w, "", "Remind two")
	// Snooze's menu is a native popup, which automation cannot choose from:
	// TestRemindersRaiseListSnoozeDismissAndClose covers it.
	dismiss := regexp.MustCompile("^Dismiss$")
	for left := 1; left >= 0; left-- {
		// Press only when the window shows the reminders maild still has: a
		// Dismiss pressed on a row that is on its way out answers nothing,
		// and the wait below then never ends (frostyard/frostmail#317).
		tree := s.waitFor(w, fmt.Sprintf("%d reminders to dismiss", left+1), func(tree []node) bool {
			n := 0
			for _, x := range tree {
				if x.Role == "button" && dismiss.MatchString(x.Label) {
					n++
				}
			}
			return n == left+1
		})
		s.press(w, *find(tree, "button", "^Dismiss$"))
		eventually(t, "a reminder answered", func() bool {
			list, err := s.client.Calendar().Reminders(ctx, &api.CalendarRemindersParams{})
			return err == nil && len(list) == left
		})
	}
	eventually(t, "the window closed", func() bool {
		for _, x := range s.windows() {
			if x.ID == w {
				return false
			}
		}
		return true
	})
}
