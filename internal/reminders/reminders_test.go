package reminders_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/notify"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/reminders"
	"github.com/frostyard/frostmail/internal/store"
)

// fakeNotifier records what the scheduler shows and withdraws.
type fakeNotifier struct {
	mu       sync.Mutex
	shown    []string // "summary | body"
	withdraw []string // the last active set, sorted
}

func (f *fakeNotifier) Remind(_ context.Context, rs []notify.Reminder) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range rs {
		f.shown = append(f.shown, r.Summary+" | "+r.Body)
	}
	return nil
}

func (f *fakeNotifier) Withdraw(_ context.Context, active map[string]bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.withdraw = f.withdraw[:0]
	for id := range active {
		f.withdraw = append(f.withdraw, id)
	}
	slices.Sort(f.withdraw)
	return nil
}

func (f *fakeNotifier) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.shown
	f.shown = nil
	return out
}

func ics(lines ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VCALENDAR\r\n")
}

func TestScheduler(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var events []string
	db.OnCommit = func(evs []api.EventEnvelope) {
		for _, e := range evs {
			events = append(events, e.Event)
		}
	}
	now := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	from, to := pimsync.Window(now)
	err = db.Tx(ctx, func(tx *store.Tx) error {
		server := store.ServerConfig{Host: "mail.example", Port: 993, TLS: api.TLSModeTLS, Username: "u"}
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "u@example.com",
			Auth: api.AuthKindPassword, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		if err := tx.SetService(ctx, acct.ID, api.ServiceKindCalendar, true, "https://dav.example/"); err != nil {
			return err
		}
		cols, err := tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindCalendar, []store.RemoteCollection{{Href: "/c/", Name: "Work"}})
		if err != nil {
			return err
		}
		alarm := func(trigger string) []string {
			return []string{"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER:" + trigger, "END:VALARM"}
		}
		objects := map[string][]byte{
			"standup": ics(append(append([]string{"BEGIN:VEVENT", "UID:standup", "SUMMARY:Standup", "LOCATION:Room 4",
				"DTSTART:20261008T093000Z", "DTEND:20261008T094500Z"}, alarm("-PT10M")...), "END:VEVENT")...),
			"lunch": ics(append(append([]string{"BEGIN:VEVENT", "UID:lunch", "SUMMARY:Lunch",
				"DTSTART:20261008T120000Z", "DTEND:20261008T130000Z"}, alarm("-PT15M")...), "END:VEVENT")...),
			"birthday": ics(append(append([]string{"BEGIN:VEVENT", "UID:birthday", "SUMMARY:Birthday",
				"DTSTART;VALUE=DATE:20261009", "DTEND;VALUE=DATE:20261010"}, alarm("-PT15H")...), "END:VEVENT")...),
		}
		for name, raw := range objects {
			o := store.Object{CollectionID: cols[0].ID, Href: "/c/" + name + ".ics", Raw: raw}
			if _, err := pimsync.StoreCalendarObject(ctx, tx, o, calendar.Options{Local: time.UTC}, from, to); err != nil {
				return err
			}
		}
		return tx.SetInstanceWindow(ctx, from, to)
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Now = func() time.Time { return now }
	cal := engine.New(engine.Deps{DB: db}).Calendar()
	list := func(ctx context.Context) ([]api.Reminder, error) {
		return cal.Reminders(ctx, &api.CalendarRemindersParams{})
	}
	attended := false
	notes := &fakeNotifier{}
	s := reminders.New(reminders.Config{DB: db, List: list, Attended: func() bool { return attended }, Notifier: notes,
		Now: func() time.Time { return now }, Local: time.UTC})
	check := func(at string) {
		t.Helper()
		var err error
		if now, err = time.Parse(time.RFC3339, at); err != nil {
			t.Fatal(err)
		}
		events = nil
		if err := s.Check(ctx); err != nil {
			t.Fatal(err)
		}
	}
	announced := func() bool { return slices.Contains(events, "calendar.reminders") }

	// The first check catches up: the birthday's alarm (15 hours before
	// midnight) is due now.
	check("2026-10-08T09:00:00Z")
	if got := notes.take(); !slices.Equal(got, []string{"Birthday | Tomorrow, All day"}) || !announced() {
		t.Errorf("first check: shown %q, events %q", got, events)
	}
	check("2026-10-08T09:20:00Z")
	if got := notes.take(); !slices.Equal(got, []string{"Standup | Today, 9:30 AM · Room 4"}) || !announced() {
		t.Errorf("at 9:20: shown %q, events %q", got, events)
	}
	check("2026-10-08T09:21:00Z")
	if got := notes.take(); len(got) != 0 || announced() {
		t.Errorf("nothing new: shown %q, events %q", got, events)
	}
	if len(notes.withdraw) != 2 {
		t.Errorf("active = %q", notes.withdraw)
	}

	// With the app attending, reminders go to it alone.
	attended = true
	check("2026-10-08T11:45:00Z")
	if got := notes.take(); len(got) != 0 || !announced() {
		t.Errorf("attended: shown %q, events %q", got, events)
	}
	all, _ := list(ctx)
	if len(all) != 3 {
		t.Fatalf("reminders = %+v", all)
	}
	var standup string
	for _, r := range all {
		if r.Summary == "Standup" {
			standup = r.ID
		}
	}

	// A snoozed reminder's note is withdrawn, and it reminds again when the
	// snooze ends.
	attended = false
	if err := cal.Snooze(ctx, &api.CalendarSnoozeParams{IDs: []string{standup}, Until: time.Date(2026, 10, 8, 11, 55, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	check("2026-10-08T11:50:00Z")
	if slices.Contains(notes.withdraw, standup) || len(notes.withdraw) != 2 {
		t.Errorf("active while snoozed = %q", notes.withdraw)
	}
	check("2026-10-08T11:55:00Z")
	if got := notes.take(); !slices.Equal(got, []string{"Standup | Today, 9:30 AM · Room 4"}) || !announced() {
		t.Errorf("snooze ended: shown %q, events %q", got, events)
	}
	if err := cal.Dismiss(ctx, &api.CalendarDismissParams{IDs: []string{standup}}); err != nil {
		t.Fatal(err)
	}
	check("2026-10-08T11:56:00Z")
	if slices.Contains(notes.withdraw, standup) || len(notes.take()) != 0 {
		t.Errorf("after dismissing: active %q", notes.withdraw)
	}
}
