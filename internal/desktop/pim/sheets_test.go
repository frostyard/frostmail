package pim

import (
	"context"
	"encoding/json/jsontext"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// fakeCalendar asks for notify on writes, and holds reminders.
type fakeCalendar struct {
	api.CalendarService
	mu        sync.Mutex
	notify    bool
	notified  []bool
	due       []api.Reminder
	snoozed   []string
	dismissed []string
	zones     []string // each Range's zone, "" for none
	snoozeErr error    // the next Snooze fails with it
	hold      chan struct{}
	// The next Reminders answers with the list it read, once remindHold
	// closes; it closes remindHeld when it holds.
	remindHold chan struct{}
	remindHeld chan struct{}
}

func (s *fakeCalendar) ask(n *bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notified = append(s.notified, n != nil && *n)
	if s.notify && n == nil {
		return &api.Error{Code: api.CodeNotifyRequired, Message: "tells invitees", Data: jsontext.Value(`{"kind":"updates","count":2}`)}
	}
	return nil
}

func (s *fakeCalendar) Update(ctx context.Context, p *api.CalendarUpdateParams) (*api.EventWrite, error) {
	if err := s.ask(p.Notify); err != nil {
		return nil, err
	}
	return s.CalendarService.Update(ctx, p)
}

func (s *fakeCalendar) Restore(ctx context.Context, p *api.CalendarRestoreParams) error {
	if err := s.ask(p.Notify); err != nil {
		return err
	}
	return s.CalendarService.Restore(ctx, p)
}

func (s *fakeCalendar) Reminders(context.Context, *api.CalendarRemindersParams) ([]api.Reminder, error) {
	s.mu.Lock()
	due := append([]api.Reminder{}, s.due...)
	hold, held := s.remindHold, s.remindHeld
	s.remindHold, s.remindHeld = nil, nil
	s.mu.Unlock()
	if hold != nil {
		close(held)
		<-hold
	}
	return due, nil
}

func (s *fakeCalendar) Range(ctx context.Context, p *api.CalendarRangeParams) ([]api.Occurrence, error) {
	zone := ""
	if p.TimeZone != nil {
		zone = *p.TimeZone
	}
	s.mu.Lock()
	s.zones = append(s.zones, zone)
	s.mu.Unlock()
	return s.CalendarService.Range(ctx, p)
}

func (s *fakeCalendar) Snooze(_ context.Context, p *api.CalendarSnoozeParams) error {
	s.mu.Lock()
	hold := s.hold
	s.mu.Unlock()
	if hold != nil {
		<-hold
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.snoozeErr; err != nil {
		s.snoozeErr = nil
		return err
	}
	s.snoozed = append(s.snoozed, p.IDs...)
	s.drop(p.IDs)
	return nil
}

func (s *fakeCalendar) Dismiss(_ context.Context, p *api.CalendarDismissParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dismissed = append(s.dismissed, p.IDs...)
	s.drop(p.IDs)
	return nil
}

func (s *fakeCalendar) drop(ids []string) {
	var keep []api.Reminder
	for _, r := range s.due {
		gone := false
		for _, id := range ids {
			gone = gone || id == r.ID
		}
		if !gone {
			keep = append(keep, r)
		}
	}
	s.due = keep
}

func withFake(fake *fakeCalendar) options {
	return options{Options: rpctest.Options{Services: func(s *api.Services) {
		fake.CalendarService = s.Calendar
		s.Calendar = fake
	}}}
}

func TestADropThatTellsInviteesAsksFirst(t *testing.T) {
	fake := &fakeCalendar{notify: true}
	f := newFixture(t, commands.Calendar, withFake(fake))
	f.text("Review")
	r := f.block("Review, ")
	f.drag(r.X+r.W/2, r.Y+10, 0, 24)
	f.until("the sheet", func() bool { return f.shell.Dialogs.Showing() && f.tt.HasText("Send Updates?") })
	if !f.tt.HasText("2 invitees will get it by email.") {
		t.Errorf("shows %s", dump(f.tt))
	}
	// Cancel writes nothing.
	f.click("Cancel")
	f.until("closed", func() bool { return !f.shell.Dialogs.Showing() })
	if len(f.m.cal.undo) != 0 {
		t.Error("cancel wrote")
	}
	r = f.block("Review, ")
	f.drag(r.X+r.W/2, r.Y+10, 0, 24)
	f.until("the sheet", func() bool { return f.shell.Dialogs.Showing() })
	f.click("Send")
	f.until("the write", func() bool { return len(f.m.cal.undo) == 1 })
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.notified) != 3 || !fake.notified[2] {
		t.Errorf("notified %v", fake.notified)
	}
}

func TestAnUndoThatTellsInviteesAsksAndCancelKeepsTheEntry(t *testing.T) {
	fake := &fakeCalendar{}
	f := newFixture(t, commands.Calendar, withFake(fake))
	f.text("Review")
	r := f.block("Review, ")
	f.drag(r.X+r.W/2, r.Y+10, 0, 24)
	f.until("the write", func() bool { return len(f.m.cal.undo) == 1 })
	fake.mu.Lock()
	fake.notify = true
	fake.mu.Unlock()
	f.tt.Key(ui.Cmd, ui.KeyZ)
	f.until("the sheet", func() bool { return f.shell.Dialogs.Showing() })
	f.click("Cancel")
	f.until("the entry back", func() bool { return len(f.m.cal.undo) == 1 })
	f.tt.Key(ui.Cmd, ui.KeyZ)
	f.until("the sheet", func() bool { return f.shell.Dialogs.Showing() })
	f.click("Send")
	f.until("restored", func() bool {
		o := f.occurrence("Review", "2026-10-08", "2026-10-09")
		return len(f.m.cal.undo) == 0 && o.Start.Equal(time.Date(2026, 10, 8, 14, 0, 0, 0, time.UTC))
	})
}

func TestEventWindowNotifySheetKeepsTheDraftOnCancel(t *testing.T) {
	fake := &fakeCalendar{notify: true}
	f := newFixture(t, commands.Calendar, withFake(fake))
	f.text("Review")
	f.click("Review, 2:00 – 3:30 PM")
	f.until("selection", func() bool { return f.m.shownEvent() != nil })
	f.m.editSelected()
	f.until("the window", func() bool { w := f.window("event-"); return w != nil && f.m.windows[w.spec.Key].loaded })
	win := f.window("event-")
	w := f.m.windows[win.spec.Key]
	w.draft.summary = "New Name"
	win.tt.Frame()
	f.clickIn(win, "Save")
	f.until("the sheet", func() bool { return win.tt.HasText("Send Updates?") })
	win.tt.Key(0, ui.KeyEscape)
	f.until("closed", func() bool { return !win.tt.HasText("Send Updates?") })
	if f.window("event-") == nil || w.draft.summary != "New Name" {
		t.Error("Cancel closed the editor or lost the draft")
	}
	f.clickIn(win, "Save")
	f.until("the sheet", func() bool { return win.tt.HasText("Send Updates?") })
	f.clickIn(win, "Send")
	f.until("closed", func() bool { return f.window("event-") == nil })
}

func TestRepeatSheetWritesTheRule(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	win, w := f.newEvent()
	w.repeat = &repeatSheet{r: defaultRepeat(w.draft.startDate), interval: "2"}
	win.tt.Frame()
	if !win.tt.HasText("Custom Repeat") {
		t.Fatalf("no sheet: %s", dump(win.tt))
	}
	f.clickIn(win, "Monday")
	f.clickIn(win, "OK")
	if w.repeat != nil || w.draft.rrule != "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,TH" {
		t.Errorf("rule %q", w.draft.rrule)
	}
	if !win.tt.HasText("End repeat") {
		t.Errorf("no End repeat row: %s", dump(win.tt))
	}
}

func TestPresetsAndAlertsChange(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	win, w := f.newEvent()
	win.tt.Frame()
	f.clickIn(win, "add alert")
	if len(w.draft.alarms) != 1 || w.draft.alarms[0] != 0 {
		t.Errorf("alarms %v", w.draft.alarms)
	}
	w.draft.allDay = true
	win.tt.Frame()
	if win.tt.HasText("Time zone") {
		t.Error("an all-day event shows a time zone")
	}
	f.clickIn(win, "add alert")
	if len(w.draft.alarms) != 2 || w.draft.alarms[1] != -540 {
		t.Errorf("alarms %v", w.draft.alarms)
	}
}

func TestInviteesInACalendarThatDoesNotInvite(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	win, w := f.newEvent()
	w.calendars[0].invites = false
	win.tt.Frame()
	if !win.tt.HasText("Invitations aren't available in this calendar.") {
		t.Errorf("shows %s", dump(win.tt))
	}
	if win.tt.HasText("Add Invitee") {
		t.Error("the input shows")
	}
	w.draft.invitees = []draftInvitee{{email: "ann@a.test"}}
	if !w.cannotSave() {
		t.Error("Save would write invitees to a calendar that does not invite")
	}
}

func TestInviteesEditing(t *testing.T) {
	f := newFixture(t, commands.Calendar)
	f.text("Standup")
	win, w := f.newEvent()
	win.tt.Frame()
	if err := win.tt.Click("Add Invitee"); err != nil {
		t.Fatalf("%v: %s", err, dump(win.tt))
	}
	win.tt.Type("Ann <ann@a.test>")
	win.tt.Key(0, ui.KeyEnter)
	win.tt.Frame()
	if len(w.draft.invitees) != 1 || w.draft.invitees[0].email != "ann@a.test" || w.draft.invitees[0].name != "Ann" {
		t.Fatalf("%+v", w.draft.invitees)
	}
	win.tt.Type("ann@a.test,")
	win.tt.Frame()
	if len(w.draft.invitees) != 1 {
		t.Errorf("an address already invited was added: %+v", w.draft.invitees)
	}
	f.clickIn(win, "Optional")
	if w.draft.invitees[0].role != api.AttendeeRoleOptional {
		t.Error("Optional did not toggle")
	}
	f.clickIn(win, "Add Invitee")
	win.tt.Type("broken,")
	win.tt.Frame()
	if !win.tt.HasText("Check the invitees' addresses") || !w.cannotSave() {
		t.Errorf("shows %s", dump(win.tt))
	}
	f.clickIn(win, "Remove broken")
	f.clickIn(win, "Remove Ann")
	if len(w.draft.invitees) != 0 {
		t.Errorf("%+v", w.draft.invitees)
	}
}

func TestRemindersRaiseListSnoozeDismissAndClose(t *testing.T) {
	due := func(id string, minutes int) api.Reminder {
		return api.Reminder{ID: id, EventID: 1, Summary: "Meeting " + id, Location: "Room", Start: testNow.Add(time.Duration(minutes) * time.Minute), DueAt: testNow}
	}
	fake := &fakeCalendar{due: []api.Reminder{due("a", 10), due("b", 0)}}
	f := newFixture(t, commands.Calendar, withFake(fake))
	f.until("the window", func() bool { return f.windows[remindersKey] != nil })
	win := f.windows[remindersKey]
	if win.spec.Width != 380 || win.spec.Height != 300 || !win.spec.AlwaysOnTop {
		t.Errorf("spec %+v", win.spec)
	}
	f.until("the rows", func() bool {
		return win.tt.HasText("Meeting a") && win.tt.HasText("In 10 minutes · Room") && win.tt.HasText("Now · Room")
	})
	if !win.tt.HasText("Dismiss All") {
		t.Error("no Dismiss All")
	}
	if err := win.tt.Click("Snooze"); err != nil {
		t.Fatal(err)
	}
	win.tt.Frame()
	if err := win.tt.ChooseMenuItem("5 minutes"); err != nil {
		t.Fatalf("%v: menu %q", err, win.tt.Menu())
	}
	f.until("the row leaving", func() bool { return !win.tt.HasText("Meeting a") })
	f.until("the snooze reaching maild", func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return len(fake.snoozed) == 1 && fake.snoozed[0] == "a"
	})
	f.until("no Dismiss All", func() bool { return !win.tt.HasText("Dismiss All") })
	// Opening keeps the reminder and shows the occurrence.
	f.clickIn(win, "Meeting b")
	if f.m.cal.selected == nil || f.m.cal.selected.eventID != 1 {
		t.Error("no selection")
	}
	// Dismissing the last closes the window.
	f.clickIn(win, "Dismiss")
	f.until("the close", func() bool { return f.windows[remindersKey] == nil })
	// A new reminder raises it again.
	fake.mu.Lock()
	fake.due = []api.Reminder{due("c", 30)}
	fake.mu.Unlock()
	f.m.event(api.CalendarReminders{Count: 1})
	f.until("the raised window", func() bool { w := f.windows[remindersKey]; return w != nil && w.tt.HasText("Meeting c") })
}

