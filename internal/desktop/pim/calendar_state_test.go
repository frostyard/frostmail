package pim

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestMiniMonthLoadsTheBrowsedMonth(t *testing.T) {
	for _, module := range []commands.Module{commands.Calendar, commands.Mail} {
		t.Run(string(module), func(t *testing.T) {
			f := newFixture(t, module)
			f.until("the calendars", func() bool { return len(f.m.cal.collections.value) > 0 })
			_, err := f.c.Calendar().Create(t.Context(), &api.CalendarCreateParams{CalendarID: &f.work,
				Fields: api.EventFields{Summary: ptr("November"), AllDay: ptr(true), StartDate: ptr("2026-11-19"), EndDate: ptr("2026-11-20")}})
			if err != nil {
				t.Fatal(err)
			}
			f.click("Next Month")
			f.text("November 2026")
			f.until("the browsed month's busy date", func() bool {
				if module == commands.Mail {
					return f.m.todo.busy["2026-11-19"]
				}
				return f.m.cal.busy["2026-11-19"]
			})
			if f.m.cal.date != "2026-10-08" {
				t.Errorf("browsing selected %s", f.m.cal.date)
			}
		})
	}
}

func TestNewEventStartsAtTheNextLocalWholeHour(t *testing.T) {
	for _, tc := range []struct{ zone, now, want string }{
		{"Asia/Kolkata", "2026-10-08T10:35:12+05:30", "2026-10-08T11:00:00+05:30"},
		{"Asia/Kathmandu", "2026-10-08T23:50:12+05:45", "2026-10-09T00:00:00+05:45"},
		{"America/St_Johns", "2026-10-08T10:00:00-02:30", "2026-10-08T11:00:00-02:30"},
		{"America/New_York", "2026-03-08T01:45:00-05:00", "2026-03-08T03:00:00-04:00"},
		{"Australia/Lord_Howe", "2026-10-04T01:45:00+10:30", "2026-10-04T03:00:00+11:00"},
	} {
		t.Run(tc.zone, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			now, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			f := newFixture(t, commands.Calendar)
			f.now = now.In(loc)
			f.m.cal.date = f.now.Format(time.DateOnly)
			_, w := f.newEvent()
			if got := w.target.times.start.Format(time.RFC3339); got != tc.want {
				t.Errorf("start = %s, want %s", got, tc.want)
			}
			if w.target.times.end.Sub(w.target.times.start) != time.Hour {
				t.Error("event is not one hour long")
			}
		})
	}
}

// A real RPC stays in flight across Leave. Only its selected method is
// held; every UI callback is delivered explicitly by the test.
type heldCalendarWrite struct {
	api.CalendarService
	method  string
	entered chan struct{}
	release chan struct{}
	event   api.CalendarEvent
}

func (s *heldCalendarWrite) wait(method string) {
	if s.method == method {
		close(s.entered)
		<-s.release
	}
}
func (s *heldCalendarWrite) Delete(context.Context, *api.CalendarDeleteParams) (*api.EventWrite, error) {
	s.wait("delete")
	return &api.EventWrite{RestoreID: "old"}, nil
}
func (s *heldCalendarWrite) Update(context.Context, *api.CalendarUpdateParams) (*api.EventWrite, error) {
	s.wait("update")
	return &api.EventWrite{RestoreID: "old"}, nil
}
func (s *heldCalendarWrite) Event(context.Context, *api.CalendarEventParams) (*api.CalendarEvent, error) {
	s.wait("event")
	return &s.event, nil
}

func TestCalendarWorkCannotReachTheNextMaild(t *testing.T) {
	for _, action := range []string{"delete", "drag write", "drag read"} {
		t.Run(action, func(t *testing.T) {
			method := map[string]string{"delete": "delete", "drag write": "update", "drag read": "event"}[action]
			service := &heldCalendarWrite{method: method, entered: make(chan struct{}), release: make(chan struct{}),
				event: api.CalendarEvent{ID: 2, CalendarID: 1, TimeZone: "UTC"}}
			release := sync.OnceFunc(func() { close(service.release) })
			defer release()
			if action == "drag read" {
				service.event.Recurring = true
				service.event.RecurrenceID = "2026-10-08T10:00:00Z"
			}
			srv := rpctest.StartWith(t, rpctest.Options{Services: func(s *api.Services) { s.Calendar = service }})
			posted := make(chan func(), 4)
			m := New(Options{Post: func(fn func()) { posted <- fn }})
			m.client = srv.Dial(t)
			m.cal.collections.value = []api.Collection{{ID: 1}}
			m.cal.event.value = &service.event
			m.cal.selected = &occKey{eventID: 2}
			runPosted := func() {
				t.Helper()
				select {
				case fn := <-posted:
					fn()
				case <-time.After(5 * time.Second):
					t.Fatal("no UI callback")
				}
			}
			if action == "delete" {
				m.deleteSelected()
			} else {
				m.dropOccurrence(api.Occurrence{EventID: 2, CalendarID: 1}, func(string) api.EventFields { return api.EventFields{Summary: ptr("Moved")} })
				if action == "drag write" {
					runPosted()
				}
			}
			select {
			case <-service.entered:
			case <-time.After(5 * time.Second):
				t.Fatalf("%s did not start", action)
			}
			m.Leave()
			m.cal.undo = []string{"next"}
			release()
			runPosted()
			if !slices.Equal(m.cal.undo, []string{"next"}) {
				t.Errorf("the next maild's undo = %v", m.cal.undo)
			}
			if m.cal.scope != nil {
				t.Error("the old read opened a scope sheet on the next maild")
			}
		})
	}
}
