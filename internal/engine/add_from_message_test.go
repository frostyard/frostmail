package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
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

// Adds of one message at the same time store one copy: the check that no
// calendar holds the event runs in the transaction that stores it.
func TestAddFromMessageAtOnceStoresOneCopy(t *testing.T) {
	e := newInviteEnv(t)
	id := e.mail(t, ticket("PUBLISH"))
	const callers = 8
	start := make(chan struct{})
	errs := make(chan error, callers)
	for range callers {
		c := e.srv.Dial(t)
		go func() {
			<-start
			_, err := c.Calendar().AddFromMessage(t.Context(), &api.CalendarAddFromMessageParams{MessageID: id})
			errs <- err
		}()
	}
	close(start)
	added := 0
	for range callers {
		switch err := <-errs; {
		case err == nil:
			added++
		case code(err) != api.CodeConflict:
			t.Errorf("an add at the same time: %v", err)
		}
	}
	rows, err := e.srv.DB.EventsByUID(t.Context(), e.acct, "flight-400@airline.example")
	if err != nil || added != 1 || len(rows) != 1 {
		t.Errorf("%d adds succeeded and %d events are stored (%v), want 1 and 1", added, len(rows), err)
	}
}

// An event added to a calendar of another account is still the message's:
// the card names it and offers no second add.
func TestAddFromMessageToAnotherAccountKeepsTheMessagesLink(t *testing.T) {
	e := newInviteEnv(t)
	ctx := t.Context()
	p := createParams("other@dav.test")
	p.IMAP.Username = davtest.User
	other, err := e.c.Account().Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: other.ID, Password: davtest.Password}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: other.ID, Service: api.ServiceKindCalendar,
		Enabled: true, URL: ptr(e.dav.URL)}); err != nil {
		t.Fatal(err)
	}
	if err := e.srv.PIM.Pass(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	cols, err := e.c.Account().Collections(ctx, &api.AccountCollectionsParams{AccountID: &other.ID})
	if err != nil || len(cols) == 0 {
		t.Fatalf("the other account's calendars = %+v, %v", cols, err)
	}
	id := e.mail(t, ticket("PUBLISH"))
	ev := e.add(t, &api.CalendarAddFromMessageParams{MessageID: id, CalendarID: &cols[0].ID})
	if ev.AccountID != other.ID || ev.AccountID == e.acct {
		t.Fatalf("added to account %d, want the other account %d", ev.AccountID, other.ID)
	}
	inv := e.invitation(t, id)
	if inv.EventID == nil || *inv.EventID != ev.ID || inv.CanAdd {
		t.Errorf("the card afterwards: eventId %v (want %d), canAdd %v", inv.EventID, ev.ID, inv.CanAdd)
	}
	if _, err := e.c.Calendar().AddFromMessage(ctx, &api.CalendarAddFromMessageParams{MessageID: id}); code(err) != api.CodeConflict {
		t.Errorf("a second add, to the message's own account: %v", err)
	}
	if rows, err := e.srv.DB.EventsByUID(ctx, e.acct, "flight-400@airline.example"); err != nil || len(rows) != 0 {
		t.Errorf("the message's account holds %+v, %v", rows, err)
	}
}

// A message that publishes two unrelated events adds the one its card
// shows, alone: the object stored and put on the server holds one UID
// (frostyard/frostmail#311; RFC 4791 §4.1).
func TestAddFromMessageOfSeveralEventsAddsTheOneShown(t *testing.T) {
	e := newInviteEnv(t)
	ics := string(vcal("METHOD:PUBLISH",
		"BEGIN:VEVENT", "UID:outbound@airline.example", "DTSTAMP:20261009T140000Z", "DTSTART:20261020T083000Z",
		"DTEND:20261020T104500Z", "SUMMARY:Outbound flight", "ORGANIZER:mailto:noreply@airline.example", "END:VEVENT",
		"BEGIN:VEVENT", "UID:return@airline.example", "DTSTAMP:20261009T140000Z", "DTSTART:20261027T160000Z",
		"DTEND:20261027T181500Z", "SUMMARY:Return flight", "ORGANIZER:mailto:noreply@airline.example", "END:VEVENT"))
	id := e.mail(t, ics)
	inv := e.invitation(t, id)
	if inv.Event.UID != "outbound@airline.example" || !inv.CanAdd {
		t.Fatalf("the card shows %q, canAdd %v", inv.Event.UID, inv.CanAdd)
	}
	ev := e.add(t, &api.CalendarAddFromMessageParams{MessageID: id})
	if ev.UID != "outbound@airline.example" || ev.Summary != "Outbound flight" {
		t.Errorf("added = %+v", ev)
	}
	rows, err := e.srv.DB.EventsByUID(t.Context(), e.acct, "outbound@airline.example")
	if err != nil || len(rows) != 1 {
		t.Fatalf("stored = %+v, %v", rows, err)
	}
	obj, err := e.srv.DB.GetObject(t.Context(), rows[0].ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if raw := string(obj.Raw); strings.Contains(raw, "return@airline.example") || strings.Count(raw, "BEGIN:VEVENT") != 1 {
		t.Errorf("the object to put holds more than the event shown:\n%s", raw)
	}
	if other, err := e.srv.DB.EventsByUID(t.Context(), e.acct, "return@airline.example"); err != nil || len(other) != 0 {
		t.Errorf("the other event is stored: %+v, %v", other, err)
	}
	e.pass(t)
	if copy := e.serverCopy(t, "outbound@airline.example"); strings.Contains(copy, "return@airline.example") || strings.Contains(copy, "Return flight") {
		t.Errorf("the server's copy holds the other event:\n%s", copy)
	}
	if after := e.invitation(t, id); after.EventID == nil || *after.EventID != ev.ID || after.CanAdd {
		t.Errorf("the card afterwards: eventId %v, canAdd %v", after.EventID, after.CanAdd)
	}
}
