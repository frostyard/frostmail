package pimsync_test

import (
	"encoding/json/v2"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/store"
)

// planning is an event as Google writes it, with a line Frostmail does not
// know.
func planning(summary, description string) []byte {
	return ics("BEGIN:VEVENT", "UID:planning@google.com", "DTSTAMP:20261001T000000Z", "SEQUENCE:0",
		"DTSTART;TZID=America/New_York:20261009T090000", "DTEND;TZID=America/New_York:20261009T100000",
		"SUMMARY:"+summary, "DESCRIPTION:"+description, "X-GOOGLE-CONFERENCE:https://meet.google.com/abc",
		"END:VEVENT")
}

// weekly is a series of four Thursdays at 09:00 in New York from October
// 15.
func weekly() []byte {
	return ics("BEGIN:VEVENT", "UID:sync@google.com", "DTSTAMP:20261001T000000Z", "SEQUENCE:0",
		"DTSTART;TZID=America/New_York:20261015T090000", "DTEND;TZID=America/New_York:20261015T093000",
		"RRULE:FREQ=WEEKLY;COUNT=4", "SUMMARY:Sync", "END:VEVENT")
}

type eventEnv struct {
	*env
	work, home string
	cal        api.CalendarService
}

func newEventEnv(t *testing.T) *eventEnv {
	now := utcAt("2026-10-08T12:00Z")
	e := newCalendarEnv(t, &now)
	work := e.dav.Calendar("work", "Work", "#3366cc", "VEVENT")
	home := e.dav.Calendar("home", "Home", "#34c759", "VEVENT")
	e.dav.Put(work+"planning.ics", planning("Planning", "Agenda in the doc"))
	e.dav.Put(work+"sync.ics", weekly())
	e.pass()
	return &eventEnv{env: e, work: work, home: home, cal: engine.New(engine.Deps{DB: e.db, PIM: e.m}).Calendar()}
}

func (e *eventEnv) server(path string) string {
	e.t.Helper()
	data, _, ok := e.dav.Object(path)
	if !ok {
		e.t.Fatalf("%s is not on the server", path)
	}
	return string(data)
}

// event is the ID of the event (single or master) with the summary.
func (e *eventEnv) event(summary string) int64 {
	e.t.Helper()
	rows, err := e.db.Events(e.t.Context(), store.EventsFilter{})
	if err != nil {
		e.t.Fatal(err)
	}
	for _, r := range rows {
		if r.Summary == summary && r.RecurrenceID == "" {
			return r.ID
		}
	}
	e.t.Fatalf("no event %q", summary)
	return 0
}

func (e *eventEnv) calendarID(path string) int64 {
	e.t.Helper()
	cols, err := e.db.Collections(e.t.Context(), store.CollectionFilter{AccountID: e.acct.ID, Kind: api.CollectionKindCalendar})
	if err != nil {
		e.t.Fatal(err)
	}
	for _, c := range cols {
		if strings.HasSuffix(c.Href, path) {
			return c.ID
		}
	}
	e.t.Fatalf("no calendar %s in %+v", path, cols)
	return 0
}

