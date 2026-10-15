package pimsync_test

import (
	"encoding/json/v2"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/store"
)

// asGmail makes the test account a Gmail one, whose calendars take
// invitees: Google's server tells them (ADR-0035).
func (e *eventEnv) asGmail() {
	e.t.Helper()
	ctx := e.t.Context()
	if err := e.db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE accounts SET kind = 'gmail' WHERE id = ?`, e.acct.ID)
		return err
	}); err != nil {
		e.t.Fatal(err)
	}
}

// notice is the InviteNotice of a notifyRequired error, or nil.
func notice(t *testing.T, err error) *api.InviteNotice {
	t.Helper()
	var ae *api.Error
	if !errors.As(err, &ae) || ae.Code != api.CodeNotifyRequired {
		return nil
	}
	var n api.InviteNotice
	if err := json.Unmarshal(ae.Data, &n); err != nil {
		t.Fatalf("notifyRequired's data %s: %v", ae.Data, err)
	}
	return &n
}

func wantNotice(t *testing.T, what string, err error, kind api.NoticeKind, count int64) {
	t.Helper()
	n := notice(t, err)
	if n == nil || n.Kind != kind || n.Count != count {
		t.Errorf("%s: %v, notice %+v; want %s to %d", what, err, n, kind, count)
	}
}

func invalid(err error) bool {
	var ae *api.Error
	return errors.As(err, &ae) && ae.Code == api.CodeInvalidParams
}

func invitees(list ...string) []api.Invitee {
	out := []api.Invitee{}
	for _, s := range list {
		in := api.Invitee{Email: strings.TrimSuffix(s, "?")}
		if strings.HasSuffix(s, "?") {
			in.Role = ptr2(api.AttendeeRoleOptional)
		}
		out = append(out, in)
	}
	return out
}

func yes() *bool { return ptr2(true) }

func TestInviteesOnlyWhereTheServerTellsThem(t *testing.T) {
	e := newEventEnv(t)
	ctx := t.Context()
	start := utcAt("2026-10-10T14:00Z")
	fields := api.EventFields{Summary: ptr2("Review"), AllDay: ptr2(false), Start: &start,
		End: ptr2(start.Add(time.Hour)), TimeZone: ptr2("UTC"), Attendees: invitees("carl@example.com")}
	if _, err := e.cal.Create(ctx, &api.CalendarCreateParams{Fields: fields, Notify: yes()}); !invalid(err) {
		t.Errorf("an IMAP account's calendar took invitees: %v", err)
	}
	cols, err := engine.New(engine.Deps{DB: e.db}).Accounts().Collections(ctx, &api.AccountCollectionsParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cols {
		if c.Invites {
			t.Errorf("%s invites on an IMAP account", c.Name)
		}
	}
	fields.Attendees = invitees()
	if _, err := e.cal.Create(ctx, &api.CalendarCreateParams{Fields: fields}); err != nil {
		t.Errorf("no invitees: %v", err)
	}
	e.asGmail()
	cols, err = engine.New(engine.Deps{DB: e.db}).Accounts().Collections(ctx, &api.AccountCollectionsParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cols {
		if !c.Invites {
			t.Errorf("%s does not invite on a Gmail account", c.Name)
		}
	}
}

func TestInvitingAsksFirst(t *testing.T) {
	e := newEventEnv(t)
	e.asGmail()
	ctx := t.Context()
	start := utcAt("2026-10-10T14:00Z")
	create := &api.CalendarCreateParams{CalendarID: ptr2(e.calendarID(e.work)), Fields: api.EventFields{Summary: ptr2("Review"), AllDay: ptr2(false),
		Start: &start, End: ptr2(start.Add(time.Hour)), TimeZone: ptr2("UTC"),
		Attendees: invitees("Carl@Example.com", "dana@example.com?", "user@example.com", "carl@example.com")}}

	_, err := e.cal.Create(ctx, create)
	wantNotice(t, "create", err, api.NoticeKindInvitations, 2)
	if rows, _ := e.db.Events(ctx, store.EventsFilter{}); len(rows) != 2 {
		t.Errorf("a refused create was stored: %d events", len(rows))
	}
	create.Notify = ptr2(false)
	if _, err := e.cal.Create(ctx, create); !invalid(err) {
		t.Errorf("Don't Send was taken: %v", err)
	}
	create.Notify = yes()
	w, err := e.cal.Create(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	if w.Event.Organizer == nil || w.Event.Organizer.Email != "user@example.com" || len(w.Event.Attendees) != 2 ||
		w.Event.Attendees[1].Role != api.AttendeeRoleOptional {
		t.Errorf("created = %+v / %+v", w.Event.Organizer, w.Event.Attendees)
	}
	e.pass()
	obj, err := e.db.GetObject(ctx, mustObject(t, e, w.Event.ID))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.ReplaceAll(e.server(obj.Href), "\r\n ", "")
	hasLines(t, got, "ORGANIZER:mailto:user@example.com",
		"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:carl@example.com")
	if strings.Contains(got, "SCHEDULE-AGENT") || strings.Contains(got, "ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:user@") {
		t.Errorf("the server's copy:\n%s", got)
	}
	id := w.Event.ID
	update := func(fields api.EventFields, notify *bool) error {
		_, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: id, Fields: fields, Notify: notify})
		return err
	}

	// What invitees do not see tells nobody.
	if err := update(api.EventFields{Alarms: []int64{10}}, nil); err != nil {
		t.Errorf("an alert asked: %v", err)
	}
	if err := update(api.EventFields{Transparent: ptr2(true)}, nil); err != nil {
		t.Errorf("Free asked: %v", err)
	}
	if err := update(api.EventFields{Attendees: invitees("dana@example.com?", "carl@example.com")}, nil); err != nil {
		t.Errorf("the same invitees asked: %v", err)
	}

	wantNotice(t, "rename", update(api.EventFields{Summary: ptr2("Design review")}, nil), api.NoticeKindUpdates, 2)
	if err := update(api.EventFields{Summary: ptr2("Design review")}, yes()); err != nil {
		t.Fatal(err)
	}
	wantNotice(t, "remove dana", update(api.EventFields{Attendees: invitees("carl@example.com")}, nil),
		api.NoticeKindCancellations, 1)
	wantNotice(t, "add erin", update(api.EventFields{Attendees: invitees("carl@example.com", "dana@example.com", "erin@example.com")}, nil),
		api.NoticeKindInvitations, 1)
	wantNotice(t, "swap carl for erin", update(api.EventFields{Attendees: invitees("dana@example.com", "erin@example.com")}, nil),
		api.NoticeKindUpdates, 2)
	later := start.Add(time.Hour)
	wantNotice(t, "move an hour and add erin", update(api.EventFields{AllDay: ptr2(false), Start: &later,
		End: ptr2(later.Add(time.Hour)), TimeZone: ptr2("UTC"), Attendees: invitees("carl@example.com", "dana@example.com", "erin@example.com")}, nil),
		api.NoticeKindUpdates, 3)
	if err := update(api.EventFields{CalendarID: ptr2(e.calendarID(e.home))}, yes()); !invalid(err) {
		t.Errorf("an event with invitees moved: %v", err)
	}

	_, err = e.cal.Delete(ctx, &api.CalendarDeleteParams{ID: id})
	wantNotice(t, "delete", err, api.NoticeKindCancellations, 2)
	deleted, err := e.cal.Delete(ctx, &api.CalendarDeleteParams{ID: id, Notify: yes()})
	if err != nil {
		t.Fatal(err)
	}
	e.pass()
	err = e.cal.Restore(ctx, &api.CalendarRestoreParams{RestoreID: deleted.RestoreID})
	wantNotice(t, "undo the delete", err, api.NoticeKindInvitations, 2)
	if err := e.cal.Restore(ctx, &api.CalendarRestoreParams{RestoreID: deleted.RestoreID, Notify: yes()}); err != nil {
		t.Errorf("the restore was used up by the refused one: %v", err)
	}
	e.pass()
	if rows, _ := e.db.Events(ctx, store.EventsFilter{}); len(rows) != 3 {
		t.Errorf("events after the restore: %d", len(rows))
	}
}

func TestSeriesOccurrencesTellTheirInvitees(t *testing.T) {
	e := newEventEnv(t)
	e.asGmail()
	ctx := t.Context()
	start := utcAt("2026-10-19T15:00Z")
	w, err := e.cal.Create(ctx, &api.CalendarCreateParams{Notify: yes(), Fields: api.EventFields{Summary: ptr2("Standup"),
		AllDay: ptr2(false), Start: &start, End: ptr2(start.Add(30 * time.Minute)), TimeZone: ptr2("UTC"),
		Rrule: ptr2("FREQ=DAILY;COUNT=5"), Attendees: invitees("carl@example.com")}})
	if err != nil {
		t.Fatal(err)
	}
	oct21 := "2026-10-21T15:00:00.000Z"
	this := func(fields api.EventFields, notify *bool) (*api.EventWrite, error) {
		return e.cal.Update(ctx, &api.CalendarUpdateParams{ID: w.Event.ID, RecurrenceID: &oct21,
			Scope: scope(api.EventScopeThis), Fields: fields, Notify: notify})
	}
	// An alert on one occurrence writes an override invitees cannot tell
	// from the occurrence.
	if _, err := this(api.EventFields{Alarms: []int64{5}}, nil); err != nil {
		t.Errorf("one occurrence's alert asked: %v", err)
	}
	later := utcAt("2026-10-21T16:00Z")
	moved := api.EventFields{AllDay: ptr2(false), Start: &later, End: ptr2(later.Add(30 * time.Minute)), TimeZone: ptr2("UTC")}
	_, err = this(moved, nil)
	wantNotice(t, "move one occurrence", err, api.NoticeKindUpdates, 1)
	m, err := this(moved, yes())
	if err != nil {
		t.Fatal(err)
	}
	err = e.cal.Restore(ctx, &api.CalendarRestoreParams{RestoreID: m.RestoreID})
	wantNotice(t, "undo the move", err, api.NoticeKindUpdates, 1)
	_, err = e.cal.Delete(ctx, &api.CalendarDeleteParams{ID: w.Event.ID, RecurrenceID: ptr2("2026-10-22T15:00:00.000Z"),
		Scope: scope(api.EventScopeThis)})
	wantNotice(t, "delete one occurrence", err, api.NoticeKindCancellations, 1)
}

// invitation is an event the user is invited to.
func invitation() []byte {
	return ics("BEGIN:VEVENT", "UID:party@example.com", "DTSTAMP:20261001T000000Z", "SEQUENCE:0",
		"DTSTART:20261011T180000Z", "DTEND:20261011T200000Z", "SUMMARY:Party",
		"ORGANIZER:mailto:ann@example.com", "ATTENDEE;PARTSTAT=ACCEPTED:mailto:user@example.com",
		"ATTENDEE:mailto:carl@example.com", "END:VEVENT")
}

func TestInvitationsTellNobody(t *testing.T) {
	e := newEventEnv(t)
	e.asGmail()
	e.dav.Put(e.work+"party.ics", invitation())
	e.pass()
	ctx := t.Context()
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Party"), Fields: api.EventFields{Alarms: []int64{60}}}); err != nil {
		t.Errorf("an alert on an invitation asked: %v", err)
	}
	if _, err := e.cal.Update(ctx, &api.CalendarUpdateParams{ID: e.event("Party"), Notify: yes(),
		Fields: api.EventFields{Attendees: invitees("dana@example.com")}}); !invalid(err) {
		t.Errorf("an invitation took invitees: %v", err)
	}
}
