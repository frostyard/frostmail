package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// ticket is a published event as a booking's mail carries it: an
// organizer, an attendee line for the buyer, a mail alarm, and a METHOD
// (or none).
func ticket(method string) string {
	lines := []string{}
	if method != "" {
		lines = append(lines, "METHOD:"+method)
	}
	lines = append(lines, "BEGIN:VEVENT", "UID:flight-400@airline.example", "DTSTAMP:20261009T140000Z",
		"DTSTART:20261020T083000Z", "DTEND:20261020T104500Z", "SUMMARY:Flight LH 400", "LOCATION:Gate B22",
		"DESCRIPTION:Booking ABC123", "ORGANIZER;CN=Airline:mailto:noreply@airline.example",
		"ATTENDEE;CN=User;PARTSTAT=ACCEPTED:mailto:user@dav.test",
		"BEGIN:VALARM", "ACTION:EMAIL", "TRIGGER:-PT3H", "SUMMARY:Check in", "DESCRIPTION:Check in now",
		"ATTENDEE:mailto:someone@else.example", "END:VALARM", "END:VEVENT")
	return string(vcal(lines...))
}

func (e *inviteEnv) add(t *testing.T, p *api.CalendarAddFromMessageParams) *api.CalendarEvent {
	t.Helper()
	ev, err := e.c.Calendar().AddFromMessage(t.Context(), p)
	if err != nil {
		t.Fatalf("add %+v: %v", p, err)
	}
	return ev
}

func TestAddFromMessageStoresThePublishedEventAsTheUsersOwn(t *testing.T) {
	for _, method := range []string{"PUBLISH", ""} {
		t.Run("method "+method, func(t *testing.T) {
			e := newInviteEnv(t)
			id := e.mail(t, ticket(method))
			inv := e.invitation(t, id)
			if inv.Method != api.ITIPMethodPublish || !inv.CanAdd || inv.CanRespond || inv.EventID != nil {
				t.Fatalf("before: method %s, canAdd %v, canRespond %v, eventId %v", inv.Method, inv.CanAdd, inv.CanRespond, inv.EventID)
			}
			ev := e.add(t, &api.CalendarAddFromMessageParams{MessageID: id})
			if ev.ID == 0 || ev.UID != "flight-400@airline.example" || ev.Summary != "Flight LH 400" || ev.Location != "Gate B22" ||
				ev.Description != "Booking ABC123" || !ev.Start.Equal(time.Date(2026, 10, 20, 8, 30, 0, 0, time.UTC)) ||
				!ev.End.Equal(time.Date(2026, 10, 20, 10, 45, 0, 0, time.UTC)) || ev.ReadOnly {
				t.Errorf("added = %+v", ev)
			}
			if ev.Organizer != nil || len(ev.Attendees) != 0 || len(ev.Alarms) != 0 || ev.Answer != nil {
				t.Errorf("the copy keeps people or alarms: organizer %+v, attendees %+v, alarms %v, answer %v",
					ev.Organizer, ev.Attendees, ev.Alarms, ev.Answer)
			}
			if items, _ := e.outbox(t); len(items) != 0 {
				t.Errorf("adding mailed %+v", items)
			}
			e.pass(t)
			copy := e.serverCopy(t, "flight-400@airline.example")
			for _, gone := range []string{"METHOD", "ORGANIZER", "ATTENDEE", "VALARM", "TRIGGER", "someone@else.example"} {
				if strings.Contains(copy, gone) {
					t.Errorf("the server's copy has %s:\n%s", gone, copy)
				}
			}
			for _, kept := range []string{"UID:flight-400@airline.example", "SUMMARY:Flight LH 400", "DTSTART:20261020T083000Z",
				"DTEND:20261020T104500Z", "LOCATION:Gate B22", "DESCRIPTION:Booking ABC123"} {
				if !strings.Contains(copy, kept) {
					t.Errorf("the server's copy lacks %s:\n%s", kept, copy)
				}
			}
			after := e.invitation(t, id)
			if after.EventID == nil || *after.EventID != ev.ID || after.CanAdd {
				t.Errorf("afterwards: eventId %v, canAdd %v", after.EventID, after.CanAdd)
			}
			if _, err := e.c.Calendar().AddFromMessage(t.Context(), &api.CalendarAddFromMessageParams{MessageID: id}); code(err) != api.CodeConflict {
				t.Errorf("a second add: %v", err)
			}
			occ, err := e.c.Calendar().Range(t.Context(), &api.CalendarRangeParams{From: "2026-10-20", To: "2026-10-21", TimeZone: ptr("UTC")})
			if err != nil || len(occ) != 1 || occ[0].EventID != ev.ID {
				t.Errorf("the day's occurrences = %+v, %v", occ, err)
			}
		})
	}
}