func (e *eventEnv) refused() []api.CalendarRefused {
	var out []api.CalendarRefused
	for _, ev := range e.events {
		if ev.Event != "calendar.refused" {
			continue
		}
		var r api.CalendarRefused
		if err := json.Unmarshal(ev.Data, &r); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func hasLines(t *testing.T, got string, lines ...string) {
	t.Helper()
	for _, l := range lines {
		if !strings.Contains(got, l+"\r\n") {
			t.Errorf("no %q in:\n%s", l, got)
		}
	}
}

func scope(s api.EventScope) *api.EventScope { return &s }

func TestEventCreateReachesTheServer(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	start := utcAt("2026-10-10T14:00Z")
	w, err := e.cal.Create(ctx, &api.CalendarCreateParams{Fields: api.EventFields{Summary: ptr2("Review"),
		AllDay: ptr2(false), Start: &start, End: ptr2(start.Add(time.Hour)), TimeZone: ptr2("America/New_York"),
		Alarms: []int64{15}}})
	if err != nil || w.Event == nil || w.Event.Summary != "Review" || w.RestoreID == "" || len(w.Event.Alarms) != 1 {
		t.Fatalf("create = %+v, %v", w, err)
	}
	// The views have it before the server does.
	if got := e.occurrences("2026-10-10", "2026-10-11"); !slices.Equal(got, []string{"10-10T14:00 Review"}) {
		t.Errorf("occurrences = %q", got)
	}
	e.pass()
	obj, err := e.db.GetObject(ctx, mustObject(t, e, w.Event.ID))
	if err != nil {
		t.Fatal(err)
	}
	got := e.server(obj.Href)
	hasLines(t, got, "DTSTART;TZID=America/New_York:20261010T100000", "SUMMARY:Review", "TRIGGER:-PT15M",
		"TZID:America/New_York")
	if _, etag, _ := e.dav.Object(obj.Href); obj.ETag != etag {
		t.Errorf("local etag %q, server %q", obj.ETag, etag)
	}
}

func mustObject(t *testing.T, e *eventEnv, eventID int64) int64 {
	t.Helper()
	row, err := e.db.Event(t.Context(), eventID)
	if err != nil {
		t.Fatal(err)
	}
	return row.ObjectID
}

func TestEventEditKeepsWhatItDoesNotTouch(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	id := e.event("Planning")
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, Fields: api.EventFields{Summary: ptr2("Planning Q4")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, Fields: api.EventFields{Location: ptr2("Room 4")}}); err != nil {
		t.Fatal(err)
	}
	if ops, _ := e.db.DuePIMOps(ctx, e.acct.ID, e.db.Now()); len(ops) != 1 {
		t.Errorf("queued %d ops; want one put", len(ops))
	}
	e.pass()
	hasLines(t, e.server(e.work+"planning.ics"), "SUMMARY:Planning Q4", "LOCATION:Room 4",
		"X-GOOGLE-CONFERENCE:https://meet.google.com/abc", "DESCRIPTION:Agenda in the doc", "UID:planning@google.com")
}

// Another client's change to another field: the edit is applied again.
func TestEventEditAppliedAgainAfterAnotherClient(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Planning"),
		Fields: api.EventFields{Summary: ptr2("Planning Q4")}}); err != nil {
		t.Fatal(err)
	}
	e.dav.Put(e.work+"planning.ics", planning("Planning", "Agenda moved to the wiki"))
	e.pass()
	got := e.server(e.work + "planning.ics")
	hasLines(t, got, "SUMMARY:Planning Q4", "DESCRIPTION:Agenda moved to the wiki")
	if r := e.refused(); len(r) != 0 {
		t.Errorf("refused = %+v", r)
	}
}

// Another client's change to the same field: the server's version stands.
func TestEventEditRefusedWhenTheFieldChanged(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	id := e.event("Planning")
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, Fields: api.EventFields{Summary: ptr2("Planning Q4")}}); err != nil {
		t.Fatal(err)
	}
	e.dav.Put(e.work+"planning.ics", planning("Roadmap", "Agenda in the doc"))
	e.pass()
	hasLines(t, e.server(e.work+"planning.ics"), "SUMMARY:Roadmap")
	r := e.refused()
	if len(r) != 1 || r[0].Reason != "changed" || r[0].EventID == nil || r[0].Summary != "Planning Q4" {
		t.Errorf("refused = %+v", r)
	}
	if got, err := e.cal.Event(ctx, &api.CalendarEventParams{ID: id}); err != nil || got.Summary != "Roadmap" {
		t.Errorf("local event = %+v, %v", got, err)
	}
}