// #317: a calendar.reminders call asked before a Dismiss answers after
// it, with the dismissed row. That answer must not bring the row back: a
// second Dismiss would answer the same reminder again.
func TestARemindersAnswerOlderThanADismissIsDropped(t *testing.T) {
	due := func(id string) api.Reminder {
		return api.Reminder{ID: id, EventID: 1, Summary: "Meeting " + id, Start: testNow, DueAt: testNow}
	}
	fake := &fakeCalendar{due: []api.Reminder{due("a"), due("b")}}
	f := newFixture(t, commands.Calendar, withFake(fake))
	f.until("the rows", func() bool {
		w := f.windows[remindersKey]
		return w != nil && w.tt.HasText("Meeting a") && w.tt.HasText("Meeting b") && !f.m.remind.list.running
	})
	win := f.windows[remindersKey]
	hold, held := make(chan struct{}), make(chan struct{})
	fake.mu.Lock()
	fake.remindHold, fake.remindHeld = hold, held
	fake.mu.Unlock()
	f.m.remindersChanged()
	f.until("the held call", func() bool {
		select {
		case <-held:
			return true
		default:
			return false
		}
	})
	f.clickIn(win, "Dismiss")
	f.until("the dismiss answered", func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return len(fake.dismissed) == 1 && len(f.m.remind.pending) == 0
	})
	if win.tt.HasText("Meeting a") || !win.tt.HasText("Meeting b") {
		t.Fatalf("after the dismiss: %q", win.tt.Texts())
	}
	// The call after the old answer is held too: its answer would replace
	// a row the old one brought back before a frame of the window shows it.
	next, nextHeld := make(chan struct{}), make(chan struct{})
	fake.mu.Lock()
	fake.remindHold, fake.remindHeld = next, nextHeld
	fake.mu.Unlock()
	close(hold)
	f.until("the next call", func() bool {
		select {
		case <-nextHeld:
			return true
		default:
			return false
		}
	})
	// The old answer has landed, and the next call cannot answer yet.
	win.tt.Frame()
	if win.tt.HasText("Meeting a") || !win.tt.HasText("Meeting b") {
		t.Errorf("the old answer brought the dismissed row back: %q", win.tt.Texts())
	}
	close(next)
	f.until("the new answer", func() bool {
		l := &f.m.remind.list
		return !l.running && !l.dirty && !l.pending
	})
	if win.tt.HasText("Meeting a") || !win.tt.HasText("Meeting b") {
		t.Errorf("after the new answer: %q", win.tt.Texts())
	}
}

func TestEscapeClosesRemindersKeepingThem(t *testing.T) {
	fake := &fakeCalendar{due: []api.Reminder{{ID: "a", Summary: "Meeting", Start: testNow}}}
	f := newFixture(t, commands.Calendar, withFake(fake))
	f.until("the row", func() bool { w := f.windows[remindersKey]; return w != nil && w.tt.HasText("Meeting") })
	win := f.windows[remindersKey]
	win.tt.Key(0, ui.KeyEscape)
	win.tt.Frame()
	if f.windows[remindersKey] != nil {
		t.Error("Escape left the window")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.due) != 1 {
		t.Error("closing dismissed the reminder")
	}
}