func TestAddFromMessageToANamedCalendar(t *testing.T) {
	e := newInviteEnv(t)
	e.dav.Calendar("trips", "Trips", "")
	e.pass(t)
	var work, trips int64
	cols, err := e.c.Account().Collections(t.Context(), &api.AccountCollectionsParams{AccountID: &e.acct})
	if err != nil {
		t.Fatal(err)
	}
	for _, col := range cols {
		switch col.Name {
		case "Work":
			work = col.ID
		case "Trips":
			trips = col.ID
		}
	}
	id := e.mail(t, ticket("PUBLISH"))
	if ev := e.add(t, &api.CalendarAddFromMessageParams{MessageID: id, CalendarID: &trips}); ev.CalendarID != trips || ev.CalendarID == work {
		t.Errorf("added to calendar %d, want Trips %d", ev.CalendarID, trips)
	}
}

func TestAddFromMessageRefuses(t *testing.T) {
	e := newInviteEnv(t)
	home := e.dav.Calendar("home", "Home", "")
	e.dav.SetReadOnly(home, true)
	e.pass(t)
	var homeID int64
	cols, err := e.c.Account().Collections(t.Context(), &api.AccountCollectionsParams{AccountID: &e.acct})
	if err != nil {
		t.Fatal(err)
	}
	for _, col := range cols {
		if col.Name == "Home" {
			homeID = col.ID
		}
	}
	published := e.mail(t, ticket("PUBLISH"))
	for name, tc := range map[string]struct {
		p    api.CalendarAddFromMessageParams
		code api.ErrorCode
	}{
		"an invitation to answer": {api.CalendarAddFromMessageParams{MessageID: e.mail(t, request("REQUEST", 1, userLine))}, api.CodeConflict},
		"a cancellation":          {api.CalendarAddFromMessageParams{MessageID: e.mail(t, request("CANCEL", 2, userLine))}, api.CodeConflict},
		"a message with no event": {api.CalendarAddFromMessageParams{MessageID: e.mail(t, "")}, api.CodeNotFound},
		"no such message":         {api.CalendarAddFromMessageParams{MessageID: 99999}, api.CodeNotFound},
		"no such calendar":        {api.CalendarAddFromMessageParams{MessageID: published, CalendarID: ptr(int64(99999))}, api.CodeNotFound},
		"a read-only calendar":    {api.CalendarAddFromMessageParams{MessageID: published, CalendarID: &homeID}, api.CodeConflict},
	} {
		if _, err := e.c.Calendar().AddFromMessage(t.Context(), &tc.p); code(err) != tc.code {
			t.Errorf("%s: %v, want %v", name, err, tc.code)
		}
	}
	if rows, err := e.srv.DB.EventsByUID(t.Context(), e.acct, "flight-400@airline.example"); err != nil || len(rows) != 0 {
		t.Errorf("a refused add stored %+v, %v", rows, err)
	}

	// With no calendar left that takes writes, the card offers nothing.
	e.dav.SetReadOnly(e.work, true)
	e.pass(t)
	if inv := e.invitation(t, published); inv.CanAdd {
		t.Error("canAdd with only read-only calendars")
	}
	if _, err := e.c.Calendar().AddFromMessage(t.Context(), &api.CalendarAddFromMessageParams{MessageID: published}); code(err) != api.CodeConflict {
		t.Errorf("an add with only read-only calendars: %v", err)
	}
}

// An invitation is never offered as an event to add, stored or not.
func TestInvitationCannotBeAdded(t *testing.T) {
	e := newInviteEnv(t)
	if inv := e.invitation(t, e.mail(t, request("REQUEST", 1, userLine))); inv.CanAdd {
		t.Error("a request offers canAdd")
	}
	if inv := e.invitation(t, e.mail(t, request("REQUEST", 1, ""))); inv.CanAdd {
		t.Error("a request that does not invite the user offers canAdd")
	}
}