func TestSeriesEditsInEachScope(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	id := e.event("Sync")
	oct22, oct29 := "2026-10-22T13:00:00.000Z", "2026-10-29T13:00:00.000Z"
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, Fields: api.EventFields{Summary: ptr2("x")}}); code(err) != api.CodeInvalidParams {
		t.Errorf("a series edit without a scope = %v", err)
	}
	w, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, RecurrenceID: &oct22, Scope: scope(api.EventScopeThis),
		Fields: api.EventFields{Location: ptr2("Room 2")}})
	if err != nil || w.Event == nil || w.Event.Location != "Room 2" || w.Event.RecurrenceID != oct22 {
		t.Fatalf("this = %+v, %v", w, err)
	}
	e.pass()
	hasLines(t, e.server(e.work+"sync.ics"), "RECURRENCE-ID;TZID=America/New_York:20261022T090000", "LOCATION:Room 2")

	w, err = e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, RecurrenceID: &oct29, Scope: scope(api.EventScopeFuture),
		Fields: api.EventFields{Summary: ptr2("Sync v2")}})
	if err != nil || w.Event == nil || w.Event.Summary != "Sync v2" || w.Event.UID == "sync@google.com" {
		t.Fatalf("future = %+v, %v", w, err)
	}
	want := []string{"10-15T13:00 Sync", "10-22T13:00 Sync", "10-29T13:00 Sync v2", "11-05T14:00 Sync v2"}
	if got := e.occurrences("2026-10-12", "2026-11-10"); !slices.Equal(got, want) {
		t.Errorf("occurrences after the split = %q", got)
	}
	e.pass()
	hasLines(t, e.server(e.work+"sync.ics"), "RRULE:FREQ=WEEKLY;UNTIL=20261029T125959Z")
	if paths := e.dav.Paths(e.work); len(paths) != 3 {
		t.Errorf("the calendar has %v; want planning, sync and the new series", paths)
	}

	if _, err := e.cal.Delete(ctx, &api.CalendarDeleteParams{ID: id, RecurrenceID: ptr2("2026-10-15T13:00:00.000Z"),
		Scope: scope(api.EventScopeThis)}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	hasLines(t, e.server(e.work+"sync.ics"), "EXDATE;TZID=America/New_York:20261015T090000")
	if got := e.occurrences("2026-10-12", "2026-11-10"); len(got) != 3 {
		t.Errorf("occurrences after the delete = %q", got)
	}
}

func TestEventMovesToAnotherCalendar(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	home := e.calendarID(e.home)
	w, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Planning"), Fields: api.EventFields{CalendarID: &home}})
	if err != nil || w.Event == nil || w.Event.CalendarID != home {
		t.Fatalf("move = %+v, %v", w, err)
	}
	e.pass()
	if _, _, ok := e.dav.Object(e.work + "planning.ics"); ok {
		t.Error("the event is still in Work")
	}
	hasLines(t, e.server(e.home+"planning.ics"), "UID:planning@google.com", "X-GOOGLE-CONFERENCE:https://meet.google.com/abc")
	if err := e.cal.Restore(ctx, &api.CalendarRestoreParams{RestoreID: w.RestoreID}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	if got := append(e.dav.Paths(e.work), e.dav.Paths(e.home)...); len(got) != 2 || slices.Contains(got, e.home+"planning.ics") {
		t.Errorf("after undoing the move the server has %v", got)
	}
}

// A move the other calendar refuses leaves the event where it was.
func TestEventMoveRefused(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	home := e.calendarID(e.home)
	e.dav.FailWith(func(r *http.Request) int {
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, e.home) {
			return http.StatusForbidden
		}
		return 0
	})
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Planning"), Fields: api.EventFields{CalendarID: &home}}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	e.dav.FailWith(nil)
	if r := e.refused(); len(r) == 0 || r[len(r)-1].Reason != "refused" {
		t.Errorf("refused = %+v", r)
	}
	e.server(e.work + "planning.ics")
	e.pass()
	e.event("Planning")
}

func TestEventDeleteAndRestore(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	before := e.server(e.work + "planning.ics")
	w, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Planning"), Fields: api.EventFields{Summary: ptr2("x")}})
	if err != nil {
		t.Fatal(err)
	}
	e.pass()
	if err := e.cal.Restore(ctx, &api.CalendarRestoreParams{RestoreID: w.RestoreID}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	if got := e.server(e.work + "planning.ics"); got != before {
		t.Errorf("after the restore the server has %s; want %s", got, before)
	}

	gone, err := e.cal.Delete(ctx, &api.CalendarDeleteParams{ID: e.event("Planning")})
	if err != nil || gone.Event != nil {
		t.Fatalf("delete = %+v, %v", gone, err)
	}
	if got := e.occurrences("2026-10-09", "2026-10-10"); len(got) != 0 {
		t.Errorf("the deleted event still shows: %q", got)
	}
	e.pass()
	if _, _, ok := e.dav.Object(e.work + "planning.ics"); ok {
		t.Error("the deleted event is still on the server")
	}
	if err := e.cal.Restore(ctx, &api.CalendarRestoreParams{RestoreID: gone.RestoreID}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	e.event("Planning")
	if paths := e.dav.Paths(e.work); len(paths) != 2 || slices.Contains(paths, e.work+"planning.ics") {
		t.Errorf("after restoring the delete the server has %v", paths)
	}

	made, _ := e.cal.Create(ctx, &api.CalendarCreateParams{Fields: api.EventFields{AllDay: ptr2(true),
		StartDate: ptr2("2026-10-12"), EndDate: ptr2("2026-10-13"), Summary: ptr2("Holiday")}})
	if err := e.cal.Restore(ctx, &api.CalendarRestoreParams{RestoreID: made.RestoreID}); err != nil {
		t.Fatal(err)
	}
	e.pass()
	if slices.ContainsFunc(e.dav.Requests(), func(r davtest.Request) bool { return r.Method == "PUT" }) {
		t.Error("an undone creation was written")
	}
}

func TestEventWritesRefuseWhatCannotBeWritten(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	e.dav.Put(e.work+"lunch.ics", ics("BEGIN:VEVENT", "UID:lunch", "DTSTAMP:20261001T000000Z",
		"DTSTART:20261009T160000Z", "DTEND:20261009T170000Z", "SUMMARY:Lunch",
		"ORGANIZER:mailto:bob@example.com", "ATTENDEE;PARTSTAT=ACCEPTED:mailto:user@example.com", "END:VEVENT"))
	e.pass()
	lunch := e.event("Lunch")
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: lunch, Fields: api.EventFields{Summary: ptr2("x")}}); code(err) != api.CodeInvalidParams {
		t.Errorf("renaming an invitation = %v", err)
	}
	if w, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: lunch, Fields: api.EventFields{Alarms: []int64{30}}}); err != nil ||
		len(w.Event.Alarms) != 1 || w.Event.Alarms[0] != 30 {
		t.Errorf("an invitation's alerts = %+v, %v", w, err)
	}
	start := utcAt("2026-10-10T14:00Z")
	for name, f := range map[string]api.EventFields{
		"no times":     {Summary: ptr2("x")},
		"end first":    {AllDay: ptr2(false), Start: &start, End: ptr2(start.Add(-time.Hour)), TimeZone: ptr2("UTC")},
		"unknown zone": {AllDay: ptr2(false), Start: &start, End: ptr2(start.Add(time.Hour)), TimeZone: ptr2("Mars/Olympus")},
		"a bad rule":   {AllDay: ptr2(true), StartDate: ptr2("2026-10-12"), EndDate: ptr2("2026-10-13"), Rrule: ptr2("FREQ=SOMETIMES")},
	} {
		if _, err := e.cal.Create(ctx, &api.CalendarCreateParams{Fields: f}); code(err) != api.CodeInvalidParams {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: 999, Fields: api.EventFields{Summary: ptr2("x")}}); code(err) != api.CodeNotFound {
		t.Errorf("an unknown event = %v", err)
	}
	e.dav.SetReadOnly(e.work, true)
	e.pass()
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Planning"), Fields: api.EventFields{Summary: ptr2("x")}}); code(err) != api.CodeConflict {
		t.Errorf("an edit in a read-only calendar = %v", err)
	}
}
